package controllercomm

import (
	"encoding/json"
	"io/ioutil"
	"local/Abhinay/globals"
	"log"
	"net/http"
	"time"
)

func GetEndpoints(svc string) {
	// log.Println("Service name: ", svc)
	req, err := http.NewRequest("GET", "http://epwatcher:62000/"+svc, nil)
	if err != nil {
		log.Println("Error reading request:", err)
	}

	req.Header.Set("Cache-Control", "no-cache")

	client := &http.Client{Timeout: time.Second * 10}

	resp, err := client.Do(req)
	if err != nil {
		log.Println("error getting response:", err.Error())
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("error reading response:", err.Error())
		return
	}

	var ep globals.Endpoints
	err = json.Unmarshal(body, &ep)
	if err != nil {

		log.Println("error json unmarshalling:", err.Error())
		return
	}
	globals.Endpoints_g.Put(svc, ep.Ips)
	//globals.Svc2BackendSrvMap_g.UpdateMap(svc, ep.Ips)
}

func getAllEndpoints() {
	if len(globals.SvcList_g) > 0 {
		for _, svc := range globals.SvcList_g {
			GetEndpoints(svc)
		}
	}
}

func RunComm(done chan bool) {
	ticker := time.NewTicker(time.Millisecond * 10)
	for {
		select {
		case <-ticker.C:
			getAllEndpoints()

		case <-done:
			return
		}
	}
}
