package stadis

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	tcpTimeOut     = 15 * time.Minute
	dialTimeOut    = 3 * time.Minute
	serverPortType = true
	clientPortType = false
)

type node interface {
	state() NodeState
	setState(state NodeState)
	String() string
}

type dataCenter struct {
	name    string
	rackMap map[string]*rack
	topo    *topology
	NodeState
}

func (dc *dataCenter) state() NodeState {
	return dc.NodeState
}

func (dc *dataCenter) setState(state NodeState) {
	if state.Latency == 0 {
		state.Latency = dc.NodeState.Latency
	}
	dc.NodeState = state
}

func (dc *dataCenter) String() string {
	var racks []*rack
	for _, rack := range dc.rackMap {
		racks = append(racks, rack)
	}
	return fmt.Sprintf("\nname:%v internalDown:%v externalDown:%v latency:%v racks:%v",
		dc.name, dc.InternalDown, dc.ExternalDown, dc.Latency, racks)
}

func newDc(confDC *DataCenter, topo *topology) (dc *dataCenter) {
	dc = new(dataCenter)
	dc.topo = topo
	dc.name = confDC.Name
	dc.rackMap = make(map[string]*rack)
	if confDC.NodeState != nil {
		dc.NodeState = *(confDC.NodeState)
	}
	for _, confRack := range confDC.Racks {
		if confRack.HostDefault == nil {
			confRack.HostDefault = confDC.HostDefault
		}
		if confRack.NodeState == nil {
			confRack.NodeState = confDC.RackDefault
		}
		rack := newRack(confRack, dc)
		dc.rackMap[rack.name] = rack
	}
	return
}

type rack struct {
	name       string
	hostMap    map[string]*host
	dataCenter *dataCenter
	NodeState
}

func (r *rack) state() NodeState {
	return r.NodeState
}
func (r *rack) setState(state NodeState) {
	if state.Latency == 0 {
		state.Latency = r.NodeState.Latency
	}
	r.NodeState = state
}

func (rack *rack) String() string {
	var hosts []*host
	for _, host := range rack.hostMap {
		hosts = append(hosts, host)
	}
	return fmt.Sprintf("\n\tname:%v internalDown:%v externalDown:%v latency:%v hosts:%v",
		rack.name, rack.InternalDown, rack.ExternalDown, rack.Latency, hosts)
}

func newRack(confRack *Rack, dc *dataCenter) (r *rack) {
	r = new(rack)
	r.dataCenter = dc
	r.name = confRack.Name
	r.hostMap = make(map[string]*host)
	if confRack.NodeState != nil {
		r.NodeState = *(confRack.NodeState)
	}
	for _, confHost := range confRack.Hosts {
		if confHost.NodeState == nil {
			confHost.NodeState = confRack.HostDefault
		}
		host := newHost(confHost, r)
		r.hostMap[host.name] = host
	}
	return
}

type host struct {
	name    string
	portMap map[int]bool //value is true for server port, false for client port.
	rack    *rack
	NodeState
}

func (host *host) state() NodeState {
	return host.NodeState
}

func (host *host) setState(state NodeState) {
	if state.Latency == 0 {
		state.Latency = host.NodeState.Latency
	}
	host.NodeState = state
}

func (host *host) String() string {
	var ports []int
	for port := range host.portMap {
		ports = append(ports, port)
	}
	return fmt.Sprintf("\n\t\tname:%v internalDown:%v externalDown:%v latency:%v ports:\n\t\t\t%v",
		host.name, host.InternalDown, host.ExternalDown, host.Latency, ports)
}

func newHost(confHost *Host, rack *rack) (h *host) {
	h = new(host)
	h.rack = rack
	h.name = confHost.Name
	h.portMap = make(map[int]bool)
	for _, port := range confHost.Ports {
		h.portMap[port] = serverPortType
		rack.dataCenter.topo.ports[port] = h
	}
	if confHost.NodeState != nil {
		h.NodeState = *(confHost.NodeState)
	}
	return
}

type topology struct {
	HostLatency   time.Duration
	RackLatency   time.Duration
	DcLatency     time.Duration
	ports         map[int]*host //ports to host map
	dataCenterMap map[string]*dataCenter
	mutex         sync.RWMutex
	updateCh      chan struct{}
}

