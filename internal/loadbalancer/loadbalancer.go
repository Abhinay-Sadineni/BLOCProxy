package loadbalancer

import (
	"errors"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/Ank0708/MiCoProxy/globals"
)

var DefaultLBPolicy_g string

const BitsPerWord = 32 << (^uint(0) >> 63)
const MaxInt = 1<<(BitsPerWord-1) - 1

func GetBackendSvcList(svc string) ([]globals.BackendSrv, error) {
	mapExists := globals.Svc2BackendSrvMap_g.Get(svc) // send a reference to the original instead of making a copy
	if len(mapExists) > 0 {
		return mapExists, nil
	}
	// else if
	// make entries into backendSrvs here
	var backendSrvs []globals.BackendSrv
	ips := globals.Endpoints_g.Get(svc)
	if len(ips) > 0 {
		for _, ip := range ips {
			backendSrvs = append(backendSrvs,
				globals.BackendSrv{Ip: ip,
					Reqs:     0,
					LastRTT:  0,
					WtAvgRTT: 0,
					// credit for all backends is set to 1 at the start
					// it's up to the backend to update it
					Credits:   1,
					ResetTime: time.Now(),
				})
		}
		// add backend to the backend maps
		globals.Svc2BackendSrvMap_g.Put(svc, backendSrvs)
		return globals.Svc2BackendSrvMap_g.Get(svc), nil
	}
	// else
	return nil, errors.New("no backends found")
}

func Random(svc string) (*globals.BackendSrv, error) {
	log.Println("Random used") // debug
	backends, err := GetBackendSvcList(svc)
	if err != nil {
		log.Println("Random error", err.Error()) // debug
		return nil, err
	}

	seed := time.Now().UTC().UnixNano()
	rand.Seed(seed)

	ln := len(backends)
	index := rand.Intn(ln)
	return &backends[index], nil
}

func NextEndpoint(svc string) (*globals.BackendSrv, error) {
	if DefaultLBPolicy_g == "" {
		DefaultLBPolicy_g = os.Getenv("LBPolicy")
	}
	switch DefaultLBPolicy_g {
	case "Random":
		return Random(svc)
	case "LeastConn":
		return LeastConn(svc)
	case "MLeastConn":
		return MLeastConn(svc)
	case "Netflix":
		return Netflix(svc)
	default:
		return nil, errors.New("no endpoint found")
	}
}
