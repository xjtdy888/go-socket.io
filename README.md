##socket.io library for Golang

*Branch master branch is compatible with socket.io 0.9.x. For latest version, please check branch 1.0.*

forked from [http://code.google.com/p/go-socketio](http://code.google.com/p/go-socketio)
Documentation: http://godoc.org/github.com/googollee/go-socket.io
##Demo

**server:**
//example/server/app.go
```go
package main

import (
	"runtime"
	//"io/ioutil"
	"encoding/json"
	"github.com/xjtdy888/go-socket.io"
	"log"
	"net/http"
	_ "net/http/pprof"
	//"strings"
)


func main() {
	log.SetFlags(log.LstdFlags|log.Lshortfile)
	//log.SetOutput(ioutil.Discard)
	sio := socketio.NewSocketIOServer(&socketio.Config{})
	//sio.Config.NameSpace = "/tbim-1/1/"

	// Set the on connect handler
	sio.On("connect", func(ns *socketio.NameSpace) {
		log.Println("Connected: ", ns.Id())
		sio.Broadcast("connected", ns.Id())
	})

	// Set the on disconnect handler
	sio.On("disconnect", func(ns *socketio.NameSpace) {
		log.Println("Disconnected: ", ns.Id())
		sio.Broadcast("disconnected", ns.Id())
	})

	// Set a handler for news messages
	sio.On("news", func(ns *socketio.NameSpace, message string) {
		sio.Broadcast("news", message)
	})

	// Set a handler for ping messages
	sio.On("ping", func(ns *socketio.NameSpace) {
		ns.Emit("pong", nil)
	})

	// Set an on connect handler for the pol channel
	sio.Of("/pol").On("connect", func(ns *socketio.NameSpace) {
		log.Println("Pol Connected: ", ns.Id())
	})

	// We can broadcast messages. Set a handler for news messages from the pol
	// channel
	sio.Of("/pol").On("news", func(ns *socketio.NameSpace, message string) {
		sio.In("/pol").Broadcast("news", message)
	})

	// And respond to messages! Set a handler with a response for poll messages
	// on the pol channel
	sio.Of("/pol").On("poll", func(ns *socketio.NameSpace, message string) bool {
		if strings.Contains(message, "Nixon") {
			return true
		}

		return false
	})

	// Set an on disconnect handler for the pol channel
	sio.Of("/pol").On("disconnect", func(ns *socketio.NameSpace) {
		log.Println("Pol Disconnected: ", ns.Id())
	})

	// Serve our website
	sio.Handle("/", http.FileServer(http.Dir("./www/")))


	sio.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request){
		stats := sio.Stats.Dump()
		data, err := json.Marshal(stats)
		if err != nil {
			return 
		}
		w.Write(data)
		runtime.GC()
	})
	
	go func() {
	        log.Println(http.ListenAndServe(":6060", nil)) 
	
	}()
	

	// Start listening for socket.io connections
	log.Println("listening on port 3000")
	log.Fatal(http.ListenAndServe(":3000", sio))
}

```



##Changelog
- Added a socket.io client for quick use
- Fixed the disconnect event
- Added persistent sessionIds
- Added session values
- Added broadcast
- Added a simpler Emit function to namespaces
- Fixed connected event on endpoints
- Added events without arguments
- Added Polling Transport
