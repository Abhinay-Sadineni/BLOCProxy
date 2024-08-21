package globals

import (
	"fmt"
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
	Server_count uint64
}

func foo() {
	fmt.Println("hello world")
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
	backend.Server_count = utz
}

func (bm *backendSrvMap) UpdateMap(svc string, ips []string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	backendSrvMap := bm.mp[svc]
	ipSet := make(map[string]bool)
	ipInUpdatedBackends := make(map[string]bool)
	updatedBackends := []BackendSrv{}

	// Create a set of IPs for quick lookup
	for _, ip := range ips {
		ipSet[ip] = true
	}

	// Add only those backends whose IPs are in the ipSet
	for _, backend := range backendSrvMap {
		if ipSet[backend.Ip] {
			updatedBackends = append(updatedBackends, backend)
			ipInUpdatedBackends[backend.Ip] = true
		}
	}

	// Add new IPs that are not already in updatedBackends
	for ip := range ipSet {
		if !ipInUpdatedBackends[ip] {
			updatedBackends = append(updatedBackends, BackendSrv{
				Ip:           ip,
				Reqs:         0,
				LastRTT:      0,
				WtAvgRTT:     0,
				Credits:      1, // credit for all backends is set to 1 at the start
				RcvTime:      time.Now(),
				Server_count: 0,
			})
		}
	}

	// Update the backend map with the new list
	bm.mp[svc] = updatedBackends
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

type inactiveIPMap struct {
	mu sync.Mutex
	mp map[string][]string
}

func newInactiveIPMap() *inactiveIPMap {
	return &inactiveIPMap{mu: sync.Mutex{}, mp: make(map[string][]string)}
}

func (im *inactiveIPMap) Get(svc string) []string {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.mp[svc]
}

func (im *inactiveIPMap) Put(svc string, ips []string) {
	im.mu.Lock()
	defer im.mu.Unlock()
	im.mp[svc] = ips
}

var (
	Capacity_g          int64
	RedirectUrl_g       string
	Svc2BackendSrvMap_g = newBackendSrvMap() // holds all backends for services
	Endpoints_g         = newEndpointsMap()  // all endpoints for all services
	InactiveIPMap_g     = newInactiveIPMap() // holds inactive IPs for services
	SvcList_g           = make([]string, 0)  // knows all service names
	NumRetries_g        int                  // how many times should a request be retried
	ResetInterval_g     time.Duration
	LoadThreshold_g     = 2 // Load threshold for server count

)

const (
	CLIENTPORT  = ":8080"
	PROXYINPORT = ":62081" // which port will the reverse proxy use for making outgoing request
	PROXOUTPORT = ":62082" // which port the reverse proxy listens on
	// RESET_INTERVAL = time.Second // interval after which credit info of backend expires
)

func AddToInactive(svc, ip string, serverCount uint64, reason string) {
	backendSrvMap := Svc2BackendSrvMap_g.Get(svc)
	inactiveIPs := InactiveIPMap_g.Get(svc)

	for i, backend := range backendSrvMap {
		if backend.Ip == ip {
			// Remove from active
			Svc2BackendSrvMap_g.mu.Lock()
			Svc2BackendSrvMap_g.mp[svc] = append(backendSrvMap[:i], backendSrvMap[i+1:]...)
			Svc2BackendSrvMap_g.mu.Unlock()

			// Add to inactive with reason
			InactiveIPMap_g.mu.Lock()
			InactiveIPMap_g.mp[svc] = append(inactiveIPs, ip)
			InactiveIPMap_g.mu.Unlock()

			if reason == "load" {
				// Start timer for IP to be moved back to active list
				delay := time.Duration((serverCount-uint64(LoadThreshold_g))*50) * time.Millisecond
				go func() {
					time.Sleep(delay)
					RemoveFromInactive(svc, ip)
				}()
			}

			return
		}
	}
}

// RemoveFromInactive moves the IP from the inactive list to the global list for the given service
func RemoveFromInactive(svc, ip string) {
	backendSrvMap := Svc2BackendSrvMap_g.Get(svc)
	inactiveIPs := InactiveIPMap_g.Get(svc)

	for i, inactiveIP := range inactiveIPs {
		if inactiveIP == ip {
			// Remove from inactive
			InactiveIPMap_g.mu.Lock()
			InactiveIPMap_g.mp[svc] = append(inactiveIPs[:i], inactiveIPs[i+1:]...)
			InactiveIPMap_g.mu.Unlock()

			// Add to active
			Svc2BackendSrvMap_g.mu.Lock()
			Svc2BackendSrvMap_g.mp[svc] = append(backendSrvMap, BackendSrv{Ip: ip})
			Svc2BackendSrvMap_g.mu.Unlock()
			return
		}
	}
}