func (topo *topology) String() (s string) {
	var dcs []*dataCenter
	for _, dc := range topo.dataCenterMap {
		dcs = append(dcs, dc)
	}
	s = fmt.Sprint(dcs)
	return
}

//When a server port is added, the topology need to close update channel, and make a new one.
//So all the blocking request will get their new states.
//It's not necessary when adding client port, because adding a client port won't affect any other connections.
func (topo *topology) addServerPort(name string, port int) (err error) {
	topo.mutex.Lock()
	defer topo.mutex.Unlock()
	host, err := topo.lookupHost(name)
	if err != nil {
		log.Println(err)
		return err
	}
	if _, ok := host.portMap[port]; ok {
		err = errors.New("port has been taken already.")
		log.Println(err)
		return
	}
	host.portMap[port] = serverPortType
	topo.ports[port] = host

	close(topo.updateCh)
	topo.updateCh = make(chan struct{})
	return nil
}

func (topo *topology) removeServerPort(name string, port int) (err error) {
	topo.mutex.Lock()
	defer topo.mutex.Unlock()
	host, err := topo.lookupHost(name)
	if err != nil {
		log.Println(err)
		return
	}
	portType, ok := host.portMap[port]
	if !ok {
		err = errors.New("port hasn't been registered")
		log.Println(err)
		return
	}
	if portType != serverPortType {
		err = errors.New("port isn't registered as server")
		log.Println(err)
		return
	}

	delete(host.portMap, port)
	close(topo.updateCh)
	topo.updateCh = make(chan struct{})
	return nil
}

func (topo *topology) addClientPort(name string, port int) (err error) {
	topo.mutex.Lock()
	defer topo.mutex.Unlock()
	host, err := topo.lookupHost(name)
	if err != nil {
		log.Println(err)
		return
	}
	if _, ok := host.portMap[port]; ok {
		err = errors.New("port has been taken already.")
		log.Println(err)
		return
	}
	host.portMap[port] = clientPortType
	topo.ports[port] = host
	return
}

func (topo *topology) removeClientPort(port int) (err error) {
	topo.mutex.Lock()
	defer topo.mutex.Unlock()
	host := topo.ports[port]
	if host == nil {
		err = errors.New("unknown port")
		log.Println(err)
		return
	}
	portType, ok := host.portMap[port]
	if !ok {
		err = errors.New("port hasn't been registered")
		log.Println(err)
		return
	}
	if portType != clientPortType {
		err = errors.New("port isn't registered as client port")
		log.Println(err)
		return
	}
	delete(host.portMap, port)
	delete(topo.ports, port)
	return
}

func (topo *topology) setNodeState(nodeName string, newState NodeState) (err error) {
	topo.mutex.Lock()
	defer topo.mutex.Unlock()
	node, err := topo.lookup(nodeName)
	if err != nil {
		log.Println(err)
		return
	}
	node.setState(newState)
	close(topo.updateCh)
	topo.updateCh = make(chan struct{})
	return
}

func (topo *topology) nodeState(nodeName string) (nodeState NodeState, err error) {
	topo.mutex.RLock()
	defer topo.mutex.RUnlock()

	node, err := topo.lookup(nodeName)
	if err != nil {
		log.Println(err)
		return
	}
	nodeState = node.state()
	return
}

func (topo *topology) dialState(clientName string, serverPort int) (connState ConnState, err error) {
	topo.mutex.RLock()
	defer topo.mutex.RUnlock()
	clientHost, err := topo.lookupHost(clientName)
	if err != nil {
		log.Println(err)
		return
	}

	serverHost := topo.ports[serverPort]
	if serverHost == nil {
		err = errors.New("host undefined for address " + strconv.Itoa(serverPort))
		log.Println(err)
		return
	}

	networkOk, latency := computeNetworkState(clientHost, serverHost)

	if networkOk {
		_, connState.OK = serverHost.portMap[serverPort]
		connState.Latency = latency * 2 //dial has double latency.
	} else {
		connState.OK = false
		connState.Latency = dialTimeOut
	}
	return
}


