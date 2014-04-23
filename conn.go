package stadis

import (
	"bytes"
	"errors"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

//Each packet is 4KB, so buffer size is 4KB*NumOfPackets which by default is 512KB,
//Larger buffer size gets more throughput, consume more memory.
var	NumOfPackets = 128

const packetSize   = 4 * 1024

type connection struct {
	conn          net.Conn
	connState     *ConnState
	readDeadline  time.Time
	writeDeadline time.Time
	mutex         sync.RWMutex
	writePacketCh chan *packet
	readPacketCh  chan *packet
	readBuffer    bytes.Buffer //handle the case when packet length is longer than read buf length
	readErr       error
	writeErrCh    chan error
	pool          packetPool
	closeCh       chan struct{}
	updateCh      chan struct{}
	updateErrCh   chan error
	oldState      *ConnState
	clientPort    string
	serverPort    string
}

type packet struct {
	data     []byte
	length   int
	sentTime int64
	err      error
}

type packetPool chan *packet

func (pp packetPool) get() (pack *packet) {
	select {
	case pack = <-pp:
	default:
		pack = new(packet)
		pack.data = make([]byte, packetSize)
	}
	return
}

func (pp packetPool) put(p *packet) {
	select {
	case pp <- p:
	default:
	}
}

func (c *connection) updateLoop() {
	for {
		select {
		case <-c.closeCh:
			return
		default:
			oldState := c.getState()
			newState, err := Cli.ConnState(c.clientPort, c.serverPort, oldState)
			if err != nil {
				log.Println(err)
				c.updateErrCh <- err
				return
			}
			if newState == oldState {
				continue
			}
			c.mutex.Lock()
			close(c.updateCh)
			c.updateCh = make(chan struct{})
			c.connState = newState
			c.mutex.Unlock()
		}

	}
}

func (c *connection) readLoop() {
	for {
		packet := c.pool.get()
		n, err := c.conn.Read(packet.data)
		packet.err = err
		packet.length = n
		packet.sentTime = time.Now().UnixNano()
		select {
		case <-c.closeCh:
			return
		case c.readPacketCh <- packet:
		}
	}
}

func (c *connection) readPacket(packet *packet, b []byte) (n int, err error) {
	for {
		now := time.Now().UnixNano()
		elapsed := now - packet.sentTime
		state := c.getState()
		remainedDelay := state.Latency - time.Duration(elapsed)
		select {
		case <-c.closeCh:
			err = errors.New("connection closed")
			return
		case <-time.After(remainedDelay):
			n = copy(b, packet.data[:packet.length])
			if packet.length <= len(b) {
				err = packet.err
				return
			} else {
				c.mutex.Lock()
				c.readBuffer.Write(packet.data[packet.length:])
				c.readErr = packet.err
				c.mutex.Unlock()
			}
			c.pool.put(packet)
			return
		case <-c.updateCh:
		}
	}
}

func (mc *connection) Read(b []byte) (n int, err error) {
	mc.mutex.Lock()
	n, _ = mc.readBuffer.Read(b)
	if n == 0 {
		err = mc.readErr
		mc.readErr = nil
	}
	mc.mutex.Unlock()
	if n > 0 {
		return
	} else if err != nil {
		return
	}
	var deadlineTimer <-chan time.Time
	mc.mutex.Lock()
	if !mc.readDeadline.IsZero() {
		deadlineTimer = time.After(mc.readDeadline.Sub(time.Now()))
	}
	mc.mutex.Unlock()
	select {
	case packet := <-mc.readPacketCh:
		n, err = mc.readPacket(packet, b)
	case err = <-mc.updateErrCh:
		log.Println(err)
	case <-mc.closeCh:
		err = errors.New("connection closed")
	case <-deadlineTimer:
		err = errors.New("read timeout")
	}
	return
}

func (c *connection) writeLoop() {
	for {
		select {
		case <-c.closeCh:
			return
		case packet := <-c.writePacketCh:
			c.writePacket(packet)
		}
	}
}
func (c *connection) writePacket(packet *packet) {
	for {
		now := time.Now().UnixNano()
		elapsed := now - packet.sentTime
		remainedLatency := c.connState.Latency - time.Duration(elapsed)
		select {
		case <-c.closeCh:
			return
		case <-c.updateCh:
			//latency has been updated by manager, it may be less than remained time to wait.
			//so we should recalculate it.
			continue
		case <-time.After(remainedLatency):
		}
		var err error
		if c.connState.OK {
			_, err = c.conn.Write(packet.data[:packet.length])
		} else {
			err = errors.New("connection error")
		}
		c.pool.put(packet)
		if err != nil {
			select {
			case c.writeErrCh <- err:
			default:
			}
		}
		return
	}
}

func (mc *connection) Write(b []byte) (n int, err error) {
	for n < len(b) {
		now := time.Now()
		packet := mc.pool.get()
		length := copy(packet.data, b[n:])
		packet.length = length
		packet.sentTime = now.UnixNano()
		var deadlineTimer <-chan time.Time
		mc.mutex.Lock()
		if !mc.writeDeadline.IsZero() {
			deadlineTimer = time.After(mc.writeDeadline.Sub(now))
		}
		mc.mutex.Unlock()
		select {
		case err = <-mc.updateErrCh:
			log.Println(err)
		case err = <-mc.writeErrCh:
		case <-deadlineTimer:
			err = errors.New("write timeout")
		case mc.writePacketCh <- packet:
			n += length
		case <-mc.closeCh:
			err = errors.New("connection closed")
		}
		if err != nil {
			return
		}
	}
	return
}

func (mc *connection) Close() error {
	close(mc.closeCh)
	return mc.conn.Close()
}

func (mc *connection) LocalAddr() net.Addr {
	return mc.conn.LocalAddr()
}
func (mc *connection) RemoteAddr() net.Addr {
	return mc.conn.RemoteAddr()
}
func (mc *connection) SetDeadline(t time.Time) (err error) {
	mc.mutex.Lock()
	mc.writeDeadline = t
	mc.readDeadline = t
	err = mc.conn.SetDeadline(t)
	mc.mutex.Unlock()
	return nil
}
func (mc *connection) SetReadDeadline(t time.Time) (err error) {
	mc.mutex.Lock()
	mc.readDeadline = t
	err = mc.conn.SetReadDeadline(t)
	mc.mutex.Unlock()
	return nil
}

func (mc *connection) SetWriteDeadline(t time.Time) (err error) {
	mc.mutex.Lock()
	mc.writeDeadline = t
	err = mc.conn.SetWriteDeadline(t)
	mc.mutex.Unlock()
	return nil
}

func (mc *connection) getState() (state *ConnState) {
	mc.mutex.RLock()
	state = mc.connState
	mc.mutex.RUnlock()
	return
}

func newConnection(conn net.Conn, clientPort, serverPort string) (mConn *connection, err error) {
	mConn = new(connection)
	mConn.conn = conn
	mConn.clientPort = clientPort
	mConn.serverPort = serverPort
	mConn.pool = make(packetPool, 10)
	mConn.writeErrCh = make(chan error, 1)
	mConn.writePacketCh = make(chan *packet, NumOfPackets)
	mConn.readPacketCh = make(chan *packet, NumOfPackets)
	mConn.updateCh = make(chan struct{})
	mConn.updateErrCh = make(chan error, 1)
	mConn.closeCh = make(chan struct{})

	connState, err := Cli.ConnState(clientPort, serverPort, nil)
	if err != nil {
		log.Println(err)
		return
	}
	mConn.connState = connState
	go mConn.writeLoop()
	go mConn.readLoop()
	go mConn.updateLoop()
	return
}

type dialer struct {
	clientName string
	timeout    time.Duration
}

func (d *dialer) dial(network, serverAddr string) (conn net.Conn, err error) {
	serverPort := serverAddr[strings.LastIndex(serverAddr, ":")+1:]
	state, err := Cli.DialState(d.clientName, serverPort)
	if err != nil {
		log.Println(err)
		return
	}
	select {
	case <-time.After(time.Duration(state.Latency)):
		if state.OK {
			var realConn net.Conn
			realConn, err = net.Dial(network, serverAddr)
			if err != nil {
				return
			}
			err = Cli.ClientConnected(d.clientName, localPort(realConn))
			if err != nil {
				log.Println(err)
				return
			}
			conn, err = newConnection(realConn, localPort(realConn), remotePort(realConn))
			if err != nil {
				log.Println(err)
				return
			}
		} else {
			err = errors.New("connection error")
		}
	case <-time.After(d.timeout):
		err = errors.New("dial timeout")
	}
	return
}

func NewDialFunc(clientName string, timeout time.Duration) func(network, addr string) (conn net.Conn, err error) {
	d := new(dialer)
	d.clientName = clientName
	if timeout == 0 {
		timeout = time.Minute * 3
	}
	d.timeout = timeout
	return d.dial
}

type listener struct {
	ol   net.Listener
	name string
}

func (l *listener) Accept() (mConn net.Conn, err error) {
	return l.ol.Accept()
}

func (l *listener) Close() (err error) {
	l.ol.Close()
	err = Cli.ServerStopped(l.name, listenerPort(l.ol))
	if err != nil {
		log.Println(err)
		return
	}
	return
}

func (l *listener) Addr() net.Addr {
	return l.ol.Addr()
}

func NewListener(ol net.Listener, name string) (l net.Listener, err error) {
	err = Cli.ServerStarted(name, listenerPort(ol))
	if err != nil {
		log.Println(err)
		return
	}
	l = &listener{ol, name}
	return
}

func Listen(network, addr, name string) (l net.Listener, err error) {
	originListener, err := net.Listen(network, addr)
	if err != nil {
		log.Println(err)
		return
	}
	l, err = NewListener(originListener, name)
	return
}

func listenerPort(l net.Listener) string {
	addr := l.Addr().String()
	return addr[strings.LastIndex(addr, ":")+1:]
}

func localPort(conn net.Conn) (port string) {
	addr := conn.LocalAddr().String()
	port = addr[strings.LastIndex(addr, ":")+1:]
	return
}

func remotePort(conn net.Conn) (port string) {
	addr := conn.RemoteAddr().String()
	port = addr[strings.LastIndex(addr, ":")+1:]
	return
}
