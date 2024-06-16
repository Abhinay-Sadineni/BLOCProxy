package globals

import (
	"sync"
	"time"
)

// BackendSrv stores information for internal decision making
type BackendSrv struct {
	RW           sync.RWMutex
	Ip           string
	Reqs         int64
	RcvTime      time.Time
	LastRTT      uint64
	WtAvgRTT     float64
	Credits      uint64
	Server_count uint64 // Ankit
}

func (backend *BackendSrv) Backoff() {
	backend.RW.Lock()
	defer backend.RW.Unlock()
	backend.RcvTime = time.Now() // now time since > globals.RESET_INTERVAL; refer to MLeastConn algo
	backend.Credits = 0
}

func (backend *BackendSrv) Incr() {
	backend.RW.Lock()
	defer backend.RW.Unlock()
	backend.Reqs++
}

func (backend *BackendSrv) Decr() {
	backend.RW.Lock()
	defer backend.RW.Unlock()
	// we use up a credit whenever a new request is sent to that backend
	backend.Credits--
	backend.Reqs--
}

func (backend *BackendSrv) Update(start time.Time, credits uint64, utz uint64, elapsed uint64) {
	backend.RW.Lock()
	defer backend.RW.Unlock()
	backend.RcvTime = start
	backend.LastRTT = elapsed
	backend.WtAvgRTT = backend.WtAvgRTT*0.5 + 0.5*float64(elapsed)
	backend.Credits += credits
	backend.Server_count = utz // Ankit
}

// Endpoints store information from the control plane
type Endpoints struct {
	Svcname string   `json:"Svcname"`
	Ips     []string `json:"Ips"`
}

type endpointsMap struct {
	mu        sync.Mutex
	endpoints map[string][]string
}

func newEndpointsMap() *endpointsMap {
	return &endpointsMap{mu: sync.Mutex{}, endpoints: make(map[string][]string)}
}

func (em *endpointsMap) Get(svc string) []string {
	em.mu.Lock()
	defer em.mu.Unlock()
	return em.endpoints[svc]
}

func (em *endpointsMap) Put(svc string, backends []string) {
	em.mu.Lock()
	defer em.mu.Unlock()
	em.endpoints[svc] = backends
}

type backendSrvMap struct {
	mu sync.Mutex
	mp map[string][]BackendSrv
}

func newBackendSrvMap() *backendSrvMap {
	return &backendSrvMap{mu: sync.Mutex{}, mp: make(map[string][]BackendSrv)}
}

func (bm *backendSrvMap) Get(svc string) []BackendSrv {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return bm.mp[svc]
}

func (bm *backendSrvMap) Put(svc string, backends []BackendSrv) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.mp[svc] = backends
}

var (
	Capacity_g          int64 // Ankit
	RedirectUrl_g       string
	Svc2BackendSrvMap_g = newBackendSrvMap() // holds all backends for services
	Endpoints_g         = newEndpointsMap()  // all endpoints for all services
	SvcList_g           = make([]string, 0)  // knows all service names
	NumRetries_g        int                  // how many times should a request be retried
	ResetInterval_g     time.Duration
)

const (
	CLIENTPORT = ":5000"
	// PROXYINPORT = ":62083" // Change the input port to avoid conflict with Envoy
	PROXYINPORT = ":62081" // which port will the reverse proxy use for making outgoing request
	PROXOUTPORT = ":62082" // which port the reverse proxy listens on
	// RESET_INTERVAL = time.Second // interval after which credit info of backend expires
)
