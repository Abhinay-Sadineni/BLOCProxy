package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"local/Abhinay/controllercomm"
	"local/Abhinay/globals"
	"local/Abhinay/internal/incoming"
	"local/Abhinay/internal/loadbalancer"
	"local/Abhinay/internal/outgoing"
	"github.com/gorilla/mux"
)

func main() {
	fmt.Println("Hello world")
	globals.RedirectUrl_g = "http://localhost" + globals.CLIENTPORT
	fmt.Println("Input Port", globals.PROXYINPORT)
	fmt.Println("Output Port", globals.PROXOUTPORT)
	fmt.Println("redirecting to:", globals.RedirectUrl_g)
	fmt.Println("User ID:", os.Getuid())
	fmt.Println("Below loadbalencer")


	loadbalancer.DefaultLBPolicy_g = os.Getenv("LBPolicy")
	if loadbalancer.DefaultLBPolicy_g == "MLeastConn" || loadbalancer.DefaultLBPolicy_g == "Netflix"  {
		globals.NumRetries_g, _ = strconv.Atoi(os.Getenv("RETRIES"))
		// get capacity
		// globals.Capacity_g, _ = strconv.ParseFloat(os.Getenv("CAPACITY"), 64)
		globals.Capacity_g, _ = strconv.ParseInt(os.Getenv("CAPACITY"), 10, 64)
		
	} else {
		globals.NumRetries_g = 1
		globals.Capacity_g = 0
	}
	reset, _ := strconv.Atoi(os.Getenv("RESET"))
	globals.ResetInterval_g = time.Duration(reset) * time.Microsecond

	// capacity has been set in the env; do not reset
	if globals.Capacity_g != 0 {
		incoming.RunAvg_g = false
	}

	// incoming request handling
	proxy := incoming.NewProxy(globals.RedirectUrl_g)
	inMux := mux.NewRouter()
	inMux.PathPrefix("/").HandlerFunc(proxy.Handle)

	// outgoing request handling
	outMux := mux.NewRouter()
	outMux.PathPrefix("/").HandlerFunc(outgoing.HandleOutgoing)

	// start running the communication server
	done := make(chan bool)
	defer close(done)
	go controllercomm.RunComm(done)

	// start the proxy services
	go func() {
		log.Fatal(http.ListenAndServe(globals.PROXYINPORT, inMux))
	}()
	log.Fatal(http.ListenAndServe(globals.PROXOUTPORT, outMux))
}
