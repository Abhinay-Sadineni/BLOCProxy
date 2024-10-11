package rttmonitor

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/Ank0708/MiCoProxy/globals"
)

var (
	rttMap   = make(map[string]float64)
	backendMap = make((map[string]*globals.BackendSrv))
	rttMutex = sync.RWMutex{} // Mutex to protect access to rttMap
	stopCh   = make(chan struct{})
	wg       sync.WaitGroup
)




// monitorRTTs runs a single loop to monitor RTTs at the specified interval
func getRawSSOutput() (string, error) {
	// Add dport filter to the ss command
	cmd := exec.Command("ss", "-tin", "dport", "62081")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to execute ss command: %w", err)
	}
	return out.String(), nil
}

// parseRTTs parses the RTTs for all active backend IPs from the ss command output
func parseRTTs(ssOutput string) map[string]float64 {
	// Regular expression to match lines containing both IP and RTT
	rttPattern := regexp.MustCompile(`(\d+\.\d+\.\d+\.\d+).*?rtt:([0-9.]+)`)
	
	// Map to hold IPs and their RTTs
	rttMap := make(map[string]float64)

	// Find all matches in the ss output
	matches := rttPattern.FindAllStringSubmatch(ssOutput, -1)
	for _, match := range matches {
		if len(match) == 3 { // match[0] is the full match, match[1] is the IP, match[2] is the RTT
			ip := match[1]
			rtt, err := strconv.ParseFloat(match[2], 64)
			if err == nil {
				rttMap[ip] = rtt
			}
		}
	}

	return rttMap
}

// monitorRTTs runs a single loop to monitor RTTs at the specified interval
func monitorRTTs(interval time.Duration) {
	defer wg.Done()
	backendIPs := getActiveBackendIPs()
	backendMap  = globals.GetIptobackendMap("yolov5")  

	for {
		select {
		case <-stopCh:
			log.Println("RTT monitoring stopped")
			return
		default:
			// Fetch ss output and handle errors
			startTime := time.Now()
			ssOutput, err := getRawSSOutput()
			elapsedTime := time.Since(startTime)

			if err != nil {
				log.Println("Error fetching ss output:", err)
				continue
			}

			if len(backendIPs) == 0 {
				log.Println("No active backend IPs found")
				continue
			}

			// Parse RTTs from the output
			rttMap := parseRTTs(ssOutput)

			// Update the latest RTT for each active backend IP
			for _, ip := range backendIPs {
				if rtt, found := rttMap[ip]; found {
					updateLatestRTT(ip, rtt)
				}
			}

			log.Println("RTT monitor elapsed time:", elapsedTime)
			
			// Sleep for the remaining interval time
			sleepDuration := interval - elapsedTime
			if sleepDuration > 0 {
				time.Sleep(sleepDuration)
			}
		}
	}
}



// updateLatestRTT updates the RTT for a given IP in the map and backend service
func updateLatestRTT(ip string, rtt float64) {
	backend := backendMap[ip]
	if backend != nil {
		backend.Update_latestRTT(rtt)

		// rttMutex.Lock()
		// rttMap[ip] = rtt
		// rttMutex.Unlock()

		//log.Printf("Updated RTT map: %s : %.2f ms", ip, rtt)
	} else {
		//log.Println("Backend not found for IP:", ip)
	}
}

// getActiveBackendIPs retrieves the list of active backend IPs
func getActiveBackendIPs() []string {
	activeIPs := make([]string, 0)
	ips := globals.Endpoints_g.Get("yolov5")
	log.Println("Inside ips: ", ips)
	for _, ip := range ips {
		if backend := globals.GetBackendSrvByIP(ip); backend != nil {
			activeIPs = append(activeIPs, ip)
		}
	}
	return activeIPs
}

// StartRTTMonitoring initializes the monitoring goroutine
func StartRTTMonitoring(interval time.Duration) {
	wg.Add(1)
	go monitorRTTs(interval)
}

// StopRTTMonitoring signals the monitoring goroutine to stop and waits for it to finish
func StopRTTMonitoring() {
	close(stopCh)
	wg.Wait()
}

// GetRTT retrieves the latest RTT for a given IP
func GetRTT(ip string) float64 {
	rttMutex.RLock()
	defer rttMutex.RUnlock()
	return rttMap[ip]
}
