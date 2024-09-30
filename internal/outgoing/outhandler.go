package outgoing

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Ank0708/MiCoProxy/globals"
	"github.com/Ank0708/MiCoProxy/internal/loadbalancer"
	//"github.com/Ank0708/MiCoProxy/internal/rttmonitor"
)

func addService(s string) {
	if strings.Contains(s, "epwatcher") {
		return
	}

	for _, svc := range globals.SvcList_g {
		if svc == s {
			return
		}
	}
	globals.SvcList_g = append(globals.SvcList_g, s)
	time.Sleep(time.Nanosecond * 100) // enough for the request handler not to hit the list before it's populated
}

func HandleOutgoing(w http.ResponseWriter, r *http.Request) {
	log.Println("In the Outhandler/HandleOutgoing")
	r.URL.Scheme = "http"
	r.RequestURI = ""

	svc, port, err := net.SplitHostPort(r.Host)
	log.Println("Port: ", port)
	if err == nil {
		addService(svc)
		svc = "yolov5"
	}
	var start time.Time
	var resp *http.Response
	var backend *globals.BackendSrv

	client := &http.Client{Timeout: time.Second * 20}
	var ip string

	for i := 0; i < globals.NumRetries_g; i++ {
		// log.Println("Svc is: ", svc)
		backend, err = loadbalancer.NextEndpoint(svc)

		//retry if server count is too much
		if(backend.Server_count > uint64(globals.LoadThreshold_g) ) {
                continue
		}
		ip = backend.Ip
		if err != nil {
			log.Println("Error fetching backend:", err)
			w.WriteHeader(http.StatusBadGateway)
			// log.Println("err: StatusConflict:", err)
			fmt.Fprint(w, err.Error())
			return
		}
		custom_port := "62081"
		r.URL.Host = net.JoinHostPort(backend.Ip, custom_port) // use the ip directly
		backend.Incr()                                         // a new request

		// Measure L7 Time
		start := time.Now()
		resp, err = client.Do(r)
		// L7Time := time.Since(start)

		// log.Println("L7 time is: ", L7Time)
		if backend != nil && backend.Ip == ip && globals.ActiveMap_g.Get(ip) {
			backend.Decr()
		} // close the request

		if err != nil {
			elapsed := time.Since(start) // how long the rejection took
			w.WriteHeader(http.StatusBadRequest)
			log.Println("err: StatusBadRequest:", err, "i:", i, "elapsed:", elapsed) // debug)
			fmt.Fprint(w, err.Error())
			return
		}

		// we retry the request three times or we break out
		if resp.StatusCode == 429 {
			backend.Backoff() // backoff from this backend for a while
		} else {
			break
		}
	}

	if resp.StatusCode != 200 {
		log.Println("Request being dropped") // debug
		if resp.StatusCode == 429 {
			w.WriteHeader(http.StatusTooManyRequests)
		} else {
			w.WriteHeader(http.StatusGatewayTimeout)
		}
		log.Println("err: StatusGatewayTimeout:", resp.StatusCode)
		fmt.Fprintf(w, "Bad reply from server")
	}

	// Read the CHIP header
	chip, _ := strconv.Atoi(resp.Header.Get("CHIP"))
	// log.Println("CHIP received for server: ", backend.Ip, " from server are: ", chip)
	// log.Println("CHIP received for server: ", backend.Ip, " from server are: ", chip)
	// Read the Server_count header

	start = time.Now()

	serverCount, _ := strconv.Atoi(resp.Header.Get("Server_count"))

	elapsed := time.Since(start).Nanoseconds()
	//msg := fmt.Sprintf("client_count time: %d", elapsed)
	//log.Println(msg) // debug
	if backend != nil && backend.Ip == ip && globals.ActiveMap_g.Get(ip) {
		log.Println("Server_count received for server: ", backend.Ip, " from server are: ", serverCount)
	}

	// Print the RTT value from latestRTT field of the backend server
	// PrintRTTMap()
	// length := globals.GetSvc2BackendSrvMapLength()
	// log.Println("The length of Active List: ", length)
	//rtt := rttmonitor.GetRTT(backend.Ip)
	//log.Printf("RTT for backend %s: %.2f ms", backend.Ip, rtt)

	// elapsed = time.Since(start).Nanoseconds()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Set(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
	if backend != nil && backend.Ip == ip && globals.ActiveMap_g.Get(ip) {
		go backend.Update(start, uint64(chip), uint64(serverCount), uint64(elapsed))
	} else {
		return
	}

	// Check if the server should be moved to inactive list based on server_count
	if int(serverCount) > globals.LoadThreshold_g {
		log.Printf("Moving backend %s to inactive list due to high server_count: %d", backend.Ip, serverCount)
		go globals.AddToInactive(svc, backend.Ip, uint64(serverCount), "load")
	}

	// Check if the server should be moved to inactive list based on RTT
	//if rtt > globals.RTTThreshold_g {
	//	log.Printf("Moving backend %s to inactive list due to high RTT: %.2f ms", backend.Ip, rtt)
	//	globals.AddToInactive(svc, backend.Ip, backend.Server_count, "rtt")
	//}
}
