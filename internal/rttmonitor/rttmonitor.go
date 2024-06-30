package rttmonitor

import (
	"bytes"
	"fmt"

	// "log"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Ank0708/MiCoProxy/globals"
)

var (
	rttMap = make(map[string]float64)
	stopCh = make(chan struct{})
	wg     sync.WaitGroup
)

func getRTTs(destinationIP string) ([]string, error) {
	cmd := exec.Command("ss", "-ti")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to execute ss command: %w", err)
	}

	lines := strings.Split(out.String(), "\n")
	rttPattern := regexp.MustCompile(`rtt:([0-9.]+)`)
	var rtts []string

	for i := 0; i < len(lines)-1; i++ {
		if strings.Contains(lines[i], destinationIP) {
			if i+1 < len(lines) {
				match := rttPattern.FindStringSubmatch(lines[i+1])
				if len(match) > 1 {
					rtts = append(rtts, match[1])
					// log.Printf("Parsed RTT for IP %s: %s ms", destinationIP, match[1])
				}
			}
		}
	}

	return rtts, nil
}

func monitorRTT(ip string, interval time.Duration) {
	defer wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			// log.Println("RTT monitoring stopped for IP:", ip)
			return
		case <-ticker.C:
			rtts, err := getRTTs(ip)
			if err != nil {
				// log.Println("Error fetching RTT:", err)
				continue
			}

			if len(rtts) > 0 {
				latestRTT, err := strconv.ParseFloat(rtts[len(rtts)-1], 64)
				if err != nil {
					// log.Println("Error parsing RTT:", err)
					continue
				}
				// log.Printf("Inside rttmonitor, %s : %.2f ms", ip, latestRTT)
				updateLatestRTT(ip, latestRTT)
			}
		}
	}
}

func updateLatestRTT(ip string, rtt float64) {
	backend := globals.GetBackendSrvByIP(ip)
	if backend != nil {
		backend.Update_latestRTT(rtt)
		rttMap[ip] = rtt
		// log.Printf("Updated RTT map: %s : %.2f ms", ip, rtt)
	}
	// else {
	// 	log.Println("Backend not found for IP:", ip)
	// }
}

func getActiveBackendIPs() []string {
	activeIPs := make([]string, 0)
	ips := globals.Endpoints_g.Get("localhost")
	for _, ip := range ips {
		if backend := globals.GetBackendSrvByIP(ip); backend != nil {
			activeIPs = append(activeIPs, ip)
		}
	}
	return activeIPs
}

func StartRTTMonitoring(interval time.Duration) {
	backendIPs := getActiveBackendIPs()
	for _, ip := range backendIPs {
		// log.Println("Starting RTT monitoring for IP:", ip)
		wg.Add(1)
		go monitorRTT(ip, interval)
	}
}

func StopRTTMonitoring() {
	close(stopCh)
	wg.Wait()
}

func GetRTT(ip string) float64 {
	// log.Println("Inside RTT Monitoring")
	return rttMap[ip]
}

// GetAllRTTs returns a copy of the rttMap
func GetAllRTTs() map[string]float64 {
	// Create a copy of rttMap to avoid race conditions
	copyMap := make(map[string]float64)
	for ip, rtt := range rttMap {
		copyMap[ip] = rtt
	}
	return copyMap
}
