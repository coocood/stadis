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
