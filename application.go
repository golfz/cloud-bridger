package main

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/spf13/viper"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const httpHeaderPrivateServerID = "X-Private-Server-ID"

type privateServerConnection struct {
	ws *websocket.Conn
	ch chan webSocketResponseMessage
}

var upgrader = websocket.Upgrader{}
var privateServer = make(map[string]privateServerConnection)
var mu sync.Mutex

type serverInfoMessage struct {
	PrivateServerID string `json:"private_server_id"`
}

type webSocketRequestMessage struct {
	RequestID string            `json:"request_id"`
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Header    map[string]string `json:"header"`
	Query     string            `json:"query"`
	Body      string            `json:"body"`
}

type webSocketResponseMessage struct {
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
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)

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
	defer log.Println("wsHandler finished")

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

	ch := make(chan webSocketResponseMessage)

	mu.Lock()
	privateServer[serverInfo.PrivateServerID] = privateServerConnection{
		ws: conn,
		ch: ch,
	}
	mu.Unlock()

	defer func() {
		log.Printf("websocket connection closed (id:%s)\n", serverInfo.PrivateServerID)

		mu.Lock()
		delete(privateServer, serverInfo.PrivateServerID)
		mu.Unlock()
		close(ch)
		log.Printf("removed private server (id:%s) from list\n", serverInfo.PrivateServerID)
	}()

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	done := make(chan struct{})
	defer close(done)

	go func() {
		for {
			var msg webSocketResponseMessage
			err := conn.ReadJSON(&msg)
			if err != nil {
				log.Printf("Error while reading: %v", err)
				ticker.Stop()
				done <- struct{}{}
				return
			}
			ch <- msg
		}
	}()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			err := conn.WriteMessage(websocket.PingMessage, []byte{})
			if err != nil {
				log.Println("Error while sending ping:", err)
				return
			}
		}
	}

}

func apiHandler(w http.ResponseWriter, r *http.Request) {
	requestID := uuid.New().String()
	fmt.Println("call api handler for requestID:", requestID)

	serverID := r.Header.Get(httpHeaderPrivateServerID)
	log.Printf("[req:%s] %s in header: %s\n", requestID, httpHeaderPrivateServerID, serverID)

	mu.Lock()
	server, ok := privateServer[serverID]
	mu.Unlock()
	if !ok {
		log.Printf("[req:%s] private server not found\n", requestID)
		http.Error(w, "private server not found", http.StatusBadGateway)
		return
	}

	var header = make(map[string]string)
	for k, v := range r.Header {
		header[k] = v[0]
	}

	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
	}

	msg := webSocketRequestMessage{
		RequestID: requestID,
		Method:    r.Method,
		Path:      r.URL.Path,
		Header:    header,
		Query:     r.URL.RawQuery,
		Body:      string(body),
	}

	log.Printf("[req:%s] try to sent method: %s, path: %s\n", requestID, msg.Method, msg.Path)

	err := server.ws.WriteJSON(msg)
	if err != nil {
		log.Printf("[req:%s] Error while sending request to private server: %v\n", requestID, err)
		http.Error(w, "failed to send request to private server", http.StatusInternalServerError)
		return
	}

	log.Printf("[req:%s] wait for response from private server\n", requestID)
	response, ok := <-server.ch
	if !ok {
		log.Printf("[req:%s] private server closed connection\n", requestID)
		http.Error(w, "private server closed connection", http.StatusBadGateway)
		return
	}

	log.Printf("[req:%s] received response from private server\n", requestID)

	for k, v := range response.Header {
		w.Header().Set(k, v)
	}
	w.WriteHeader(response.StatusCode)
	respBody := []byte(response.Body)
	_, err = w.Write(respBody)
	if err != nil {
		log.Println("Error writing response body:", err)
	}

	log.Printf("[req:%s] already responsed, api handler finished\n", requestID)
}
