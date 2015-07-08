package socketio

import (
	"crypto/rand"
	"errors"
	"io"
	log "github.com/cihub/seelog"
	"net/http"
	"reflect"
	"sync"
	"time"
)

const (
	SessionIDLength  = 32
	SessionIDCharset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

var NotConnected = errors.New("not connected")

type SessionHandler interface {
	SendMessage(data []byte) error
}

type Session struct {
	sync.Mutex
	SessionId string

	emitters          map[string]*EventEmitter
	nameSpaces        map[string]*NameSpace
	transport         Transport

	defaultNS         *NameSpace
	Values            map[interface{}]interface{}
	Request           *http.Request

	heartTicker chan bool
	server *SocketIOServer
	missedHeartbeats int
	isClosed	bool
	promoted	int64
	expiry		int64
	expiryTime int64
	handler    SessionHandler

	SendBuffer [][]byte
}

func NewSessionID() string {
	b := make([]byte, SessionIDLength)

	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return ""
	}

	for i := 0; i < SessionIDLength; i++ {
		b[i] = SessionIDCharset[b[i]%uint8(len(SessionIDCharset))]
	}

	return string(b)
}

func NewSession(sessionId string, srv *SocketIOServer, r *http.Request) *Session {
	sess := &Session{
		expiry :			int64(srv.Config.SessionExpiry),
		server :			srv,
		emitters:          srv.eventEmitters,
		SessionId:         sessionId,
		nameSpaces:        make(map[string]*NameSpace),

		Values:            make(map[interface{}]interface{}),
		Request:           r,

		SendBuffer: make([][]byte, 0),
	}
	if sess.expiry > 0 {
		sess.expiryTime = time.Now().Unix() + sess.expiry
	}
	
	sess.server.Stats.SessionOpened()
	
	sess.defaultNS = sess.Of("")
	sess.onOpen()
	sess.resetHeartBeat()
	return sess
}


func (ss *Session) Of(name string) (nameSpace *NameSpace) {
	ss.Lock()
	defer ss.Unlock()
	if nameSpace = ss.nameSpaces[name]; nameSpace == nil {
		ee := ss.emitters[name]
		if ee == nil {
			ss.emitters[name] = NewEventEmitter()
			ee = ss.emitters[name]
		}
		nameSpace = NewNameSpace(ss, name, ee)
		ss.nameSpaces[name] = nameSpace
	}
	return
}
func (ss *Session) isAlive() bool {
	return ss.expiryTime > time.Now().Unix()
}
func (ss *Session) promote()  {
	ss.promoted = time.Now().Unix() + ss.expiry
}

func (ss *Session) resetHeartBeat() {
	ss.stopHeartBeat()
	ss.heartBeat()
}
func (ss *Session) stopHeartBeat() {
	if ss.heartTicker != nil {
		//close(ss.heartTicker)
		select{
			case ss.heartTicker <- true:{}
			default:{}
		}
		
	}
}
func (ss *Session) heartBeat() chan bool{
	done := make(chan bool)
	ss.heartTicker = done
	go func(){
		t := time.Duration(ss.server.Config.HeartbeatInterval) * time.Second
		ticker := time.NewTicker(t)
		loop := true
		for loop {
			select {
				case <- ticker.C : 
					err := ss.defaultNS.sendPacket(new(heartbeatPacket))
					if err != nil {
						
					}
					log.Info("sent heart beat missed = ", ss.missedHeartbeats)
					ss.missedHeartbeats += 1
					// TODO: Configurable
					if ss.missedHeartbeats > 2 {
						log.Info("heartBeat missedHeartbeats ", ss.SessionId)
						ss.Close("")
						loop = false
					}
				case <- done:
					log.Infof("[%s] stop heartBeat", ss.SessionId)
					ticker.Stop()
					//ss.heartTicker = nil
					return 
			}
	    }
	}()
	return done
}


func (ss *Session) onPacket(packet Packet) error {
	switch packet.(type) {
	case *heartbeatPacket:
		ss.missedHeartbeats = 0
	case *disconnectPacket:
		//ss.defaultNS.onDisconnect()
		ss.Close(packet.EndPoint())
		//return NotConnected
	}
	return nil
}

