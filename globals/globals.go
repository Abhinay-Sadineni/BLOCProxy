package globals

import (
	"sync"
	"time"

	"github.com/Ank0708/MiCoProxy/prober"
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
	latestRTT    float64
}

// GetBackendSrvByIP returns the BackendSrv instance for the given IP
func GetBackendSrvByIP(ip string) *BackendSrv {
	Svc2BackendSrvMap_g.mu.Lock()
	defer Svc2BackendSrvMap_g.mu.Unlock()

	for _, backends := range Svc2BackendSrvMap_g.mp {
		for i := range backends {
			if backends[i].Ip == ip {
				return &backends[i]
			}
		}
	}
	return nil
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

func (backend *BackendSrv) Update_latestRTT(latestRTT float64) {
	backend.RW.Lock()
	defer backend.RW.Unlock()
	backend.latestRTT = latestRTT
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
	Capacity_g          int64 // Ankit
	RedirectUrl_g       string
	Svc2BackendSrvMap_g = newBackendSrvMap() // holds all backends for services
	Endpoints_g         = newEndpointsMap()  // all endpoints for all services
	InactiveIPMap_g     = newInactiveIPMap() // holds inactive IPs for services
	SvcList_g           = make([]string, 0)  // knows all service names
	NumRetries_g        int                  // how many times should a request be retried
	ResetInterval_g     time.Duration
	RTTThreshold_g      = 10.0 // RTT threshold value
	LoadThreshold_g     = 2    // Load threshold for server count
)

const (
	CLIENTPORT  = ":8080"
	PROXYINPORT = ":62081" // which port will the reverse proxy use for making outgoing request
	PROXOUTPORT = ":62082" // which port the reverse proxy listens on
	// RESET_INTERVAL = time.Second // interval after which credit info of backend expires
)

func InitEndpoints() {
	// Example service name and hard-coded IPs
	serviceName := "localhost"
	hardcodedIPs := []string{
		"10.244.1.245",
		"10.244.1.246",
		"10.244.1.247",
		"10.244.1.248",
		"10.244.2.247",
		"10.244.2.248",
		"10.244.2.249",
		"10.244.2.250",
	}

	Endpoints_g.Put(serviceName, hardcodedIPs)

	// Initialize BackendSrv instances for each IP and put them into Svc2BackendSrvMap_g
	backends := make([]BackendSrv, len(hardcodedIPs))
	for i, ip := range hardcodedIPs {
		backends[i] = BackendSrv{
			Ip: ip,
		}
	}
	Svc2BackendSrvMap_g.Put(serviceName, backends)
}

// AddToInactive moves the IP from the global list to the inactive list for the given service
func AddToInactive(svc, ip string, serverCount uint64, reason string) {
	// Move IP from active to inactive
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
			} else if reason == "rtt" {
				// Start probing the RTT for the IP
				go prober.ProbeRTT(ip, 10*time.Millisecond, svc)
			}

			return
		}
	}
}

// RemoveFromInactive moves the IP from the inactive list to the global list for the given service
func RemoveFromInactive(svc, ip string) {
	// Move IP from inactive to active
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
