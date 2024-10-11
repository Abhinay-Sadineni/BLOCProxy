package globals

import (
	// "fmt"
	// "log"

	"log"
	//"net"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"sync"
	"time"
	"github.com/go-ping/ping"
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
	index        int64
	ResetTime    time.Time
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

func (backend *BackendSrv) GetRTT() float64{
     backend.RW.Lock()
	 defer backend.RW.Unlock()
	 return backend.latestRTT
}



func (backend *BackendSrv) GetRESETTIME() time.Time{
	backend.RW.Lock()
	defer backend.RW.Unlock()
	return backend.ResetTime
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


	var difference uint64
	if utz > uint64(LoadThreshold_g) {
		difference = utz - uint64(LoadThreshold_g)
	} else {
		difference = 0
	}
	
	differenceDuration := time.Duration(difference) * time.Millisecond *100
	
	backend.ResetTime = time.Now().Add(differenceDuration)
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

type IndexMap struct {
	mu sync.Mutex
	mp map[string][]int64
}

func newIndexMap() *IndexMap {
	return &IndexMap{mp: make(map[string][]int64)}
}

// Get returns the active status of the given IP.
func (am *IndexMap) Get(svc string) []int64 {
	am.mu.Lock()
	defer am.mu.Unlock()
	return am.mp[svc]
}

// Put sets the active status for the given IP.
func (am *IndexMap) Put(svc string, indices []int64) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.mp[svc] = indices
}

var (
	Capacity_g          int64 // Ankit
	RedirectUrl_g       string
	Svc2BackendSrvMap_g = newBackendSrvMap() // holds all backends for services
	Endpoints_g         = newEndpointsMap()  // all endpoints for all services
	//InactiveIPMap_g     = newInactiveIPMap() // holds inactive IPs for services
	ActiveMap_g     = newIndexMap()
	InactiveMap_g   = newIndexMap()
	SvcList_g       = make([]string, 0) // knows all service names
	NumRetries_g    int                 // how many times should a request be retried
	ResetInterval_g time.Duration
	RTTThreshold_g  = 10.0 // RTT threshold value
	LoadThreshold_g = 10   // Load threshold for server count
)

const (
	CLIENTPORT  = ":8080"
	PROXYINPORT = ":62081" // which port will the reverse proxy use for making outgoing request
	PROXOUTPORT = ":62082" // which port the reverse proxy listens on
	// RESET_INTERVAL = time.Second // interval after which credit info of backend expires
)

func InitEndpoints(svc string) {
	// Example service name and hard-coded IPs

	IPS := Endpoints_g.Get(svc)

	// Initialize BackendSrv instances for each IP and put them into Svc2BackendSrvMap_g
	backends := make([]BackendSrv, len(IPS))
	indices := make([]int64, len(IPS))
	for i, ip := range IPS {
		backends[i] = BackendSrv{
			Ip:    ip,
			index: int64(i),
		}
		indices[i] = (int64(i))
	}
	Svc2BackendSrvMap_g.Put(svc, backends)
	ActiveMap_g.Put(svc, indices)
}

func GetIptobackendMap(svc string) map[string]*BackendSrv {
	Svc2BackendSrvMap_g.mu.Lock()
	defer Svc2BackendSrvMap_g.mu.Unlock()

	backendMap := make(map[string]*BackendSrv)
	for _, backends := range Svc2BackendSrvMap_g.mp {
		for i := range backends {
			backendMap[backends[i].Ip] = &backends[i]
		}
	}
	return backendMap
}

// AddToInactive moves the IP from the global list to the inactive list for the given service
func AddToInactive(svc string, index int64) {
	AIndexMap := ActiveMap_g.Get(svc)
	IIndexMap := InactiveMap_g.Get(svc)
	for i := range AIndexMap {
		if AIndexMap[i] == index {
			NewActive := append(AIndexMap[:i], AIndexMap[i+1:]...)
			ActiveMap_g.Put(svc, NewActive)

			ANewInactive := append(IIndexMap, index)
			InactiveMap_g.Put(svc, ANewInactive)
			return
		}
	}
}

// RemoveFromInactive moves the IP from the inactive list to the global list for the given service
func RemoveFromInactive(svc string, index int64) {
	IIndexMap := InactiveMap_g.Get(svc)
	AIndexMap := ActiveMap_g.Get(svc)

	for i := range IIndexMap {
		if IIndexMap[i] == index {
			NewInActive := append(IIndexMap[:i], IIndexMap[i+1:]...)
			InactiveMap_g.Put(svc, NewInActive)

			Newactive := append(AIndexMap, index)
			ActiveMap_g.Put(svc, Newactive)
			return
		}
	}
}

func MonitoringAgent(svc string, backend *BackendSrv, exitChan chan bool) {
    ticker := time.NewTicker(2 * time.Millisecond)
    defer ticker.Stop() // Ensure the ticker is stopped when the function exits

    for {
        select {
			
		case <-exitChan:
            if time.Now().Before(backend.GetRESETTIME()){
                    AddToInactive(svc, backend.index)

					RetractingAgent(svc , backend ,exitChan ,"load")
			} 
            return

        case <-ticker.C:
            // Check if the RTT exceeds the threshold
            if backend.GetRTT() > RTTThreshold_g {
                log.Printf("RTT for %s exceeded threshold. Moving to inactive.", svc)

                // Move the backend to inactive
                AddToInactive(svc, backend.index)

                // Optionally call the retracting agent after moving to inactive
                RetractingAgent(svc, backend, exitChan ,"rtt")

            }
        }
    }
}


func RetractingAgent(svc string, backend *BackendSrv, exitChan chan bool, reason string) {
    switch reason {
              case "rtt":

				pinger, err := ping.NewPinger(backend.Ip)		
				pinger.Count = 1  // Only ping once per tick
				pinger.Timeout = 1 * time.Second  				

				if err != nil {
					log.Printf("Failed to create pinger for backend %s: %v", backend.Ip, err)
					 return
					}

				ticker := time.NewTicker(10 * time.Millisecond)
				defer ticker.Stop() 

				for{
					select{

						// IF RESPONSE IS RECEVIED 
					    case <-exitChan:


							err := pinger.Run() // Send ICMP ping
                            if err != nil {
                                     log.Printf("Ping failed for backend %s: %v", backend.Ip, err)
                                      continue // Retry on failure
                                    }

                            stats := pinger.Statistics()
                            currentRTT := stats.AvgRtt.Milliseconds()
                            if float64(currentRTT) <= RTTThreshold_g{
								log.Printf("Backend %s has recovered from RTT. Moving back to active list.", backend.Ip)
								RemoveFromInactive(svc, backend.index)
								backend.Update_latestRTT(float64(currentRTT))
								return
							}
	
                        // RESPONSE NOT RECEIVED 
						case <-ticker.C:
							if backend.GetRTT() <= RTTThreshold_g 	{
								log.Printf("Backend %s has recovered from RTT. Moving back to active list.", backend.Ip)
								RemoveFromInactive(svc, backend.index)
								return
							}
                          
					}
				}
			
              case "load":        
                 time.Sleep(time.Until(backend.GetRESETTIME()))
                 log.Printf("Backend %s has recovered from load. Moving back to active list.", backend.Ip)
                 RemoveFromInactive(svc, backend.index)
				 return
         
    }
}


// probeRTT probes the RTT for a given IP at specified intervals
// func probeRTT(ip string, interval time.Duration, svc string) {
// 	ticker := time.NewTicker(interval)
// 	defer ticker.Stop()
// 	log.Println("Inside ProbeRTT")

// 	for range ticker.C {
// 		rtt, err := measureRTT(ip)
// 		//log.Printf("Inside probeRTT: %.2f ms",rtt)
// 		if err != nil {
// 			log.Println("Error fetching RTT:", err)
// 			continue
// 		}

// 		if rtt < RTTThreshold_g {
// 			log.Printf("RTT for inactive backend %s is now below threshold: %.2f ms", ip, rtt)
// 			RemoveFromInactive(svc, ip)
// 			return
// 		}
// 		//log.Printf("RTT for inactive backend %s is now above threshold: %.2f ms", ip, rtt)
// 	}
// }

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
// func measureRTT(ip string) (float64, error) {
// 	log.Println("Inside measureRTT")
// 	conn, err := net.Dial("tcp", ip+":8080")
// 	if err != nil {
// 		log.Println("1 ", err)
// 		return 0, err
// 	}
// 	defer conn.Close()

// 	start := time.Now()
// 	log.Println("start time: ",start)
// 	_, err = conn.Write([]byte("ping"))
// 	if err != nil {
// 		log.Println("2 ", err)
// 		return 0, err
// 	}

// 	buf := make([]byte, 1024)
// 	_, err = conn.Read(buf)
// 	if err != nil {
// 		return 0, err
// 	}
// 	log.Println("completed")

// 	elapsed := time.Since(start).Seconds() * 1000 // convert to milliseconds
// 	log.Println("Inside measureRTT",elapsed)
// 	return elapsed, nil
// }

func measureRTT(ip string) (float64, error) {
	// Define the command to run hping3 with -c 5 for multiple packets
	cmd := exec.Command("sudo", "hping3", "-S", "-c", "1", "-p", "8080", ip)

	// Run the command and capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Println("Error executing hping3 command:", err)
		return 0, err
	}

	// Convert output to string
	outputStr := string(output)

	// Regular expression to extract the average RTT from the statistics line
	re := regexp.MustCompile(`round-trip min/avg/max = [\d.]+/([\d.]+)/[\d.]+ ms`)
	matches := re.FindStringSubmatch(outputStr)
	if len(matches) < 2 {
		return 0, fmt.Errorf("failed to parse average RTT from hping3 output")
	}

	// Convert average RTT value to float64
	avgRTT, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		log.Println("Error parsing average RTT:", err)
		return 0, err
	}

	return avgRTT, nil
}
