package tcp_stats

import (
	"bytes"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	rttMap   map[string][]string
	rttMutex sync.Mutex
)

func init() {
	rttMap = make(map[string][]string)
}

// Function to get RTTs using ss command
func getRTTs(destinationIP string) ([]string, error) {
	// Execute the ss command
	cmd := exec.Command("ss", "-ti")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to execute ss command: %w", err)
	}

	// Parse the output to extract RTT values for the given destination IP
	lines := strings.Split(out.String(), "\n")
	rttPattern := regexp.MustCompile(`rtt:([0-9.]+)`)
	var rtts []string

	for i := 0; i < len(lines)-1; i++ {
		if strings.Contains(lines[i], destinationIP) {
			if i+1 < len(lines) {
				match := rttPattern.FindStringSubmatch(lines[i+1])
				if len(match) > 1 {
					rtts = append(rtts, match[1])
				}
			}
		}
	}

	return rtts, nil
}

// Function to continuously capture RTTs until stopped
func captureRTTs(destinationIP string, interval time.Duration, stopCh <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rttMutex.Lock()
			rttList, err := getRTTs(destinationIP)
			if err != nil {
				fmt.Printf("Error getting RTTs for %s: %v\n", destinationIP, err)
			} else {
				rttMap[destinationIP] = rttList
			}
			rttMutex.Unlock()

		case <-stopCh:
			fmt.Printf("Stopping RTT capture for %s\n", destinationIP)
			return
		}
	}
}

// Start capturing RTTs for a given destination IP
func startRTTCapture(destinationIP string, stopCh <-chan struct{}) {
	interval := 200 * time.Microsecond
	go captureRTTs(destinationIP, interval, stopCh)
}

func rttHandler(w http.ResponseWriter, r *http.Request) {
	// Get the destination IP from the request
	destinationIP := r.URL.Query().Get("destinationIP")
	if destinationIP == "" {
		http.Error(w, "destinationIP is required", http.StatusBadRequest)
		return
	}

	// Create a channel to signal stop capturing RTTs
	stopCh := make(chan struct{})
	defer close(stopCh)

	// Start capturing RTTs for the provided destination IP
	startRTTCapture(destinationIP, stopCh)

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "RTT capture started for %s\n", destinationIP)
}

func main() {
	// Start HTTP server
	http.HandleFunc("/startCapture", rttHandler)
	fmt.Println("RTT Capture Server started at :8088")
	if err := http.ListenAndServe(":8088", nil); err != nil {
		fmt.Printf("Error starting server: %v\n", err)
	}
}
