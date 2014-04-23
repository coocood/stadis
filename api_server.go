package stadis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

//The API server holds the state of topology and proxy servers, serve requests from API client.
type ApiServer struct {
	mu      sync.RWMutex
	topo    *topology
	proxies map[string]*proxyServer
}

func NewApiServer() (ms *ApiServer) {
	ms = new(ApiServer)
	ms.proxies = make(map[string]*proxyServer)
	ms.topo, _ = newTopology(bytes.NewReader(DefaultConfig))
	return
}

func (s *ApiServer) connState(w http.ResponseWriter, r *http.Request) {
	clientPort := intFormValue(r, "clientPort")
	if clientPort == 0 {
		http.Error(w, "'clientPort' required", 400)
		return
	}
	serverPort := intFormValue(r, "serverPort")
	if serverPort == 0 {
		http.Error(w, "'serverPort' required", 400)
		return
	}
	s.mu.Lock()
	topo := s.topo
	s.mu.Unlock()
	updateCh := topo.getUpdateChannel()
	connState, err := topo.connState(clientPort, serverPort)
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), 400)
		return
	}
	oldStateStr := r.Header.Get("If-None-Match")
	if oldStateStr == "" {
		connStateBytes, _ := json.Marshal(connState)
		w.Write(connStateBytes)
		return
	}

	var oldState ConnState
	err = json.Unmarshal([]byte(oldStateStr), &oldState)
	if err != nil {
		log.Println(err)
		http.Error(w, "invalid If-None-Match header", 400)
		return
	}
	if oldState == connState {
		//long-poling
		select {
		case <-time.After(time.Second * 3):
		case <-updateCh:
			connState, _ = topo.connState(clientPort, serverPort)
		}
	}
	if oldState == connState {
		w.WriteHeader(304)
	} else {
		connStateBytes, _ := json.Marshal(connState)
		w.Write(connStateBytes)
	}
}

func (s *ApiServer) dialState(w http.ResponseWriter, r *http.Request) {
	clientName := r.FormValue("clientName")
	if clientName == "" {
		http.Error(w, "'clientName' required", 400)
		return
	}
	serverPort := intFormValue(r, "serverPort")
	if serverPort == 0 {
		http.Error(w, "'serverPort' required", 400)
		return
	}
	connState, err := s.topo.dialState(clientName, serverPort)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	jsonBytes, _ := json.Marshal(connState)
	w.Write(jsonBytes)
}

func (s *ApiServer) serverPort(w http.ResponseWriter, r *http.Request) {
	port := intFormValue(r, "port")
	if port == 0 {
		http.Error(w, "'port' required", 400)
		return
	}
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "'name' required", 400)
		return
	}
	var err error
	switch r.Method {
	case "POST":
		err = s.topo.addServerPort(name, port)
	case "DELETE":
		err = s.topo.removeServerPort(name, port)
	}
	if err != nil {
		http.Error(w, err.Error(), 400)
	}
}

func (s *ApiServer) clientPort(w http.ResponseWriter, r *http.Request) {
	port := intFormValue(r, "port")
	if port == 0 {
		http.Error(w, "'port' required", 400)
		return
	}
	var err error
	switch r.Method {
	case "POST":
		name := r.FormValue("name")
		if name == "" {
			http.Error(w, "'name' required", 400)
			return
		}
		err = s.topo.addClientPort(name, port)
	case "DELETE":
		err = s.topo.removeClientPort(port)
	}
	if err != nil {
		http.Error(w, err.Error(), 400)
	}
}

func (s *ApiServer) nodeState(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	nodeState, err := s.topo.nodeState(name)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if r.Method == "GET" {
		data, _ := json.Marshal(nodeState)
		w.Write(data)
	} else if r.Method == "POST" {
		var newState NodeState
		decoder := json.NewDecoder(r.Body)
		err = decoder.Decode(&newState)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		s.topo.setNodeState(name, newState)
	}
}

func (s *ApiServer) postConfig(w http.ResponseWriter, r *http.Request) {
	topo, err := newTopology(r.Body)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.mu.Lock()
	if s.topo != nil {
		close(s.topo.getUpdateChannel())
	}
	s.topo = topo
	s.mu.Unlock()
}

func (s *ApiServer) proxy(w http.ResponseWriter, r *http.Request) {

	proxyPort := r.FormValue("proxyPort")
	if proxyPort == "" {
		http.Error(w, "'proxyPort' required", 400)
		return
	}
	ps := s.getProxy(proxyPort)
	var err error
	switch r.Method {
	case "POST":
		if ps != nil {
			errStr := "proxy port is taken"
			log.Println(errStr)
			http.Error(w, errStr, 400)
			return
		}
		proxyName := r.FormValue("proxyName")
		if proxyName == "" {
			http.Error(w, "'proxyName' required", 400)
			return
		}
		originAddr := r.FormValue("originAddr")
		if originAddr == "" {
			http.Error(w, "'originAddr' required", 400)
			return
		}
		clientName := r.FormValue("clientName")
		if clientName == "" {
			http.Error(w, "'clientName' required", 400)
			return
		}
		ps, err = newProxyServer(clientName, proxyName, proxyPort, originAddr)
		if err != nil {
			log.Println(err)
			http.Error(w, err.Error(), 400)
			return
		}
		go ps.serve()
		s.setProxy(proxyPort, ps)
	case "PUT":
		if ps == nil {
			errStr := "proxy server not found"
			log.Println(errStr)
			http.Error(w, errStr, 404)
			return
		}
		clientName := r.FormValue("clientName")
		if clientName == "" {
			http.Error(w, "'clientName' required", 400)
			return
		}
		ps.clientName = clientName
	case "DELETE":
		if ps == nil {
			errStr := "proxy server not found"
			log.Println(errStr)
			http.Error(w, errStr, 404)
			return
		}
		err = ps.close()
		if err != nil {
			log.Println(err)
			return
		}
		s.setProxy(proxyPort, nil)
	}
}

func (s *ApiServer) getProxy(port string) (ps *proxyServer) {
	s.mu.RLock()
	ps = s.proxies[port]
	s.mu.RUnlock()
	return
}

func (s *ApiServer) setProxy(port string, ps *proxyServer) {
	s.mu.Lock()
	s.proxies[port] = ps
	s.mu.Unlock()
}

//dump the node state, if 'name' is empty, the whole topology will be dumped.
func (s *ApiServer) DumpNode(name string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if name == "" {
		fmt.Println(s.topo)
		return
	}
	node, err := s.topo.lookup(name)
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Println(node)
}

func (s *ApiServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/config" && r.Method == "POST" {
		s.postConfig(w, r)
		return
	}
	s.mu.Lock()
	topo := s.topo
	s.mu.Unlock()
	if topo == nil {
		http.Error(w, "server uninitialized.", 400)
		return
	}
	switch r.URL.Path {
	case "/connState":
		s.connState(w, r)
	case "/nodeState":
		s.nodeState(w, r)
	case "/serverPort":
		s.serverPort(w, r)
	case "/clientPort":
		s.clientPort(w, r)
	case "/dialState":
		s.dialState(w, r)
	case "/proxy":
		s.proxy(w, r)
	default:
		w.WriteHeader(404)
	}
}

//just return 0 if form value is empty string.
func intFormValue(r *http.Request, key string) (intVal int) {
	val := r.FormValue(key)
	intVal, _ = strconv.Atoi(val)
	return
}
