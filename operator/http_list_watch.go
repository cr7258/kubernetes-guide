package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const token = "eyJhbGciOiJSUzI1NiIsImtpZCI6ImFRM2J0Z3NmUk1hR2VhV2VRbE5vbkVHbGRSMUIwdEdTU3ZPb21TSXEtMkUifQ.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJkZWZhdWx0Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9zZWNyZXQubmFtZSI6ImFwaS1leHBsb3Jlci10b2tlbi02enMycSIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VydmljZS1hY2NvdW50Lm5hbWUiOiJhcGktZXhwbG9yZXIiLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC51aWQiOiJmNzMwNDZhYS1jYTcyLTQ0ZjAtODMzNy0zYzk4NWY1NjJkNmYiLCJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6ZGVmYXVsdDphcGktZXhwbG9yZXIifQ.ZK6O4ss4qn2qwvw315jjnyvva8EnUszfXDH6vpxa-R5nxbD3t1pDN5us0AYZEkLfTPgDYc9DsKFUmkWCum7AIpAqB79bM8p7NNNDiU5V-DphwT9BAAJqSG2UKhzHtxyY4rzwdKs5n2gVIWGYytmgUYffbkltAMWMJcT7sVUQRMDS3m4we_GS8MDl1mNLzghmPqfcBQKRKJNS0JCjLpdexYZaqw79e4HSa_sMh02P_azWiJWxhDvT-VZPJELmkiwpV6named87SMijBd6EIIu3IOFAa7mqCKzNtp8AJQSc-Ey53AkQlH_7BGRuyfNqx16lhE3ioBbk0NVQkKwVwONkw"
const apiServer = "https://127.0.0.1:55429"

type Pod struct {
	Metadata struct {
		Name              string    `json:"name"`
		Namespace         string    `json:"namespace"`
		CreationTimestamp time.Time `json:"creationTimestamp"`
	} `json:"metadata"`
}

type Event struct {
	EventType string `json:"type"`
	Object    Pod    `json:"object"`
}

func main() {
	// create an HTTP client with authorization token or certificate
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // only use this for testing purposes
			},
		},
	}
	req, err := http.NewRequest("GET", apiServer+"/api/v1/namespaces/default/pods?watch=true",
		nil)
	if err != nil {
		panic(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	// send the initial request to list all pods
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	var event Event
	decoder := json.NewDecoder(resp.Body)

	// read the response and parse event
	for {
		if err := decoder.Decode(&event); err != nil {
			panic(err)
		}
		fmt.Printf("%s Pod %s \n", event.EventType, event.Object.Metadata.Name)
	}
}
