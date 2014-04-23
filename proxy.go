package stadis

import (
	"io"
	"log"
	"net"
	"sync"
	"time"
)

type proxyServer struct {
	mu         sync.RWMutex
	clientName string
	proxyName  string
	proxyPort  string
	originAddr string
	listener   net.Listener
}

func newProxyServer(clientName, proxyName, proxyPort, originAddr string) (ps *proxyServer, err error) {
	ps = new(proxyServer)
	ps.clientName = clientName
	ps.proxyName = proxyName
	ps.proxyPort = proxyPort
	ps.originAddr = originAddr
	ps.listener, err = net.Listen("tcp", "localhost:"+ps.proxyPort)
	if err != nil {
		log.Println("failed to listen proxy", err)
		return
	}
	err = Cli.ServerStarted(ps.proxyName, ps.proxyPort)
	if err != nil {
		log.Println("at newProxyServer", err)
		return
	}
	return
}

func (ps *proxyServer) serve() {
	for {
		downstream, err := ps.listener.Accept()
		if err != nil {
			return
		}

		originConn, err := net.DialTimeout("tcp", ps.originAddr, time.Second)
		if err != nil {
			log.Println(err)
			downstream.Close()
			return
		}
		clientPort := remotePort(downstream)
		err = Cli.ClientConnected(ps.getClientName(), clientPort)
		if err != nil {
			log.Printf("%v, clinetPort:%s\n", err, clientPort)
			return
		}

		//the delay and failing happens on this upstream conn.
		upstream, err := newConnection(originConn, clientPort, ps.proxyPort)
		if err != nil {
			log.Println(err)
			downstream.Close()
			originConn.Close()
			return
		}
		go handleCopy(downstream, upstream)
	}
}

func (ps *proxyServer) close() (err error) {
	ps.listener.Close()
	err = Cli.ServerStopped(ps.proxyName, ps.proxyPort)
	return
}

func (ps *proxyServer) setClientName(clientName string) {
	ps.mu.Lock()
	ps.clientName = clientName
	ps.mu.Unlock()
}

func (ps *proxyServer) getClientName() (clientName string) {
	ps.mu.RLock()
	clientName = ps.clientName
	ps.mu.RUnlock()
	return
}

func handleCopy(downstream, upstream net.Conn) {
	done := make(chan bool)
	go func() {
		n, err := io.Copy(downstream, upstream)
		if err != nil {
			log.Println(n, err)
		}
		done <- true
	}()
	n, err := io.Copy(upstream, downstream)
	if err != nil {
		log.Println(n, err)
	}
	<-done
	upstream.Close()
	downstream.Close()
	err = Cli.ClientDisconnected(remotePort(downstream))
	if err != nil {
		log.Println(err)
	}
}
