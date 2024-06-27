package prober

import (
	"log"
	"net"
	"time"

	"github.com/Ank0708/MiCoProxy/globals"
)

// ProbeRTT probes the RTT for a given IP at specified intervals
func ProbeRTT(ip string, interval time.Duration, svc string) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		rtt, err := measureRTT(ip)
		if err != nil {
			log.Println("Error fetching RTT:", err)
			continue
		}

		if rtt < globals.RTTThreshold_g {
			log.Printf("RTT for inactive backend %s is now below threshold: %.2f ms", ip, rtt)
			globals.RemoveFromInactive(svc, ip)
			return
		}
	}
}

// measureRTT measures the RTT to the given IP address
func measureRTT(ip string) (float64, error) {
	// Simulated RTT measurement logic
	conn, err := net.Dial("udp", ip+":0")
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	start := time.Now()
	if _, err := conn.Write([]byte("ping")); err != nil {
		return 0, err
	}

	buf := make([]byte, 1024)
	if _, err := conn.Read(buf); err != nil {
		return 0, err
	}
	rtt := time.Since(start).Seconds() * 1000 // RTT in milliseconds

	return rtt, nil
}
