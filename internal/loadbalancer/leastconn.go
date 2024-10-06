package loadbalancer

import (
	"log"
	"math/rand"
	"time"

	"github.com/Ank0708/MiCoProxy/globals"
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
		ts := time.Since(backends[index1].ResetTime)
		if ts > globals.ResetInterval_g || backends[index1].Credits > 0 {
			break
		}
		index1 = rand.Intn(ln)
	}

	index2 := rand.Intn(ln)

	for {
		ts := time.Since(backends[index2].ResetTime)
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
	ts := time.Since(backend2Return.ResetTime)
	if backend2Return.Credits <= 0 && ts > globals.ResetInterval_g {
		backend2Return.ResetTime = time.Now()
	}

	return backend2Return, nil
}

func Netflix(svc string) (*globals.BackendSrv, error) {
	log.Println("Netflix is used") // debug
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
		ts := time.Now()
		if ts.After(backends[index1].ResetTime) {
			log.Println("reset completed")
			break
		}
		log.Println("RESET working")
		index1 = rand.Intn(ln)
	}

	index2 := rand.Intn(ln)

	for {
		ts := time.Now()
		if index1!=index2 && ts.After(backends[index2].ResetTime){
			break
		}
		index2 = rand.Intn(ln)
	}

	srv1 := &backends[index1]
	srv2 := &backends[index2]

	var backend2Return *globals.BackendSrv
	// var ip string
	if srv1.Reqs+int64(srv1.Server_count) < srv2.Reqs+int64(srv2.Server_count) {
		backend2Return = srv1
	} else {
		backend2Return = srv2
	}

	return backend2Return, nil
}
