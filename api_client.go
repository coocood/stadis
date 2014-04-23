package stadis

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
)

//Default API client
var Cli = &ApiClient{ApiAddr: "localhost:8989"}

var httpClient = &http.Client{Transport: &http.Transport{}}

type ApiClient struct {
	ApiAddr string
}

//Register the server port on API server.
//Should be called after open a listener.
func (client *ApiClient) ServerStarted(name, port string) error {
	return client.serverPort("POST", name, port)
}

//Unregister the server port on API server.
//Should be called after closed a listener.
func (client *ApiClient) ServerStopped(name, port string) error {
	return client.serverPort("DELETE", name, port)
}

func (client *ApiClient) serverPort(method, name, port string) (err error) {
	url := fmt.Sprintf("http://%v/serverPort?name=%v&port=%v", client.ApiAddr, name, port)
	req, _ := http.NewRequest(method, url, nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Println(err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		err = errorFromResponse(resp)
		log.Println(err)
		return
	}
	return
}


//Register client port at API server.
//Should be called after client created a connection.
func (client *ApiClient) ClientConnected(name, port string) error {
	return client.clientPort("POST", name, port)
}

//Unregister client port at API server.
//Should be called after client closed a connection.
func (client *ApiClient) ClientDisconnected(port string) error {
	return client.clientPort("DELETE", "", port)
}

func (client *ApiClient) clientPort(method, name, port string) (err error) {
	url := fmt.Sprintf("http://%v/clientPort?name=%v&port=%v",
		client.ApiAddr, name, port)
	req, _ := http.NewRequest(method, url, nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Println(err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		err = errorFromResponse(resp)
		log.Println(err)
		return
	}
	return
}

//Get the dial state which can be used to simulate network latency or failure before actually dial the server.
func (client *ApiClient) DialState(clientName, serverPort string) (state ConnState, err error) {
	url := fmt.Sprintf("http://%v/dialState?clientName=%v&serverPort=%v", client.ApiAddr, clientName, serverPort)
	resp, err := httpClient.Get(url)
	if err != nil {
		log.Println(err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		err = errorFromResponse(resp)
		log.Println(err)
		return
	}
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
		return
	}
	err = json.Unmarshal(data, &state)
	if err != nil {
		log.Println(err)
		return
	}
	return
}

//Get the current connection state for the connection between 'clientPort' and 'serverPort'.
//If 'oldState' is provided, this request will do long-polling, blocking for a few seconds
//before get response if there is no new state updated.
func (client *ApiClient) ConnState(clientPort, serverPort string, oldState *ConnState) (state *ConnState, err error) {
	url := fmt.Sprintf("http://%v/connState?clientPort=%v&serverPort=%v", client.ApiAddr, clientPort, serverPort)
	req, _ := http.NewRequest("GET", url, nil)
	if oldState != nil {
		jsonBytes, _ := json.Marshal(oldState)
		req.Header.Add("If-None-Match", string(jsonBytes))
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Println(err)
		return
	}
	if resp.StatusCode == 304 {
		state = oldState
		return
	}
	if resp.StatusCode != 200 {
		err = errorFromResponse(resp)
		log.Println(err)
		return
	}
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
		return
	}
	state = new(ConnState)
	err = json.Unmarshal(data, state)
	if err != nil {
		log.Println(err)
		return
	}
	return
}

//Get the node state by node name
func (client *ApiClient) NodeState(name string) (nodeState NodeState, err error) {
	url := fmt.Sprintf("http://%v/nodeState?name=%v", client.ApiAddr, name)
	resp, err := httpClient.Get(url)
	if err != nil {
		log.Println(err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		err = errorFromResponse(resp)
		log.Println(err)
		return
	}
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
		return
	}
	err = json.Unmarshal(data, &nodeState)
	if err != nil {
		log.Println(err)
		return
	}
	return
}

//Set the node's state, the name can be dc, rack or host depends on the number of dot in the name.
//If latency of the nodeState is zero, the target node's latency will stay unchanged.
func (client *ApiClient) UpdateNodeState(name string, nodeState NodeState) (err error) {
	url := fmt.Sprintf("http://%v/nodeState?name=%v", client.ApiAddr, name)
	jsonData, _ := json.Marshal(nodeState)
	resp, err := httpClient.Post(url, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		log.Println(err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		err = errorFromResponse(resp)
		log.Println(err)
		return
	}
	return
}

//Update the API server config, the topology on API server will be rebuild.
//You can use the 'DefaultConfig' as a base config, then do some modification to meet your requirement.
func (client *ApiClient) UpdateConfig(reader io.Reader) (err error) {
	url := fmt.Sprintf("http://%v/config", client.ApiAddr)
	resp, err := httpClient.Post(url, "application/json", reader)
	if err != nil {
		log.Println(err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		err = errorFromResponse(resp)
		log.Println(err)
		return
	}
	return
}

//Start a proxy server in API server process.
//The 'clientName' going to be used to register a client port at API server when the proxy server accepts a new connection,
//For example, if you pass 'matter.metal.gold' as clientName, every client connected to the proxy server will be considered
//located at 'matter.metal.gold'.
//This can be updated after the proxy is open, but only one 'clientName' can be used by a proxy server at a time.
func (client *ApiClient) StartProxy(clientName, proxyName, proxyPort, originAddr string) (err error) {
	return client.proxy("POST", clientName, proxyName, proxyPort, originAddr)
}

//Update the 'clientName' for a proxy server, so future connection will be registered with the new name.
//This will not affect connections that have been registered already.
func (client *ApiClient) UpdateProxy(clientName, proxyPort string) (err error) {
	return client.proxy("PUT", clientName, "", proxyPort, "")
}

//Stop a proxy server
func (client *ApiClient) StopProxy(proxyPort string) (err error) {
	return client.proxy("DELETE", "", "", proxyPort, "")
}

func (client *ApiClient) proxy(method, clientName, proxyName, proxyPort, originAddr string) (err error) {
	url := fmt.Sprintf("http://%v/proxy?clientName=%s&proxyName=%s&proxyPort=%s&originAddr=%s",
		client.ApiAddr, clientName, proxyName, proxyPort, originAddr)
	req, _ := http.NewRequest(method, url, nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Println(err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		err = errorFromResponse(resp)
		log.Println(err)
		return
	}
	return
}

func errorFromResponse(resp *http.Response) error {
	bodyBytes := make([]byte, resp.ContentLength)
	io.ReadFull(resp.Body, bodyBytes)
	return errors.New(string(bodyBytes))
}
