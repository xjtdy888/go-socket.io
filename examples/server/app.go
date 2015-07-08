package main

import (
	"runtime"
	//"io/ioutil"
	"encoding/json"
	"qudao.com/tech/socket.io"
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
	/*sio.On("disconnect", func(ns *socketio.NameSpace) {
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
	})*/

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
