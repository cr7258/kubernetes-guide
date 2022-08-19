package helper

import (
	"context"
	"fmt"
	v1 "jtapp/api/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetRedisPodNames(redisConfig *v1.Redis) []string {
	// 根据副本数生成 pod 名称
	podNames := make([]string, redisConfig.Spec.Num)
	for i := 0; i < redisConfig.Spec.Num; i++ {
		podNames[i] = fmt.Sprintf("%s-%d", redisConfig.Name, i)
	}
	fmt.Println("podnames:", podNames)
	return podNames

}

func IsExist(podName string, redis *v1.Redis) bool {
	for _, po := range redis.Finalizers {
		if podName == po {
			return true
		}
	}
	return false
}

func CreateRedis(client client.Client, redisConfig *v1.Redis, podName string) (string, error) {
	if IsExist(podName, redisConfig) {
		return "", nil
	}
	newpod := &corev1.Pod{}
	newpod.Name = podName
	newpod.Namespace = redisConfig.Namespace
	newpod.Spec.Containers = []corev1.Container{
		{
			Name:            podName,
			Image:           "redis:5-alpine",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Ports: []corev1.ContainerPort{
				{
					ContainerPort: int32(redisConfig.Spec.Port),
				},
			},
		},
	}
	return podName, client.Create(context.Background(), newpod)
}
