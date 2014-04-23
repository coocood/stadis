package stadis

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"testing"
	"time"
)

const (
	appleHostName = "plant.fruit.apple"
	lionHostName  = "animal.land.lion"
	tigerHostName = "animal.land.tiger"
)

var defaultServer = NewApiServer()

func init() {
	log.SetFlags(log.Lshortfile)
	go http.ListenAndServe(Cli.ApiAddr, defaultServer)
	time.Sleep(time.Millisecond)
}

func resetDefaultServer() (err error) {
	err = Cli.UpdateConfig(bytes.NewReader(DefaultConfig))
	if err != nil {
		log.Println(err)
		return
	}
	return
}

func TestApi(t *testing.T) {
	err := resetDefaultServer()
	if err != nil {
		t.Fatal(err)
	}

	lionPort := "30001"
	Cli.UpdateNodeState("animal.land", NodeState{ExternalDown: true})
	Cli.ServerStarted(lionHostName, lionPort)

	appleDialLion, _ := Cli.DialState(appleHostName, lionPort)
	expectedState := ConnState{
		Latency: 3 * time.Minute,
	}
	if expectedState != appleDialLion {
		t.Fatal("wrong state for apple dial lion, expected", expectedState, "actual", appleDialLion)
	}

	applePort := "30003"
	Cli.ServerStarted(appleHostName, applePort)

	appleDialApple, _ := Cli.DialState(appleHostName, applePort)

	expectedState = ConnState{
		OK:      true,
		Latency: 0,
	}
	if expectedState != appleDialApple {
		t.Fatal("latency for connection between processes on the same host should be 0, expected",
			expectedState, "actual ", appleDialApple)
	}

	tigerPort := "30011"
	Cli.ServerStarted(tigerHostName, tigerPort)
	Cli.UpdateNodeState("animal.land", NodeState{ExternalDown: false})

	appleDialTiger, _ := Cli.DialState(appleHostName, tigerPort)

	expectedState = ConnState{
		OK:      true,
		Latency: 2 * (1 + 10 + 100 + 100 + 10 + 1) * time.Millisecond,
	}
	if expectedState != appleDialTiger {
		defaultServer.DumpNode("")
		t.Fatal("latency should be two times the sum of latency of every node along the path, expected",
			expectedState, "actual", appleDialTiger)
	}

	Cli.UpdateNodeState(appleHostName, NodeState{Latency: 30 * time.Millisecond})
	appleDialTiger, _ = Cli.DialState(appleHostName, tigerPort)

	expectedState = ConnState{
		OK:      true,
		Latency: 2 * (30 + 10 + 100 + 100 + 10 + 1) * time.Millisecond,
	}
	if expectedState != appleDialTiger {
		t.Fatal("wrong state for apple dial tiger, expected", expectedState, "actual", appleDialTiger)
	}

	Cli.UpdateNodeState("plant.fruit", NodeState{ExternalDown: true})
	appleDialTiger, _ = Cli.DialState(appleHostName, tigerPort)

	expectedState = ConnState{
		OK:      false,
		Latency: 3 * time.Minute,
	}
	if appleDialTiger != expectedState {
		defaultServer.DumpNode("")
		t.Fatal("wrong state for apple dial tiger, expected", expectedState, "actual", appleDialTiger)
	}

	//set NodeState to internal down, the expected state will stay the same.
	Cli.UpdateNodeState("plant.fruit", NodeState{InternalDown: true})

	appleDialTiger, _ = Cli.DialState(appleHostName, tigerPort)
	if appleDialTiger != expectedState {
		t.Fatal("wrong state for apple dial tiger, expected", expectedState, "actual", appleDialTiger)
	}

	Cli.UpdateNodeState("plant.fruit", NodeState{})
	appleDialTiger, _ = Cli.DialState(appleHostName, tigerPort)
	if appleDialTiger.OK != true {
		t.Fatal("apple dial tiger should be ok.")
	}

	Cli.UpdateNodeState("plant", NodeState{InternalDown: true})

	appleDialTiger, _ = Cli.DialState(appleHostName, tigerPort)
	if appleDialTiger.OK != false {
		t.Fatal("apple dial tiger should not be ok when plant internal is down.")
	}

	Cli.UpdateNodeState("plant", NodeState{ExternalDown: true})
	appleDialTiger, _ = Cli.DialState(appleHostName, tigerPort)
	if appleDialTiger.OK != false {
		t.Fatal("apple dial tiger should not be ok when plant external is down.")
	}

	Cli.UpdateNodeState("plant", NodeState{})
	Cli.UpdateNodeState("animal", NodeState{ExternalDown: true})
	appleDialTiger, _ = Cli.DialState(appleHostName, tigerPort)
	if appleDialTiger.OK != false {
		t.Fatal("apple dial tiger should not be ok when animal external is down.")
	}

	Cli.UpdateNodeState("animal", NodeState{InternalDown: true})
	appleDialTiger, _ = Cli.DialState(appleHostName, tigerPort)
	if appleDialTiger.OK != false {
		t.Fatal("apple dial tiger should not be ok when animal internal is down.")
	}

	Cli.UpdateNodeState("animal", NodeState{})
	Cli.UpdateNodeState("animal.land", NodeState{ExternalDown: true})
	appleDialTiger, _ = Cli.DialState(appleHostName, tigerPort)
	if appleDialTiger.OK != false {
		t.Fatal("apple dial tiger should not be ok when animal.land external is down.")
	}

	Cli.UpdateNodeState("animal.land", NodeState{InternalDown: true})
	appleDialTiger, _ = Cli.DialState(appleHostName, tigerPort)
	if appleDialTiger.OK != false {
		t.Fatal("apple dial tiger should not be ok when animal.land internal is down.")
	}

	Cli.UpdateNodeState("animal.land", NodeState{})
	Cli.UpdateNodeState(tigerHostName, NodeState{ExternalDown: true})
	appleDialTiger, _ = Cli.DialState(appleHostName, tigerPort)
	if appleDialTiger.OK != false {
		t.Fatal("apple dial tiger should not be ok when tiger external is down.")
	}

	Cli.UpdateNodeState(tigerHostName, NodeState{InternalDown: true})
	appleDialTiger, _ = Cli.DialState(appleHostName, tigerPort)
	if appleDialTiger.OK != false {
		t.Fatal("apple dial tiger should not be ok when tiger internal is down.")
	}
	if appleDialTiger.Latency != 3*time.Minute {
		t.Fatal("apple dial tiger's latency should be 15 minutes.")
	}

	Cli.UpdateNodeState(tigerHostName, NodeState{})
	appleDialTiger, _ = Cli.DialState(appleHostName, tigerPort)
	if appleDialTiger.OK != true {
		defaultServer.DumpNode("")
		t.Fatal("apple dial tiger should be ok.")
	}

	err = Cli.ServerStopped(tigerHostName, tigerPort)
	if err != nil {
		t.Fatal(err)
	}
	appleDialTiger, _ = Cli.DialState(appleHostName, tigerPort)
	expectedState = ConnState{
		OK:      false,
		Latency: 2 * (30 + 10 + 100 + 100 + 10 + 1) * time.Millisecond,
	}
	if appleDialTiger != expectedState {
		defaultServer.DumpNode("")
		t.Fatal("apple dial tiger latency should be ", expectedState, "actual", appleDialTiger)
	}
}

