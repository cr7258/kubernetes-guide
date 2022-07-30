package pkg

import v1 "k8s.io/api/core/v1"

type User struct {
	Id   int
	Name string
}

var Pod struct {
	v1.Pod
}
