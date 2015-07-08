package socketio

import (
	"time"
	"fmt"
	"net/http"
	"regexp"
	"strings"
//	"golang.org/x/net/websocket"
	
)

var (
	uriRegexp = regexp.MustCompile(`^(.+?)/(1)(?:/([^/]+)/([^/]+))?/?$`)
)

type Config struct {
	ResourceName		string     //命名空间
	HeartbeatInterval int		//心跳时间
	SessionExpiry	 int		//session 有效时间
	SessionCheckInterval int	//session 检查间隔
	PollingTimeout	 int		//polling 超时时间
	ClosingTimeout   int
	NewSessionID     func() string
	Transports       *TransportManager
	Authorize        func(*http.Request, map[interface{}]interface{}) bool
}

type SocketIOServer struct {
	*http.ServeMux
	
	Config 			 *Config
	Stats 			*StatsCollector
	sessions		*SessionContainer
	eventEmitters    map[string]*EventEmitter
}

func NewSocketIOServer(config *Config) *SocketIOServer {
	c := &Config {
		ResourceName: "/socket.io/1/",
		HeartbeatInterval : 12,
		SessionExpiry : 30,
		SessionCheckInterval : 15,
		PollingTimeout: 20,
		ClosingTimeout : 15,
		NewSessionID: NewSessionID,
		Transports: DefaultTransports,
	}
	server := &SocketIOServer{
		ServeMux: http.NewServeMux(),
		sessions : NewSessionContainer(),
		eventEmitters : make(map[string]*EventEmitter),
		Stats : NewStatsCollector(),
	}
	server.Config = c;
	if config != nil {
		c.Authorize = config.Authorize
		if config.HeartbeatInterval != 0 {
			c.HeartbeatInterval = config.HeartbeatInterval
		}
		if config.ClosingTimeout != 0 {
			c.ClosingTimeout = config.ClosingTimeout
		}
		if config.NewSessionID != nil {
			c.NewSessionID = config.NewSessionID
		}
		if config.Transports != nil {
			c.Transports = config.Transports
		}
		if config.ResourceName != "" {
			c.ResourceName = config.ResourceName
		}
	}

	server.sessionCheckInterval()
	server.Stats.Start()

	return server
}

func (srv *SocketIOServer) sessionCheckInterval() {
	go func(){
		t := time.Duration(srv.Config.SessionCheckInterval) * time.Second
		ticker := time.NewTicker(t)
		for _ = range ticker.C {
			srv.sessions.Expire()
		}
	}()
	
}

func (srv *SocketIOServer) Sessions() *SessionContainer{
	return srv.sessions
}

func (srv *SocketIOServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	
	path := r.URL.Path
	io := srv.Config.ResourceName
	if !strings.HasPrefix(path, io) {

		/*cookie, _ := r.Cookie("socket.io.sid")
		if cookie == nil {
			http.SetCookie(w, &http.Cookie{
				Name:  "socket.io.sid",
				Value: NewSessionID(),
				Path:  "/",
			})
		}*/
		srv.ServeMux.ServeHTTP(w, r)
		return
	}
	path = path[len(io):]
	if path == "" {
		srv.handShake(w, r)
		return
	}

	spliter := strings.SplitN(path, "/",3)
	if len(spliter) < 2 {
		http.NotFound(w, r)
		return
	}

	transportName, sessionId := spliter[0], spliter[1]
	if transportName != "websocket" && transportName != "xhr-polling" {
		http.Error(w, "not websocket", http.StatusBadRequest)
		return
	}

	session := srv.sessions.Get(sessionId)
	if session == nil {
		http.Error(w, "invalid session id "+sessionId, http.StatusBadRequest)
		return
	}
	
	
	
	switch transportName {
		case "websocket":
			// open
			//transport := newWebSocket(session, srv)
			//websocket.Handler(transport.webSocketHandler).ServeHTTP(w, r)
		case "xhr-polling":
			polling := newPollingSocket(session, srv)
			polling.ServeHTTP(w, r)
	}
	


	
}

func (srv *SocketIOServer) Of(name string) *EventEmitter {
	ret, ok := srv.eventEmitters[name]
	if !ok {
		ret = NewEventEmitter()
		srv.eventEmitters[name] = ret
	}
	return ret
}

func (srv *SocketIOServer) In(name string) *Broadcaster {
	namespaces := []*NameSpace{}
	for _, session := range srv.sessions.Range() {
		ns := session.Of(name)
		if ns != nil {
			namespaces = append(namespaces, ns)
		}
	}

	return &Broadcaster{Namespaces: namespaces}
}

func (srv *SocketIOServer) Broadcast(name string, args ...interface{}) {
	srv.In("").Broadcast(name, args...)
}

func (srv *SocketIOServer) Except(ns *NameSpace) *Broadcaster {
	return srv.In("").Except(ns)
}

func (srv *SocketIOServer) On(name string, fn interface{}) error {
	return srv.Of("").On(name, fn)
}

func (srv *SocketIOServer) RemoveListener(name string, fn interface{}) {
	srv.Of("").RemoveListener(name, fn)
}

func (srv *SocketIOServer) RemoveAllListeners(name string) {
	srv.Of("").RemoveAllListeners(name)
}

// authorize origin!!
func (srv *SocketIOServer) handShake(w http.ResponseWriter, r *http.Request) {
	srv.Stats.ConnectionOpened()
	defer srv.Stats.ConnectionClosed()
	
	var values = make(map[interface{}]interface{})
	if srv.Config.Authorize != nil {
		if ok := srv.Config.Authorize(r, values); !ok {
			http.Error(w, "", 401)
			return
		}
	}
	w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("origin"))
	w.Header().Set("Access-Control-Allow-Methods", "GET")
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	/*cookie, _ := r.Cookie("socket.io.sid")
	var sessionId string

	if cookie == nil {
		sessionId = NewSessionID()
	} else {
		sessionId = cookie.Value
	}*/
	var sessionId string = NewSessionID()
	if sessionId == "" {
		http.Error(w, "", 503)
		return
	}

	transportNames := srv.Config.Transports.GetTransportNames()
	
	data := fmt.Sprintf("%s:%d:%d:%s",
			sessionId,
			srv.Config.HeartbeatInterval,
			srv.Config.ClosingTimeout,
			strings.Join(transportNames, ","))
	
	jsonp := r.URL.Query().Get("jsonp")
	if jsonp != "" {
		w.Header().Set("Content-Type", "application/javascript; charset=UTF-8")
		data = fmt.Sprintf("io.j[%s](\"%s\");",  jsonp, data)
	} else {
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	}
	fmt.Fprintf(w, "%s", data)
	
	

	session := srv.sessions.Get(sessionId)
	if session == nil {
		session = NewSession(sessionId, srv, r)
		srv.sessions.Add(session)
	}

	if values != nil {
		for k, v := range values {
			session.Values[k] = v
		}
	}
}