func TestLatency(t *testing.T) {
	err := resetDefaultServer()
	if err != nil {
		t.Fatal(err)
	}
	appleHostName := "plant.fruit.apple"
	applePort := "30003"
	appleListener, err := net.Listen("tcp", "localhost:"+applePort)
	if err != nil {
		t.Fatal(err)
	}

	mListener, err := NewListener(appleListener, appleHostName)
	if err != nil {
		t.Fatal(err)
	}
	go echoServe(mListener)

	tigerHostName := "animal.land.tiger"
	tigerDialFunc := NewDialFunc(tigerHostName, 0)
	tigerConn, err := tigerDialFunc("tcp", "localhost:30003")
	if err != nil {
		t.Fatal(err)
	}
	defer tigerConn.Close()
	tigerToAppleState, _ := Cli.ConnState(localPort(tigerConn), applePort, nil)
	buf := make([]byte, 4096)
	before := time.Now()
	tigerConn.Write(buf)
	tigerConn.Write(buf)
	_, err = tigerConn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	tigerConn.Read(buf)
	latency := time.Now().Sub(before)
	if latency < tigerToAppleState.Latency*2 || latency > tigerToAppleState.Latency*3 {
		defaultServer.DumpNode("")
		t.Fatal("expected latency", tigerToAppleState.Latency*2, "actual", latency)
	}
}

func TestProxy(t *testing.T) {
	originAddr := "localhost:6546"
	originListener, err := net.Listen("tcp", originAddr)
	if err != nil {
		t.Fatal(err)
	}
	go echoServe(originListener)
	resetDefaultServer()

	proxyName := "matter.metal.gold"
	proxyPort := "6577"
	localName := "animal.air.eagle"
	err = Cli.StartProxy(localName, proxyName, proxyPort, originAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer Cli.StopProxy(proxyPort)
	testConn, err := net.DialTimeout("tcp", "localhost:"+proxyPort, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	before := time.Now()
	writeData := bytes.Repeat([]byte("abcdefghi"), 1000)
	testConn.Write(writeData)
	readData, err := ioutil.ReadAll(io.LimitReader(testConn, int64(len(writeData))))
	if err != nil {
		t.Fatal(err)
	}
	duration := time.Now().Sub(before)
	t.Log(duration)
	if duration < time.Millisecond*444 || duration > time.Millisecond*555 {
		t.Error("wrong latency, expected", 444*time.Millisecond, "actual", duration)
		defaultServer.DumpNode("")
	}

	if !bytes.Equal(readData, writeData) {
		t.Error("read data should be equal to written data")
	}
	err = testConn.Close()
	if err != nil {
		t.Fatal(err)
	}

	//	change proxy's client name.
	newLocalName := "matter.metal.gold"
	err = Cli.UpdateProxy(newLocalName, proxyPort)
	if err != nil {
		t.Fatal(err)
	}

	newConn, err := net.Dial("tcp", "localhost:"+proxyPort)
	if err != nil {
		t.Fatal(err)
	}
	defer newConn.Close()
	before = time.Now()
	newConn.Write(writeData)
	_, err = io.ReadFull(newConn, readData)
	if err != nil {
		t.Fatal(err)
	}
	duration = time.Now().Sub(before)
	t.Log(duration)
	if duration > time.Millisecond*10 {
		t.Error("wrong latency, expected less than", 10*time.Millisecond, "actual", duration)
		defaultServer.DumpNode("")
	}

	if !bytes.Equal(readData, writeData) {
		t.Error("read data should be equal to wrttien data")
	}
}

func echoServe(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go func() {
			n, err := io.Copy(conn, conn)
			if err != nil {
				log.Println(n, err)
			}
			conn.Close()
		}()
	}
}
