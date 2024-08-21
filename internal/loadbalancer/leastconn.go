package loadbalancer

import (
	"log"
	"math/rand"
	"time"

	"local/Abhinay/globals"
)

func LeastConn(svc string) (*globals.BackendSrv, error) {
	log.Println("Least Connection used") // debug
	backends, err := GetBackendSvcList(svc)
	if err != nil {
		return nil, err
	}

	// P2C Least Conn
	seed := time.Now().UTC().UnixNano()
	rand.Seed(seed)
	srv1 := &backends[rand.Intn(len(backends))]
	srv2 := &backends[rand.Intn(len(backends))]

	// var ip string
	if srv1.Reqs < srv2.Reqs {
		return srv1, nil
	}
	return srv2, nil
}

func MLeastConn(svc string) (*globals.BackendSrv, error) {
	log.Println("MLeast Connection used") // debug
	backends, err := GetBackendSvcList(svc)
	if err != nil {
		return nil, err
	}

	// P2C Least Conn
	seed := time.Now().UTC().UnixNano()
	rand.Seed(seed)
	ln := len(backends)

	// we select two servers if they have a credit
	// or it has been more than a second since the last response
	index1 := rand.Intn(ln)

	for {
		ts := time.Since(backends[index1].RcvTime)
		if ts > globals.ResetInterval_g || backends[index1].Credits > 0 {
			break
		}
		index1 = rand.Intn(ln)
	}

	index2 := rand.Intn(ln)

	for {
		ts := time.Since(backends[index2].RcvTime)
		if ts > globals.ResetInterval_g || backends[index2].Credits > 0 {
			break
		}
		index2 = rand.Intn(ln)
	}

	srv1 := &backends[index1]
	srv2 := &backends[index2]

	var backend2Return *globals.BackendSrv
	// var ip string
	if srv1.Reqs < srv2.Reqs {
		backend2Return = srv1
	} else {
		backend2Return = srv2
	}

	// if credits have expired then we want to send a single probe
	ts := time.Since(backend2Return.RcvTime)
	if backend2Return.Credits <= 0 && ts > globals.ResetInterval_g {
		backend2Return.RcvTime = time.Now()
	}

	return backend2Return, nil
}

func Netflix(svc string) (*globals.BackendSrv, error) {
	log.Println("Netflix algorithm used") // debug
	backends, err := GetBackendSvcList(svc)
	if err != nil {
		return nil, err
	}

	// P2C Least Conn
	seed := time.Now().UTC().UnixNano()
	rand.Seed(seed)
	ln := len(backends)

	// we select two servers if they have a
	// or it has been more than a second since the last response
	index1 := rand.Intn(ln)

	for { // filtering the servers
		ts := time.Since(backends[index1].RcvTime)
		if ts > globals.ResetInterval_g || float64(backends[index1].Server_count) < float64(0.95)*float64(globals.Capacity_g) {
			break
		}
		index1 = rand.Intn(ln)
	}

	index2 := rand.Intn(ln)

	for {
		ts := time.Since(backends[index2].RcvTime)
		if ts > globals.ResetInterval_g || float64(backends[index1].Server_count) < float64(0.95)*float64(globals.Capacity_g) {
			break
		}
		index2 = rand.Intn(ln)
	}

	srv1 := &backends[index1]
	srv2 := &backends[index2]

	var backend2Return *globals.BackendSrv
	// compare scores of these 2 randomly selected servers
	if score(srv1) < score(srv2) {
		backend2Return = srv1
	} else {
		backend2Return = srv2
	}

	// if capacity is more than set constraint then we want to send a single probe
	ts := time.Since(backend2Return.RcvTime)
	if float64(backend2Return.Server_count) >= float64(0.95)*float64(globals.Capacity_g) && ts > globals.ResetInterval_g {
		backend2Return.RcvTime = time.Now()
	}
    log.Println("backend selected")
	return backend2Return, nil
}

func score(srv *globals.BackendSrv) float64 {

	return float64(-1 * (2*srv.Reqs + 1*int64(srv.Server_count)))
}