//compute the state of the connection based on entire network state.
func (topo *topology) connState(clientPort, serverPort int) (connState ConnState, err error) {
	topo.mutex.RLock()
	defer topo.mutex.RUnlock()
	clientHost := topo.ports[clientPort]
	if clientHost == nil {
		err = fmt.Errorf("connState:unknown client port %d", clientPort)
		return
	}
	serverHost := topo.ports[serverPort]
	if serverHost == nil {
		err = fmt.Errorf("connState:unknown server port %d", serverPort)
		return
	}

	_, ok := clientHost.portMap[clientPort]
	if !ok {
		return
	}

	networkOk, latency := computeNetworkState(clientHost, serverHost)

	if networkOk {
		_, connState.OK = serverHost.portMap[serverPort]
		connState.Latency = latency
	} else {
		connState.OK = false
		connState.Latency = tcpTimeOut
	}
	return
}

//compute network connection state between client and server host, no ports involved.
func computeNetworkState(clientHost, serverHost *host) (ok bool, latency time.Duration) {
	clientSideDown := clientHost.InternalDown
	serverSideDown := serverHost.InternalDown
	if clientHost != serverHost {
		clientSideDown = clientSideDown || clientHost.ExternalDown || clientHost.rack.InternalDown
		serverSideDown = serverSideDown || serverHost.ExternalDown || serverHost.rack.InternalDown
		latency += clientHost.Latency + serverHost.Latency
		if clientHost.rack != serverHost.rack {
			clientSideDown = clientSideDown || clientHost.rack.ExternalDown || clientHost.rack.dataCenter.InternalDown
			serverSideDown = serverSideDown || serverHost.rack.ExternalDown || serverHost.rack.dataCenter.InternalDown
			latency += clientHost.rack.Latency + serverHost.rack.Latency
			if clientHost.rack.dataCenter != serverHost.rack.dataCenter {
				clientSideDown = clientSideDown || clientHost.rack.dataCenter.ExternalDown
				serverSideDown = serverSideDown || serverHost.rack.dataCenter.ExternalDown
				latency += clientHost.rack.dataCenter.Latency + serverHost.rack.dataCenter.Latency
			}
		}
	}
	down := clientSideDown || serverSideDown
	ok = !down
	return
}

func newTopology(configReader io.Reader) (topo *topology, err error) {
	decoder := json.NewDecoder(configReader)
	config := new(Config)
	err = decoder.Decode(config)
	if err != nil {
		log.Println(err)
		return
	}

	topo = new(topology)
	topo.ports = make(map[int]*host)
	topo.dataCenterMap = make(map[string]*dataCenter)
	topo.updateCh = make(chan struct{})
	for _, confDC := range config.DataCenters {
		if confDC.RackDefault == nil {
			confDC.RackDefault = config.RackDefault
		}
		if confDC.HostDefault == nil {
			confDC.HostDefault = config.HostDefault
		}
		if confDC.NodeState == nil {
			confDC.NodeState = config.DcDefault
		}
		dc := newDc(confDC, topo)
		topo.dataCenterMap[dc.name] = dc
	}
	return
}

func (topo *topology) lookup(fullName string) (nod node, err error) {
	parts := strings.Split(fullName, ".")
	dc := topo.dataCenterMap[parts[0]]
	if dc == nil {
		err = errors.New("undefined dataCenter name," + parts[0])
		log.Println(err)
		return
	}
	if len(parts) < 2 {
		nod = dc
		return
	}
	rack := dc.rackMap[parts[1]]
	if rack == nil {
		err = errors.New("undefined rack name," + parts[1])
		log.Println(err)
		return
	}
	if len(parts) < 3 {
		nod = rack
		return
	}
	host := rack.hostMap[parts[2]]
	if host == nil {
		err = errors.New("undefined host name," + parts[2])
		log.Println(err)
		return
	}
	nod = host
	return
}

func (topo *topology) lookupHost(hostName string) (ho *host, err error) {
	node, err := topo.lookup(hostName)
	if err != nil {
		log.Println(err)
		return
	}
	ho, ok := node.(*host)
	if !ok {
		err = errors.New("invalid host name")
		log.Println(err)
	}
	return
}

func (topo *topology) getUpdateChannel() (updateCh chan struct{}) {
	topo.mutex.RLock()
	updateCh = topo.updateCh
	topo.mutex.RUnlock()
	return
}

func parsePort(port string) (portNum int, passive bool, err error) {
	portNum, err = strconv.Atoi(port)
	if err != nil {
		log.Println(err)
		return
	}
	if portNum < 0 {
		portNum = -portNum
		passive = true
	}
	return
}
