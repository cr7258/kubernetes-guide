package main

import (
	"context"
	"fmt"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"log"
	"strings"
)

var body string = `
{
  "apiVersion": "admission.k8s.io/v1",
  "kind": "AdmissionReview",
  "request": {
    "uid": "705ab4f5-6393-11e8-b7cc-42010a800002",
    "kind": {"group":"","version":"v1","kind":"pods"},
    "resource": {"group":"","version":"v1","resource":"pods"},
    "name": "mypod",
    "namespace": "default",
    "operation": "CREATE",
    "object": {"apiVersion":"v1","kind":"Pod","metadata":{"name":"shenyi","namespace":"default"}},
    "userInfo": {
      "username": "admin",
      "uid": "014fbff9a07c",
      "groups": ["system:authenticated","my-admin-group"],
      "extra": {
        "some-key":["some-value1", "some-value2"]
      }
    },
    "dryRun": false
  }
}
`

func main() {
	mainconfig := &rest.Config{
		Host: "http://localhost:8080",
	}
	c, err := kubernetes.NewForConfig(mainconfig)
	if err != nil {
		log.Fatal(err)
	}
	result := c.AdmissionregistrationV1().RESTClient().Post().Body(strings.NewReader(body)).
		Do(context.Background())
	b, _ := result.Raw()
	fmt.Println(string(b))

}
