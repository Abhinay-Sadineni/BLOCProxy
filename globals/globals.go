package globals

import (
	// "fmt"
	// "log"

	"log"
	"net"
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

// Get returns the active status of the given IP.
func (am *activeMap) Get(ip string) bool {
	am.mu.Lock()
	defer am.mu.Unlock()
	return am.mp[ip]
}

// Put sets the active status for the given IP.
func (am *activeMap) Put(ip string, status bool) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.mp[ip] = status
}

// Delete removes the given IP from the map.
func (am *activeMap) Delete(ip string) {
	am.mu.Lock()
	defer am.mu.Unlock()
	delete(am.mp, ip)
}


var (
	Capacity_g          int64 // Ankit
	RedirectUrl_g       string
	Svc2BackendSrvMap_g = newBackendSrvMap() // holds all backends for services
	Endpoints_g         = newEndpointsMap()  // all endpoints for all services
	//InactiveIPMap_g     = newInactiveIPMap() // holds inactive IPs for services
	ActiveMap_g     = newActiveMap()
	SvcList_g       = make([]string, 0) // knows all service names
	NumRetries_g    int                 // how many times should a request be retried
	ResetInterval_g time.Duration
	RTTThreshold_g  = 10.0 // RTT threshold value
	LoadThreshold_g = 10   // Load threshold for server count
)

func AddToInactive(svc, ip string, serverCount uint64, reason string) {
	backendSrvMap := Svc2BackendSrvMap_g.Get(svc)
	//inactiveIPs := InactiveIPMap_g.Get(svc)

	for i, backend := range backendSrvMap {
		//log.Println(backend.Ip," ",ip)
		if backend.Ip == ip {
			// Remove from active

			Svc2BackendSrvMap_g.mu.Lock()
			Svc2BackendSrvMap_g.mp[svc] = append(backendSrvMap[:i], backendSrvMap[i+1:]...)
			Svc2BackendSrvMap_g.mu.Unlock()

			// Add to inactive with reason
			// InactiveIPMap_g.mu.Lock()
			// InactiveIPMap_g.mp[svc] = append(inactiveIPs, ip)
			// InactiveIPMap_g.mu.Unlock()

			if reason == "load" {
				// Start timer for IP to be moved back to active list
				delay := time.Duration((serverCount-uint64(LoadThreshold_g))*50) * time.Millisecond
				go func() {
					time.Sleep(delay)
					RemoveFromInactive(svc, ip)
				}()
			} else if reason == "rtt" {
				// Start probing the RTT for the IP
				log.Println("Start probing")
				go probeRTT(ip, 10*time.Millisecond, svc)
			}

			return
		}
	}
}

// RemoveFromInactive moves the IP from the inactive list to the global list for the given service
func RemoveFromInactive(svc, ip string) {
	backendSrvMap := Svc2BackendSrvMap_g.Get(svc)
	log.Println("Length of backedn list: ", len(backendSrvMap))
	//inactiveIPs := InactiveIPMap_g.Get(svc)

	//for i, inactiveIP := range inactiveIPs {
	//if inactiveIP == ip {
	// Remove from inactive
	// InactiveIPMap_g.mu.Lock()
	// InactiveIPMap_g.mp[svc] = append(inactiveIPs[:i], inactiveIPs[i+1:]...)
	// InactiveIPMap_g.mu.Unlock()

	// Add to active
	Svc2BackendSrvMap_g.mu.Lock()
	Svc2BackendSrvMap_g.mp[svc] = append(backendSrvMap, BackendSrv{Ip: ip})
	Svc2BackendSrvMap_g.mu.Unlock()

	log.Println("removed from inactive: ", ip)
	ActiveMap_g.Put(ip, true)
	return
	//}
	//}
}

// probeRTT probes the RTT for a given IP at specified intervals
func probeRTT(ip string, interval time.Duration, svc string) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		rtt, err := measureRTT(ip)
		if err != nil {
			// log.Println("Error fetching RTT:", err)
			continue
		}

		if rtt < RTTThreshold_g {
			// log.Printf("RTT for inactive backend %s is now below threshold: %.2f ms", ip, rtt)
			RemoveFromInactive(svc, ip)
			return
		}
	}
}

// measureRTT measures the RTT to the given IP address
/*func measureRTT(ip string) (float64, error) {
	// Simulated RTT measurement logic
	conn, err := net.Dial("udp", ip+":8080")
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	start := time.Now()
	_, err = conn.Write([]byte("ping"))
	if err != nil {
		return 0, err
	}

	buf := make([]byte, 1024)
	_, err = conn.Read(buf)
	if err != nil {
		return 0, err
	}

	elapsed := time.Since(start).Seconds() * 1000 // convert to milliseconds
	return elapsed, nil
}*/

// measureRTT measures the RTT to the given IP address using ICMP
func measureRTT(ip string) (float64, error) {
	conn, err := net.Dial("tcp", ip+":8080")
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	start := time.Now()
	_, err = conn.Write([]byte("ping"))
	if err != nil {
		return 0, err
	}

	buf := make([]byte, 1024)
	_, err = conn.Read(buf)
	if err != nil {
		return 0, err
	}

	elapsed := time.Since(start).Seconds() * 1000 // convert to milliseconds
	return elapsed, nil
}
