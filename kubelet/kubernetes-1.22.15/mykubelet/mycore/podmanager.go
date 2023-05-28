package mycore

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	"k8s.io/kubernetes/pkg/kubelet/config"
	"k8s.io/kubernetes/pkg/kubelet/configmap"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
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
	client        *kubernetes.Clientset
	PodManager    kubepod.Manager
	PodWorkers    PodWorkers
	PodConfig     *config.PodConfig
	Clock         clock.RealClock     //时钟对象
	InnerPodCache kubecontainer.Cache //内部 POD 对象 。存的是 POD 和 状态之间的对应关系
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

	cl := clock.RealClock{}
	//下面是创建PodWorker 对象 。 注意：使用的是自己的。 源码里是私有没法调用
	eventBroadcaster := record.NewBroadcaster() // 事件分发器 广播
	eventRecorder := eventBroadcaster.NewRecorder(legacyscheme.Scheme, v1.EventSource{Component: "kubelet", Host: nodeName})

	innerPodCache := kubecontainer.NewCache() // 内部 podcache 用于记录pod和状态的对应关系

	pw := NewPodWorkers(innerPodCache, eventRecorder, cl)
	return &PodCache{
		Clock:         cl,
		client:        client,
		PodManager:    podManager,
		PodConfig:     newPodConfig(nodeName, client, fact, eventRecorder),
		PodWorkers:    pw,
		InnerPodCache: innerPodCache,
	}
}

// 创建PodConfig
func newPodConfig(nodeName string, client *kubernetes.Clientset,
	fact informers.SharedInformerFactory, recorder record.EventRecorder) *config.PodConfig {

	cfg := config.NewPodConfig(config.PodConfigNotificationIncremental, recorder)

	config.NewSourceApiserver(client, types.NodeName(nodeName),
		func() bool {
			return fact.Core().V1().Nodes().Informer().HasSynced()
		}, cfg.Channel(kubetypes.ApiserverSource))
	return cfg
}
