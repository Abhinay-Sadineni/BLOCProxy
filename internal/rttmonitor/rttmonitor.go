package rttmonitor

import (
	"bufio"
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/Ank0708/MiCoProxy/globals"
)

var (
	RTTMap      = make(map[string]float64)
	rttMapMutex sync.RWMutex
	stopCh      = make(chan struct{})
	wg          sync.WaitGroup
)

func getRTT(destinationIP string) float64 {
	cmd := exec.Command("ss", "-ti")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Println("Error creating StdoutPipe for ss command:", err)
		return -1
	}

	if err := cmd.Start(); err != nil {
		log.Println("Error starting ss command:", err)
		return -1
	}

	scanner := bufio.NewScanner(stdout)
	pattern := regexp.MustCompile(destinationIP + `:\S+\s+cubic.*\brtt:(\d+\.\d+)`)

	for scanner.Scan() {
		line := scanner.Text()
		match := pattern.FindStringSubmatch(line)
		if match != nil {
			rtt, err := strconv.ParseFloat(match[1], 64)
			if err != nil {
				log.Println("Error parsing RTT:", err)
				return -1
			}
			return rtt
		}
	}

	if err := scanner.Err(); err != nil {
		log.Println("Error reading ss command output:", err)
		return -1
	}

	if err := cmd.Wait(); err != nil {
		log.Println("Error waiting for ss command to finish:", err)
		return -1
	}

	return -1
}

func monitorRTT(ip string, interval time.Duration) {
	defer wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			log.Println("RTT monitoring stopped for IP:", ip)
			return
		case <-ticker.C:
			rtt := getRTT(ip)
			if rtt != -1 {
				rttMapMutex.Lock()
				RTTMap[ip] = rtt
				rttMapMutex.Unlock()
			}
		}
	}
}

func getAllBackendIPs() []string {
	var backendIPs []string
	for _, svc := range globals.SvcList_g {
		ips := globals.Endpoints_g.Get(svc)
		backendIPs = append(backendIPs, ips...)
	}
	return backendIPs
}

func StartRTTMonitoring(interval time.Duration) {
	backendIPs := getAllBackendIPs()
	for _, ip := range backendIPs {
		wg.Add(1)
		go monitorRTT(ip, interval)
	}
}

func StopRTTMonitoring() {
	close(stopCh)
	wg.Wait()
}

func GetRTT(ip string) float64 {
	rttMapMutex.RLock()
	defer rttMapMutex.RUnlock()
	return RTTMap[ip]
}
