package socketio

import (
	"fmt"
	"errors"
	"sync"
	"bytes"
	log "github.com/cihub/seelog"
	"io/ioutil"
	"net/http"
	"time"
)

func init() {
	DefaultTransports.RegisterTransport("xhr-polling")
}

type pollingSocket struct {
	sync.Mutex
	server *SocketIOServer
	session *Session
	timeout time.Duration

	response http.ResponseWriter
	request *http.Request
	done chan bool
	waiting bool
	closed bool
	
}

func newPollingSocket(session *Session, srv *SocketIOServer) *pollingSocket {
	ret := &pollingSocket{
		server : srv,
		session: session,
		timeout: time.Second * time.Duration(session.server.Config.PollingTimeout),
		done: make(chan bool),
	}
	return ret
}


func (s *pollingSocket) get(w http.ResponseWriter, r *http.Request) {

	session := s.session
	
	if res := session.SetHandler(s); res != true {
		log.Warn("Polling Get SetHandler False ", r.URL.Path)
		w.WriteHeader(401)
		return 
	}

	if len(session.SendBuffer) > 0 {
		log.Debug("Polling Get Have Buffered Flush ", r.URL.Path)
		session.Flush()
	}else {
		log.Debug("Polling Waiting Data ", r.URL.Path)
		s.waiting = true
		if session.Closed() {
			//Not Send Buffer And Session is Closed
			//Reject poll data
			log.Warn("Reject Closed Session Polled data", r.URL.Path)
			w.WriteHeader(401)
			//s.detach()
			return 
		}
		
		cn, _ := w.(http.CloseNotifier)
		
		var isDone = false
		select {
			case <-cn.CloseNotify():
				log.Debug("CloseNotifier ", r.URL.Path)
			break
			case <- time.After(s.timeout):
				log.Debug("Polling timeout ", s.timeout)
				s.SendMessage(encodePacket("", new(noopPacket)))
			break
			case <- s.done:
				log.Debug("Get Done ",  r.URL.Path)
				isDone = true
			break
		}
		s.waiting = false
		if isDone == false {
			s.Lock()
			defer s.Unlock()
			s.detach()
			s.finish()
		}

	}
}


func (s *pollingSocket) detach() {

	if s.session != nil {
		s.session.RemoveHandler(s)

		log.Infof("[%s] detach", s.session.SessionId)
		
	}
}
func (s *pollingSocket) finish() {

	if s.closed == true { return }
	log.Infof("[%s] finish", s.session.SessionId)
	select {
		case s.done <- true:
		default: {
			if s.waiting {
				log.Errorf("[%s] set done status failed closed=[%v]", s.session.SessionId, s.closed)
			}
		}
	}

	s.closed = true
	s.session = nil
	s.server = nil
	s.request = nil
}

func (s *pollingSocket) post(w http.ResponseWriter, r *http.Request) {
	
	//Stats
	s.server.Stats.ConnectionOpened()
	defer s.server.Stats.ConnectionClosed()
	defer s.finish()
	
	data, err := ioutil.ReadAll(r.Body)

	if err != nil {
		log.Error("read post body error", err, r.URL.Path)
		return
	}
	
	//IE XDomainRequest support
	if bytes.HasPrefix(data, []byte("data=")) {
		log.Debug("IE XDomainRequest remove [data=]", string(data), r.URL.Path)
		data = data[5:len(data)]
	}
	
	// Tracking
	s.server.Stats.OnPacketRecv(len(data))
	
	log.Tracef("[%s]>>> %s", s.session.SessionId, string(data))
	
	packets := decodeFrames(data)
	for _, msg := range packets {
		s.session.RawMessage(msg)
	}
}

var ClosingError = errors.New("Handler Closing")

func (s *pollingSocket) SendMessage(data []byte) error{
	s.Lock()
	defer s.Unlock()
	
	if s.closed {
		// 每个GET请求，只返回一次信息，所有SendMessage后 就将连接关闭，等待下一次GET
		// 这里的锁和closed属性避免多次SendMessage 导致的异常情况
		return ClosingError
	}
	
	defer s.finish()
	defer s.detach()
	
	log.Info("polling sendmessage")
	
	s.server.Stats.OnPacketSent(len(data))
			
	log.Tracef("[%s]<<< %s", s.session.SessionId, string(data))
	s.response.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	s.response.Header().Set("Content-Length", fmt.Sprintf("%d",len(data)))
	
	_, err := s.response.Write(data)
	if err != nil {
		return err
	}

	/*s.detach()
	s.finish()*/
	return nil
}

func (s *pollingSocket) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != "" {
		w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
		w.Header().Set("Access-Control-Allow-Credentials", "true")
        w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	}
	s.response = w
	s.request = r
	switch r.Method {
	case "GET":
		s.get(w, r)
	case "POST":
		s.post(w, r)
	}
}
