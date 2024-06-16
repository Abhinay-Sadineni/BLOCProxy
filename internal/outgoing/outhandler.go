package outgoing

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Ank0708/MiCoProxy/globals"
	"github.com/Ank0708/MiCoProxy/internal/loadbalancer"
)

var rttMap sync.Map // To store RTTs for each backend IP

// Function to get RTTs using ss command
func getRTTsForIP(destinationIP string) ([]string, error) {
	// Execute the ss command
	cmd := exec.Command("ss", "-ti")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to execute ss command: %w", err)
	}

	// Parse the output to extract RTT values for the given destination IP
	lines := strings.Split(out.String(), "\n")
	rttPattern := regexp.MustCompile(`rtt:([0-9.]+)`)
	var rtts []string

	for i := 0; i < len(lines)-1; i++ {
		if strings.Contains(lines[i], destinationIP) {
			if i+1 < len(lines) {
				match := rttPattern.FindStringSubmatch(lines[i+1])
				if len(match) > 1 {
					rtts = append(rtts, match[1])
				}
			}
		}
	}

	return rtts, nil
}

func monitorRTTs(destIP string, interval time.Duration) {
	for {
		rtts, err := getRTTsForIP(destIP)
		if err != nil {
			log.Printf("Error getting RTTs for %s: %v\n", destIP, err)
			return
		}
		rttMap.Store(destIP, rtts)
		time.Sleep(interval)
	}
}

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
	log.Println("In the Outhandler/HandleOutgoing")
	r.URL.Scheme = "http"
	r.RequestURI = ""

	svc, port, err := net.SplitHostPort(r.Host)
	if err == nil {
		addService(svc)
	}
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

		// Start monitoring RTTs for the backend IP if not already started
		if _, loaded := rttMap.LoadOrStore(backend.Ip, []string{}); !loaded {
			go monitorRTTs(backend.Ip, 2*time.Second) // adjust the interval as needed
		}

		r.URL.Host = net.JoinHostPort(backend.Ip, port) // use the ip directly
		backend.Incr()                                  // a new request

		// Measure L7 Time
		start := time.Now()
		resp, err = client.Do(r)
		L7Time := time.Since(start)

		log.Println("L7 time is: ", L7Time)
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
		} else {
			w.WriteHeader(http.StatusGatewayTimeout)
		}
		log.Println("err: StatusGatewayTimeout:", resp.StatusCode)
		fmt.Fprintf(w, "Bad reply from server")
	}

	// we always receive a new credit value from the backend
	// it can be a 1 or a 0

	// Read the CHIP header
	chip, _ := strconv.Atoi(resp.Header.Get("CHIP"))
	log.Println("CHIP recieved for server: ", backend.Ip, " from server are: ", chip)
	// Read the Server_count header
	serverCount, _ := strconv.Atoi(resp.Header.Get("Server_count"))
	log.Println("Server_count recieved for server: ", backend.Ip, " from server are: ", serverCount)

	elapsed := time.Since(start).Nanoseconds()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Set(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
	go backend.Update(start, uint64(chip), uint64(serverCount), uint64(elapsed)) // updating server values

	// Log the RTTs for the backend IP
	if rtts, ok := rttMap.Load(backend.Ip); ok {
		log.Printf("RTTs for %s: %v\n", backend.Ip, rtts)
	}

}
