package main

import (
	"flag"
	"github.com/coocood/stadis"
	"net/http"
)

var port = flag.String("port", "", "the port to listen on")

func main() {
	flag.Parse()
	if *port != "" {
		stadis.Cli.ApiAddr = "localhost:" + *port
	}
	http.ListenAndServe(stadis.Cli.ApiAddr, stadis.NewApiServer())
}