func (ss *Session) onOpen() error {
	packet := new(connectPacket)
	ss.defaultNS.connected = true
	err := ss.defaultNS.sendPacket(packet)
	ss.defaultNS.emit("connect", ss.defaultNS, nil)

	return err
}

func (ss *Session) onClose() error {

	ss.defaultNS.emit("close", ss.defaultNS, nil)

	return nil
}

func (ss *Session) onDelete(forced bool) error {
	//Session expiration callback
    // `forced`
    //    If session item explicitly deleted, forced will be set to True. If
    //     item expired, will be set to False.
     
    // Do not remove connection if it was not forced and there's running connection
    if forced == false && ss.handler != nil && ss.isClosed == false {
		ss.promote()
	}else{
		ss.Close("")
	}
	return nil
}


func (ss *Session) Close(params ...interface{}) error {
	var endpoint string
	if len(params) == 0 {
		endpoint = ""
	}else {
		endpoint = params[0].(string)
	}
	if endpoint == "" {
		if ss.isClosed == false {
			for _, ns := range ss.nameSpaces {
				ns.onDisconnect()
			}
			ss.stopHeartBeat()
			ss.isClosed = true
			ss.server.Stats.SessionClosed()
			//ss.SendBuffer = nil
			ss.onClose()
		}
	}else{
		ns := ss.Of(endpoint)
		if ns != nil {
			ns.onDisconnect()
		}
	}
	return nil
}

func (ss *Session) Closed() bool {
	return ss.isClosed
}


func (ss *Session) Less(b *Session) bool {
	return ss.expiryTime < b.expiryTime
}


func (ss *Session) SetHandler(h SessionHandler) bool {
	ss.Lock()
	defer ss.Unlock()
	
	if ss.handler != nil {
		return false
	}
	ss.server.Stats.ConnectionOpened()
	ss.handler = h
	ss.promote()
	return true
}

func (ss *Session) RemoveHandler(h SessionHandler) bool {
	ss.Lock()
	defer ss.Unlock()
	
	if ss.handler == nil {
		return false
	}
	if reflect.ValueOf(h).Pointer() != reflect.ValueOf(ss.handler).Pointer() {
		log.Info("Attempted to remove invalid handler")
		return false
	}
	ss.server.Stats.ConnectionClosed()
	ss.handler = nil
	ss.promote()
	return true
}

func (ss *Session) RawMessage(msg []byte) error {
	log.Trace("RawMessage ", string(msg))
	packet, err := decodePacket(msg)
	if err != nil {
		log.Info("decodePacket error ", err, string(msg))
		return nil
	}
	if packet == nil {
		log.Info("packet == nil ")
		return nil
		
	}

	if packet.EndPoint() == "" {
		if err := ss.onPacket(packet); err != nil {
			log.Error(err)
			return nil
		}
	}

	ns := ss.Of(packet.EndPoint())
	if ns == nil {
		return nil
	}
	ns.onPacket(packet)
	return nil
}

func (ss *Session) Send(data []byte) error {
	ss.Lock()
	ss.SendBuffer = append(ss.SendBuffer, data)
	ss.Unlock()
	
	return ss.Flush()

}

func (ss *Session) Flush() error {

	c, queues := func() (bool, [][]byte){
		//获取要发送的数据
		//锁不能放在全局里 因为handler.SendMessage有可能会调用session有锁的方法
		//会导致死锁，所以这里是为了缩短锁周期
		ss.Lock()
		defer ss.Unlock()
		
		if len(ss.SendBuffer) == 0 {
			log.Debug("Flush not buffered")
			return false, nil
		}
		if ss.handler == nil {
			log.Debug("Flush Handler is Null ")
			return false, nil
		}
		data := ss.SendBuffer[:]
		ss.SendBuffer = ss.SendBuffer[:0]
		return true, data
	}()
	
	if c == false {
		return nil
	}
	
	//发送数据
	data := encodePayload(queues)
	r := ss.handler.SendMessage(data)
	if r != nil {
		log.Error("Flush error", r)
		
		//出错将数据重置回缓冲区
		ss.Lock()
		ss.SendBuffer = append(queues, ss.SendBuffer ... )
		ss.Unlock()
		return r 
	}


	return r

}
