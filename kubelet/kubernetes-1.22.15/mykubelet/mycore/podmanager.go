package mycore

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	"k8s.io/kubernetes/pkg/kubelet/config"
	"k8s.io/kubernetes/pkg/kubelet/configmap"
	kubepod "k8s.io/kubernetes/pkg/kubelet/pod"
	"k8s.io/kubernetes/pkg/kubelet/secret"
	kubetypes "k8s.io/kubernetes/pkg/kubelet/types"
)

/**
* @description 构建 Pod 缓存
* @author chengzw
* @since 2023/5/26
* @link
 */

// 就是官方的 PodManager 做一些改造
type PodCache struct {
	client     *kubernetes.Clientset
	PodManager kubepod.Manager
	PodConfig  *config.PodConfig
}

func NewPodCache(client *kubernetes.Clientset, nodeName string) *PodCache {
	ch := make(chan struct{})
	fact := informers.NewSharedInformerFactory(client, 0)
	fact.Core().V1().Nodes().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	fact.Start(ch)

	nodeLister := fact.Core().V1().Nodes().Lister()
	mirrorPodClient := kubepod.NewBasicMirrorClient(client, nodeName, nodeLister)
	secretManager := secret.NewSimpleSecretManager(client)
	configMapManager := configmap.NewSimpleConfigMapManager(client)
	podManager := kubepod.NewBasicPodManager(mirrorPodClient, secretManager, configMapManager)

	return &PodCache{
		client:     client,
		PodManager: podManager,
		PodConfig:  newPodConfig(nodeName, client, fact),
	}
}

// 创建 PodConfig
func newPodConfig(nodeName string, client *kubernetes.Clientset,
	fact informers.SharedInformerFactory) *config.PodConfig {
	eventBroadcaster := record.NewBroadcaster() // 事件分发器 广播
	eventRecorder := eventBroadcaster.NewRecorder(legacyscheme.Scheme, v1.EventSource{Component: "kubelet", Host: nodeName})

	cfg := config.NewPodConfig(config.PodConfigNotificationIncremental, eventRecorder)

	config.NewSourceApiserver(client, types.NodeName(nodeName),
		func() bool {
			return fact.Core().V1().Nodes().Informer().HasSynced()
		}, cfg.Channel(kubetypes.ApiserverSource))
	return cfg
}
