##Stadis - Stand-alone Distributed System

The easiest way to learn, develop and test a distributed system.

[![GoDoc](https://godoc.org/github.com/coocood/stadis?status.png)](https://godoc.org/github.com/coocood/stadis)
[![Build Status](https://travis-ci.org/coocood/stadis.png?branch=master)](https://travis-ci.org/coocood/stadis)


##Why should I use it?

Testing distributed system is very expensive.

It takes a large amount of resources, takes a lot of time to deploy and config.

More importantly, as your application is running on multiple machine,
it's nearly impossible to coordinate the network state with your application state.
as a result, there will be lots of error handling code leave untested,
which can cause serious problem once that actually happen.

With stadis, you can run a multi-data-center distributed system on a single machine.

You can change the topology and node state at any time by a single API call.
Everything happens exactly the way you want.
Every network error handling code can be easily covered.

##Get started

Require Go 1.2+ installed.

Stadis can be used as a library in Go application.

For applications written in other programming language, stadis can be used as a proxy server.

###Use stadis as a library in Go program.

Hello world example

    package main

    import (
    	"fmt"
    	"github.com/coocood/stadis"
    	"io"
    	"log"
    	"net/http"
    	"os"
    	"time"
    )

    func main() {
    	//Start a api server.
    	go http.ListenAndServe(stadis.Cli.ApiAddr, stadis.NewApiServer())
    	//Setup the http client dialer.
    	http.DefaultTransport.(*http.Transport).Dial = stadis.NewDialFunc("matter.metal.gold", 5*time.Second)

    	eagleListener, err := stadis.Listen("tcp", "localhost:8585", "animal.air.eagle")
    	if err != nil {
    		log.Fatal(err)
    	}

    	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request){
    		w.Write([]byte("hello\n"))
    	})

    	go http.Serve(eagleListener, nil)

    	time.Sleep(time.Millisecond)
    	before := time.Now()
    	resp, err := http.Get("http://localhost:8585/")
    	if err != nil {
    		log.Fatal(err)
    	}
    	io.Copy(os.Stdout, resp.Body)
    	resp.Body.Close()
    	//the latency should be a little more than 888ms which is define at stadis.DefaultConfig.
    	fmt.Println("latency:", time.Now().Sub(before))
    }

###Run stadis as a API/proxy server.

Build and Run

    go get github.com/coocood/stadis
    go build github.com/coocood/stadis/stadiserver
    ./stadiserver

Now, the stadis API server is started at port `8989`.

Then start a trivial http server as the origin server on port `12345` by run

    go run $GOROOT/src/pkg/net/http/triv.go

Then call the REST API to open a proxy

    curl -X POST 'http://localhost:8989/proxy?clientName=matter.metal.gold&proxyName=animal.air.eagle&proxyPort=8586&originAddr=localhost:12345'

We can ask the API server for the dial state:

    curl 'http://localhost:8989/dialState?clientName=matter.metal.gold&serverPort=8586'

You will get dial state like `{"Latency":444000000,"OK":true}`, the latency unit is nano second.
Normally you should sleep for that amount of time in your program before actually dial the proxy server.
If 'OK' is 'false' you should return error or throw an exception.

Then request the proxy server and record the time.

    time curl -i 'http://localhost:8586/counter'

The real time should be a little more than 444ms, which is the roundtrip time from 'matter.metal.gold' to 'animal.air.eagle'.

##Configuration
Stadis server does not use any config file, it starts with a default configuration, and you can update it by REST API call.

Stadis API server maintains a virtual topology which has three level:'DataCenter', 'Rack' and 'Host'.
All of them has a 'NodeState' of attributes 'Name', 'Latency', 'InternalDown' and 'ExternalDown' .
The topology contains multiple 'DataCenter', which contains multiple 'Rack', which in turn contains
multiple 'Host'.

We all know naming is hard, so I did the hard work for you.
After spent many hours, I managed to come up with 39 node names.
The topology has three data centers, each data center has three racks, each rack has three hosts.

    {
    	"DcDefault":{"Latency":100000000},
    	"RackDefault":{"Latency":10000000},
    	"HostDefault":{"Latency":1000000},
    	"DataCenters":[
    		{
    			"Name":"animal",
    			"Racks":[
    				{
    					"Name":"land",
    					"Hosts":[{"Name":"tiger"},{"Name":"lion"},{"Name":"wolf"}]
    				},
    				{
    					"Name":"sea",
    					"Hosts":[{"Name":"shark"},{"Name":"whale"},{"Name":"cod"}]
    				},
    				{
    					"Name":"air",
    					"Hosts":[{"Name":"eagle"},{"Name":"crow"},{"Name":"owl"}]
    				}
    			]
    		},
    		{
    			"Name":"plant",
    			"Racks":[
    				{
    					"Name":"fruit",
    					"Hosts":[{"Name":"apple"},{"Name":"pear"},{"Name":"grape"}]
    				},
    				{
    					"Name":"crop",
    					"Hosts":[{"Name":"corn"},{"Name":"rice"},{"Name":"wheat"}]
    				},
    				{
    					"Name":"flower",
    					"Hosts":[{"Name":"rose"},{"Name":"lily"},{"Name":"lotus"}]
    				}
    			]
    		},
    		{
    			"Name":"matter",
    			"Racks":[
    				{
    					"Name":"metal",
    					"Hosts":[{"Name":"gold"},{"Name":"silver"},{"Name":"iron"}]
    				},
    				{
    					"Name":"gem",
    					"Hosts":[{"Name":"ruby"},{"Name":"ivory"},{"Name":"pearl"}]
    				},
    				{
    					"Name":"liquid",
    					"Hosts":[{"Name":"water"},{"Name":"oil"},{"Name":"wine"}]
    				}
    			]
    		}
    	]
    }

In default configuration, each data center has 100ms latency, each rack has 10ms latency, each host has 1ms latency.

So the latency from 'matter.metal.gold' to 'animal.air.eagle' should be "1ms+10ms+100ms+100ms+10ms+1ms = 222ms"

The round trip time should be 444ms, so the total time to create a connection
and then make a http request from 'gold' to 'eagle' should be a little more than 888ms.

##REST API

- Update configuration with json payload like the default config shown above.

        POST /config


- Update node state with json body like `{"Latency":10000000,"InternalDown":false,"ExternalDown":true}`

        POST /nodeState?name=%s

    The 'name' parameter is the node name in the topology, e.g. "animal.air.eagle".


- Get node state:

        GET /nodeState?name=%s


- Start a proxy:

        POST /proxy?clientName=%s&proxyName=%s&proxyPort=%s&originAddr=%s

    'clientName' defines where the client process is located in the topology.
    'proxyName' defines where the proxy is located in the topology.
    'proxyPort' will be opened and accepts incoming connection.
    'originAddr' is the origin server address for the proxy, it can be an address in another machine,
    eg. `192.168.1.100:12345`, so if you can not run the origin server on your localhost, you can proxy it.


- Stop a proxy:

        DELETE /proxy?proxyPort=%s


- Get dial state from a client to server.

        GET /dialState?clientName={clientName}&serverPort={serverPort}

    The response is a json object like  `{"Latency":10000000,"OK":true}`


- Get the current connection state after that connection has been created.

        GET /connState?clientPort={clientPort}&serverPort={serverPort}

##Performance

Stadis adds an extra layer on top of tcp connection, the throughput is greatly decreased.
On my laptop, a proxy connection with 1ms latency gets about 30MB/s throughput.
I think it is sufficient for most of applications for testing purpose.
Higher latency gets lower throughput which is pretty much the way raw connections work.

##LICENSE

The MIT License
