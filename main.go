package main

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"sync"
)

type clientConnection struct {
	WS *websocket.Conn
	ch chan WebSocketResponse
}

var upgrader = websocket.Upgrader{}
var clients = make(map[string]clientConnection)
var mu sync.Mutex

type InitialWSMessage struct {
	ClientID string `json:"client_id"`
}

type WebSocketMessage struct {
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Header    map[string]string `json:"header"`
	Query     string            `json:"query"`
	Body      string            `json:"body"`
	RequestID string            `json:"request_id"`
}

type WebSocketResponse struct {
	StatusCode int               `json:"status_code"`
	Header     map[string]string `json:"header"`
	Body       string            `json:"body"`
	RequestID  string            `json:"request_id"`
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/ws", wsHandler)
	r.PathPrefix("/").HandlerFunc(apiHandler)

	http.Handle("/", r)
	fmt.Println("Server started at :9999")
	log.Fatal(http.ListenAndServe(":9999", nil))
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("WS handler")
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal("Upgrade:", err)
		return
	}
	defer conn.Close()

	var initialMsg InitialWSMessage
	err = conn.ReadJSON(&initialMsg)
	if err != nil {
		log.Printf("Error while reading initial message: %v", err)
		return
	}

	log.Println("Client ID:", initialMsg.ClientID)

	ch := make(chan WebSocketResponse)

	clientID := initialMsg.ClientID
	mu.Lock()
	clients[clientID] = clientConnection{
		WS: conn,
		ch: ch,
	}
	mu.Unlock()

	for {
		var msg WebSocketResponse
		err := conn.ReadJSON(&msg)
		if err != nil {
			log.Printf("Error while reading: %v", err)
			break
		}
		ch <- msg
	}

	mu.Lock()
	delete(clients, clientID)
	mu.Unlock()
}

func apiHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("API handler")
	requestID := uuid.New().String()

	clientID := r.Header.Get("X-Client-ID")
	log.Println("Client ID:", clientID)

	mu.Lock()
	client, ok := clients[clientID]
	mu.Unlock()

	if !ok {
		http.Error(w, "Client not found", http.StatusBadGateway)
		return
	}

	var header = make(map[string]string)
	for k, v := range r.Header {
		header[k] = v[0]
	}

	msg := WebSocketMessage{
		Method:    r.Method,
		Path:      r.URL.Path,
		Header:    header,
		Query:     r.URL.RawQuery,
		Body:      "some-body", // Read and set the body content
		RequestID: requestID,
	}

	err := client.WS.WriteJSON(msg)
	if err != nil {
		http.Error(w, "Failed to send to client", http.StatusInternalServerError)
		return
	}

	select {
	case response := <-client.ch:
		for k, v := range response.Header {
			w.Header().Set(k, v)
		}
		w.WriteHeader(response.StatusCode)
		body := []byte(response.Body)
		w.Write(body)
		//json.NewEncoder(w).Encode(response)
	}

}
