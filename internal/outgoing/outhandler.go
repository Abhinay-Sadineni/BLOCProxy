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

	"local/Abhinay/globals"
	"local/Abhinay/internal/loadbalancer"
)

func addService(s string) {
	// add the service we are looking for to the list of services
	// assumes we only ever make requests to internal servers

	// if the request is being made to epwatcher then it will create an infinite loop
	// we have also set a rule that any request to port 30000 is to be ignored
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
	r.URL.Scheme = "http"
	r.RequestURI = ""

	svc, port, err := net.SplitHostPort(r.Host)
	if err == nil {
		addService(svc)
	}
	log.Println("(Before Load balencing)Outgoing request to ", r.Host)
	var start time.Time
	var resp *http.Response
	var backend *globals.BackendSrv

	client := &http.Client{Timeout: time.Second * 20}

	for i := 0; i < globals.NumRetries_g; i++ {
		backend, err = loadbalancer.NextEndpoint(svc)
		if err != nil {
			log.Println("Error fetching backend:", err)
			w.WriteHeader(http.StatusBadGateway)
			log.Println("err: StatusConflict:", err)
			fmt.Fprint(w, err.Error())
			return
		}

		r.URL.Host = net.JoinHostPort(backend.Ip, port) // use the ip directly
		backend.Incr()                                  // a new request

		start = time.Now()
		log.Println("(After Load balencing)Outgoing request to ", r.URL.Host)
		resp, err = client.Do(r)

		backend.Decr() // close the request

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
		} else if resp.StatusCode == 0 { // ensure WriteHeader is called even if the loop breaks due to a non-retryable error
			w.WriteHeader(http.StatusGatewayTimeout)
		} else {
			w.WriteHeader(resp.StatusCode)
		}
		log.Println("err: StatusGatewayTimeout:", resp.StatusCode)
		fmt.Fprintf(w, "Bad reply from server")
		return
	}

	// we always receive a new credit value from the backend
	// it can be a 1 or a 0
	chip, _ := strconv.Atoi(resp.Header.Get("CHIP"))
	utz, _ := strconv.Atoi(resp.Header.Get("Server_count"))
	log.Printf("In outhandler, Server count: %d\n", utz)

	elapsed := time.Since(start).Nanoseconds()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Set(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode) // Set the response status code once
	io.Copy(w, resp.Body)
	go backend.Update(start, uint64(chip), uint64(utz), uint64(elapsed))

	if int(utz) > globals.LoadThreshold_g {
		// log.Printf("Moving backend %s to inactive list due to high server_count: %d", backend.Ip, serverCount)
		globals.AddToInactive(svc, backend.Ip, uint64(utz), "load")
	}
}
