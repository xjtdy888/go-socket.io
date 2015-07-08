package socketio

import (
	log "github.com/cihub/seelog"

	"time"
	"container/heap"
	"sync"
)

type sessHeap []*Session

func (h sessHeap) Len() int                { return len(h) }
func (h sessHeap) Less(i, j int) bool { return h[i].Less(h[j]) }
func (h sessHeap) Swap(i, j int)           { h[i], h[j] = h[j], h[i] }

func (h *sessHeap) Push(x interface{}) {
	// Push and Pop use pointer receivers because they modify the slice's length,
	// not just its contents.
	*h = append(*h, x.(*Session))
}

func (h *sessHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

type SessionContainer struct {
	mutex    sync.RWMutex
	sessions map[string]*Session
	queue    *sessHeap
}

func NewSessionContainer() *SessionContainer {
	r := &SessionContainer{
		sessions: make(map[string]*Session),
		queue: &sessHeap{},
	}
	heap.Init(r.queue)
	return r 
}
func (s *SessionContainer) Add(ss *Session) {

	s.mutex.Lock()
	defer s.mutex.Unlock()
	log.Info("Add Session", ss.SessionId)
	s.sessions[ss.SessionId] = ss
	heap.Push(s.queue, ss)

}

func (s *SessionContainer) Remove(ss *Session) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	ss.promoted = -1
  	ss.onDelete(true)

	delete(s.sessions, ss.SessionId)
}

func (s *SessionContainer) Get(sessionId string) *Session {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.sessions[sessionId]
}

func (s *SessionContainer) Expire() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.queue.Len() == 0 { 
		return 
	}
	log.Debug("sessioncontainer ExpireSession ", s.queue.Len())
	currentTime := time.Now().Unix()
	i := 0
	for s.queue.Len() > 0 {
		i++
		top := (*s.queue)[0]
		// Early exit if item was not promoted and its expiration time
        // is greater than now.
        if top.promoted == 0 && top.expiryTime > currentTime {
			break
		}
		// Pop item from the stack
        top = heap.Pop(s.queue).(*Session)
		need_reschedule := (top.promoted !=0  && top.promoted > currentTime)
							
		// Give chance to reschedule
        if need_reschedule == false {
			top.promoted = 0
           	top.onDelete(false)
			
            need_reschedule = (top.promoted != 0 && top.promoted > currentTime)	
		}
		// If item is promoted and expiration time somewhere in future
        // just reschedule it
        if need_reschedule {
            top.expiryTime = top.promoted
            top.promoted = 0
			heap.Push(s.queue, top)
		} else {
			log.Debug("session expire ", top.SessionId, top.expiryTime, currentTime)
			delete(s.sessions, top.SessionId)
		}  
	}
}

func (s *SessionContainer) Range() map[string]*Session {
	return s.sessions
}
