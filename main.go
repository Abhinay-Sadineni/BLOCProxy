package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/mux"
)

const (
	CLIENTPORT   = ":5000"
	PROXYINPORT  = ":62081" // which port will the reverse proxy use for making outgoing request
	PROXYOUTPORT = ":62082" // which port the reverse proxy listens on
)

var (
	g_redirectUrl string
	globalMap     = make(map[string]PathStats) // used for timing
)

// used for timing
type PathStats struct {
	// Path      string
	Count     int
	totalTime int64
	RTT       int64
	AvgRTT    int64
	wtAvgRTT  int64
}

type myTransport struct{}

func (t *myTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := http.DefaultTransport.RoundTrip(r)
	if err != nil {
		log.Print("\n\ncame in error resp here", err)
		return nil, err
	}

	_, err = httputil.DumpResponse(response, true) // check if the response is valid
	if err != nil {
		log.Print("\n\nerror in dump response\n")
		return nil, err
	}
	return response, err
}

type Proxy struct {
	target *url.URL
	proxy  *httputil.ReverseProxy
}

func NewProxy(target string) *Proxy {
	url, _ := url.Parse(target)
	return &Proxy{target: url, proxy: httputil.NewSingleHostReverseProxy(url)}
}

func (p *Proxy) handle(w http.ResponseWriter, r *http.Request) {
	// set forwarded for header
	log.Println("incoming") // debug
	s, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		log.Fatal(err)
	}
	w.Header().Set("X-Forwarded-For", s)

	// fmt.Println(r.URL.Path)
	p.proxy.Transport = &myTransport{}
	p.proxy.ServeHTTP(w, r)
}

func addService(s string) bool {
	// add the service we are looking for to the list of services
	// assumes we only ever make requests to internal servers
	found := false

	// if the request is being made to epwatcher then it will create an infinite loop otherwise
	// we also set a rule that any request to port 30000 is to be ignored
	if strings.Contains(s, "epwatcher") {
		return true
	}

	for _, svc := range svcList {
		if svc == s {
			found = true
			break
		}
	}
	if !found {
		svcList = append(svcList, s)
	}
	return found
}

func handleOutgoing(w http.ResponseWriter, r *http.Request) {
	// key := r.Method + r.URL.Path // used for timing

	r.URL.Scheme = "http"
	// r.URL.Host = r.Host
	r.RequestURI = ""

	svc, port, err := net.SplitHostPort(r.Host)
	if err == nil {
		found := addService(svc) // add service to list of services
		if !found {              // first request to service
			getEndpoints(svc)
		}
	} // else we just wing it

	// // supporting http2
	// http2.ConfigureTransport(http.DefaultTransport.(*http.Transport))

	backend, err := NextEndpoint(svc)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err.Error())
		return
	}

	r.URL.Host = net.JoinHostPort(backend.ip, port) // use the ip directly
	atomic.AddInt64(&backend.reqs, 1)               // a new open request
	// backend.reqs += 1                               // a new open request
	// log.Printf("Host %s with %d requests selected", backend.ip, backend.reqs) // debug

	start := time.Now() // used for timing
	rcvTime := start.UnixNano()
	atomic.StoreInt64(&backend.rcvTime, rcvTime)
	// backend.rcvTime = start.UnixNano()
	var client = &http.Client{Timeout: time.Second * 10}
	// response, err := http.DefaultClient.Do(r)
	response, err := client.Do(r)
	elapsed := time.Since(start) // used for timing

	atomic.AddInt64(&backend.reqs, -1) // a request closed
	// backend.reqs -= 1 // a request closed

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err.Error())
		return
	}

	for key, values := range response.Header {
		for _, value := range values {
			w.Header().Set(key, value)
		}
	}

	// // implementing a flusher
	// done := make(chan bool)
	// go func() {
	// 	for {
	// 		select {
	// 		case <-time.Tick(10 * time.Millisecond):
	// 			w.(http.Flusher).Flush()
	// 		case <-done:
	// 			return
	// 		}
	// 	}
	// }()

	// // supporting trailers
	// trailerKeys := []string{}
	// for key := range response.Trailer {
	// 	trailerKeys = append(trailerKeys, key)
	// }
	// w.Header().Set("Trailer", strings.Join(trailerKeys, ","))

	w.WriteHeader(response.StatusCode)
	io.Copy(w, response.Body)

	// // adding trailers
	// for key, values := range response.Trailer {
	// 	for _, value := range values {
	// 		w.Header().Set(key, value)
	// 	}
	// }

	// close(done)

	// if val, ok := globalMap[key]; ok {
	if val, ok := globalMap[backend.ip]; ok {
		val.Count++
		val.RTT = elapsed.Nanoseconds()
		val.totalTime += val.RTT
		val.AvgRTT = val.totalTime / int64(val.Count)
		val.wtAvgRTT = int64(float64(val.wtAvgRTT)*0.5 + float64(val.RTT)*0.5)
		globalMap[backend.ip] = val
	} else {
		var m PathStats
		m.Count = 1
		m.RTT = elapsed.Nanoseconds()
		val.totalTime = val.RTT
		val.AvgRTT = val.RTT
		val.wtAvgRTT = val.RTT
		globalMap[backend.ip] = m
	}

	// update timing of the ip
	var ip atomic.Value
	ip.Store(backend.ip)
	ipString := ip.Load().(string)
	// rtt := atomic.LoadInt64()
	atomic.SwapInt64(&backend.lastRTT, globalMap[ipString].RTT)
	// backend.lastRTT = globalMap[backend.ip].RTT
	atomic.SwapInt64(&backend.avgRTT, globalMap[ipString].AvgRTT)
	// backend.avgRTT = globalMap[backend.ip].AvgRTT
	// backend.wtAvgRTT = globalMap[backend.ip].wtAvgRTT
	atomic.SwapInt64(&backend.wtAvgRTT, globalMap[ipString].wtAvgRTT)

	// log.Printf("%#+v\n", Svc2BackendSrvMap[svc]) // debug
}

func getStats(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, globalMap)
}

func main() {
	g_redirectUrl = "http://localhost" + CLIENTPORT
	fmt.Println("Input Port", PROXYINPORT)
	fmt.Println("Output Port", PROXYOUTPORT)
	fmt.Println("redirecting to:", g_redirectUrl)
	fmt.Println("User ID:", os.Getuid())
	proxy := NewProxy(g_redirectUrl)
	// outMux := http.NewServeMux()
	// outMux.HandleFunc("/", handleOutgoing)
	outMux := mux.NewRouter()
	outMux.PathPrefix("/").HandlerFunc(handleOutgoing)
	// inMux := http.NewServeMux()
	// inMux.HandleFunc("/", proxy.handle)
	// inMux.HandleFunc("/stats", getStats)
	inMux := mux.NewRouter()
	inMux.HandleFunc("/stats", getStats)
	inMux.PathPrefix("/").HandlerFunc(proxy.handle)

	// start running the communication server
	done := make(chan bool)
	defer close(done)
	go runComm(done)

	go func() { log.Fatal(http.ListenAndServe(PROXYINPORT, inMux)) }()
	log.Fatal(http.ListenAndServe(PROXYOUTPORT, outMux))
}
