package helper

import (
	"context"
	"fmt"
	v1 "jtapp/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

// 判断redis pod 是否 能获取
func IsExistPod(podName string, redis *v1.Redis, client client.Client) bool {
	err := client.Get(context.Background(),
		types.NamespacedName{Namespace: redis.Namespace, Name: podName}, &corev1.Pod{})
	if err != nil {
		return false
	}
	return true

}
func CreateRedis(client client.Client, redisConfig *v1.Redis, podName string, schema *runtime.Scheme) (string, error) {
	//if  IsExistInFinalizers(podName, redisConfig) {
	//	return "", nil
	//}
	if IsExistPod(podName, redisConfig, client) { //如果Pod已经存在，则不处理
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
	// 设置 OwnerReference
	err := controllerutil.SetControllerReference(redisConfig, newpod, schema)
	if err != nil {
		return "", err
	}
	//return podName, client.Create(context.Background(), newpod)
	err = client.Create(context.Background(), newpod)
	if err != nil {
		return podName, err
	}

	return podName, nil
}
