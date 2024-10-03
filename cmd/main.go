package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
	"strings"
	//"os/exec"
	//"github.com/Ank0708/MiCoProxy/controllercomm"
	"github.com/Ank0708/MiCoProxy/globals"
	"github.com/Ank0708/MiCoProxy/internal/incoming"
	"github.com/Ank0708/MiCoProxy/internal/loadbalancer"
	"github.com/Ank0708/MiCoProxy/internal/outgoing"
	"github.com/Ank0708/MiCoProxy/internal/rttmonitor"
	"github.com/gorilla/mux"
)

func main() {
	globals.RedirectUrl_g = "http://localhost:" + os.Getenv("CLIENTPORT")
	fmt.Println("Input Port", globals.PROXYINPORT)
	fmt.Println("Output Port", globals.PROXOUTPORT)
	fmt.Println("redirecting to:", globals.RedirectUrl_g)
	fmt.Println("User ID:", os.Getuid())


	loadbalancer.DefaultLBPolicy_g = os.Getenv("LBPolicy")
	if loadbalancer.DefaultLBPolicy_g == "MLeastConn" {
		globals.NumRetries_g, _ = strconv.Atoi(os.Getenv("RETRIES"))
		// get capacity
		// incoming.Capacity_g, _ = strconv.ParseFloat(os.Getenv("CAPACITY"), 64)
		incoming.Capacity_g, _ = strconv.ParseInt(os.Getenv("CAPACITY"), 10, 64)
	} else {
		globals.NumRetries_g = 1
		incoming.Capacity_g = 0
	}
	reset, _ := strconv.Atoi(os.Getenv("RESET"))
	globals.ResetInterval_g = time.Duration(reset) * time.Microsecond

	// capacity has been set in the env; do not reset
	if incoming.Capacity_g != 0 {
		incoming.RunAvg_g = false
	}

	// Initialize endpoints
	//globals.InitEndpoints()
	svc := os.Getenv("SVC")
	if svc != "" {
		podIPsEnv := os.Getenv("POD_IPS")
	
		// Check if POD_IPS is not empty
		if podIPsEnv == "" {
			log.Println("POD_IPS environment variable is empty or not set")
			return
		}
	
		// Split the pod IPs into a slice (assuming they are comma-separated)
		podIPs := strings.Split(podIPsEnv, ",")
	
		// Log the fetched IPs (for debugging)
		log.Println("Fetched POD_IPS: ", podIPs)
	
		// Populate the globals.Endpoints struct
		var ep globals.Endpoints
		ep.Ips = podIPs
	
		// Store the result in the global Endpoints_g map
		globals.Endpoints_g.Put(svc, ep.Ips)
		globals.InitEndpoints(svc)
		globals.ActiveMap_g.Init(podIPs)
	}

	// Start RTT monitoring with a 2-millisecond interval
	go rttmonitor.StartRTTMonitoring(30 * time.Millisecond)
	log.Println("Started RTT monitoring")

	// incoming request handling
	proxy := incoming.NewProxy(globals.RedirectUrl_g)
	inMux := mux.NewRouter()
	inMux.PathPrefix("/").HandlerFunc(proxy.Handle)

	// outgoing request handling
	outMux := mux.NewRouter()
	outMux.PathPrefix("/").HandlerFunc(outgoing.HandleOutgoing)

	// start running the communication server
	//done := make(chan bool)
	//defer close(done)
	//go controllercomm.RunComm(done)

	// start the proxy services
	go func() {
		log.Fatal(http.ListenAndServe(globals.PROXYINPORT, inMux))
	}()
	log.Fatal(http.ListenAndServe(globals.PROXOUTPORT, outMux))
}
