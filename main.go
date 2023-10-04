package main

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/spf13/viper"
	"log"
	"net/http"
	"strings"
	"sync"
)

type privateServerConnection struct {
	ws *websocket.Conn
	ch chan webSocketResponse
}

var upgrader = websocket.Upgrader{}
var privateServer = make(map[string]privateServerConnection)
var mu sync.Mutex

type serverInfoMessage struct {
	PrivateServerID string `json:"private_server_id"`
}

type webSocketMessage struct {
	RequestID string            `json:"request_id"`
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Header    map[string]string `json:"header"`
	Query     string            `json:"query"`
	Body      string            `json:"body"`
}

type webSocketResponse struct {
	RequestID  string            `json:"request_id"`
	StatusCode int               `json:"status_code"`
	Header     map[string]string `json:"header"`
	Body       string            `json:"body"`
}

func initConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	err := viper.ReadInConfig()
	if err != nil {
		log.Fatal("Error reading config file:", err)
	}
}

func main() {
	initConfig()

	r := mux.NewRouter()
	r.HandleFunc("/ws", wsHandler)

	// handle all other requests
	r.PathPrefix("/").HandlerFunc(apiHandler)

	http.Handle("/", r)
	fmt.Printf("Server started at port: %v...\n", viper.GetInt("service.port"))
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%v", viper.GetInt("service.port")), nil))
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("call websocket handler")

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal("Upgrade:", err)
		return
	}
	defer conn.Close()

	var serverInfo serverInfoMessage
	err = conn.ReadJSON(&serverInfo)
	if err != nil {
		log.Printf("Error while reading server info message: %v", err)
		return
	}
	log.Println("received server info message, private_server_id:", serverInfo.PrivateServerID)

	ch := make(chan webSocketResponse)

	mu.Lock()
	privateServer[serverInfo.PrivateServerID] = privateServerConnection{
		ws: conn,
		ch: ch,
	}
	mu.Unlock()

	for {
		var msg webSocketResponse
		err := conn.ReadJSON(&msg)
		if err != nil {
			log.Printf("Error while reading: %v", err)
			break
		}
		ch <- msg
	}

	mu.Lock()
	delete(privateServer, serverInfo.PrivateServerID)
	mu.Unlock()
}

func apiHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("API handler")
	requestID := uuid.New().String()

	clientID := r.Header.Get("X-Client-ID")
	log.Println("Client ID:", clientID)

	mu.Lock()
	client, ok := privateServer[clientID]
	mu.Unlock()

	if !ok {
		http.Error(w, "Client not found", http.StatusBadGateway)
		return
	}

	var header = make(map[string]string)
	for k, v := range r.Header {
		header[k] = v[0]
	}

	msg := webSocketMessage{
		Method:    r.Method,
		Path:      r.URL.Path,
		Header:    header,
		Query:     r.URL.RawQuery,
		Body:      "some-body", // Read and set the body content
		RequestID: requestID,
	}

	err := client.ws.WriteJSON(msg)
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
