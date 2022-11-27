package utils

import (
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

var DeployChan=make(chan *appv1.Deployment)
var PodChan=make(chan *corev1.Pod)