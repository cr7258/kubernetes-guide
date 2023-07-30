* [Controller Runtime 深入学习和源码理解篇](#controller-runtime-深入学习和源码理解篇)
  * [Controller Runtime 架构](#controller-runtime-架构)
  * [让 client 绕过缓存读取资源](#让-client-绕过缓存读取资源)
  * [手工初始化 Scheme](#手工初始化-scheme)
  * [查看 Manager 中的 Informer](#查看-manager-中的-informer)
  * [半手工创建 Controller](#半手工创建-controller)
  * [手工触发 Reconcile 函数](#手工触发-reconcile-函数)
  * [工作队列](#工作队列)
    * [启动工作队列](#启动工作队列)
    * [手工处理队列数据](#手工处理队列数据)
    * [限速队列](#限速队列)
    * [控制器的限流队列](#控制器的限流队列)
  * [控制器并发设置](#控制器并发设置)
  * [Owner 资源监听](#owner-资源监听)
  * [参考资料](#参考资料)

# Controller Runtime 深入学习和源码理解篇

## Controller Runtime 架构

![](https://chengzw258.oss-cn-beijing.aliyuncs.com/Article/20230729145536.png)

Controller Runtime 可以分别两个部分：
- Manager：
  - 1.创建 client (和 K8S APIServer 交互 。如 create、delete 操作)，其他创建和配置的还有 metrics、设置日志对象、选主等等。 
  - 2.根据预设好的 Map(schema)中 gvk，创建各个 Informer 对资源进行 List & Watch。 
  - 3.做控制器的初始化工作。并传递 client 等对象给控制器。
- Controller：
  - 1.监听事件并放入到队列里。
  - 2.从队列取出事件，触发 Reconcile 函数。

## 让 client 绕过缓存读取资源

在 controller-runtime（v0.12.0 版本是可以的，v0.15.0 版本发现不可以）可以在创建 Manager 的时候设置 NewClient 来绕过缓存。 

```go
mgr, err := manager.New(lib.K8sRestConfig(),
    manager.Options{
        Logger: logf.Log.WithName("test"),
        NewClient: func(cache cache.Cache, config *rest.Config, options client.Options, uncachedObjects ...client.Object) (client.Client, error) {
            return cluster.DefaultNewClient(cache, config, options, &v1.Pod{}) // &v1.Pod{} 资源不从缓存读取
    },
})
```

## 手工初始化 Scheme

对于 Kubernetes 内置的资源，在创建 manager 会自动进行注册，源码参见：
- sigs.k8s.io/controller-runtime@v0.15.0/pkg/cluster/cluster.go

```go
func setOptionsDefaults(options Options, config *rest.Config) (Options, error) {
  ......
  // Use the Kubernetes client-go scheme if none is specified
  if options.Scheme == nil {
	  options.Scheme = scheme.Scheme
  }
  ......
}
```

- k8s.io/client-go@v0.27.4/kubernetes/scheme/register.go

```go
var Scheme = runtime.NewScheme()
var Codecs = serializer.NewCodecFactory(Scheme)
var ParameterCodec = runtime.NewParameterCodec(Scheme)
// 一堆写死的内置的 Scheme
var localSchemeBuilder = runtime.SchemeBuilder{
	admissionregistrationv1.AddToScheme,
	admissionregistrationv1alpha1.AddToScheme,
	admissionregistrationv1beta1.AddToScheme,
	internalv1alpha1.AddToScheme,
	appsv1.AddToScheme,
	appsv1beta1.AddToScheme,
	appsv1beta2.AddToScheme,
	authenticationv1.AddToScheme,
	authenticationv1alpha1.AddToScheme,
	authenticationv1beta1.AddToScheme,
	authorizationv1.AddToScheme,
	authorizationv1beta1.AddToScheme,
	autoscalingv1.AddToScheme,
	autoscalingv2.AddToScheme,
	autoscalingv2beta1.AddToScheme,
	autoscalingv2beta2.AddToScheme,
	batchv1.AddToScheme,
	batchv1beta1.AddToScheme,
	certificatesv1.AddToScheme,
	certificatesv1beta1.AddToScheme,
	certificatesv1alpha1.AddToScheme,
	coordinationv1beta1.AddToScheme,
	coordinationv1.AddToScheme,
	corev1.AddToScheme,
	discoveryv1.AddToScheme,
	discoveryv1beta1.AddToScheme,
	eventsv1.AddToScheme,
	eventsv1beta1.AddToScheme,
	extensionsv1beta1.AddToScheme,
	flowcontrolv1alpha1.AddToScheme,
	flowcontrolv1beta1.AddToScheme,
	flowcontrolv1beta2.AddToScheme,
	flowcontrolv1beta3.AddToScheme,
	networkingv1.AddToScheme,
	networkingv1alpha1.AddToScheme,
	networkingv1beta1.AddToScheme,
	nodev1.AddToScheme,
	nodev1alpha1.AddToScheme,
	nodev1beta1.AddToScheme,
	policyv1.AddToScheme,
	policyv1beta1.AddToScheme,
	rbacv1.AddToScheme,
	rbacv1beta1.AddToScheme,
	rbacv1alpha1.AddToScheme,
	resourcev1alpha2.AddToScheme,
	schedulingv1alpha1.AddToScheme,
	schedulingv1beta1.AddToScheme,
	schedulingv1.AddToScheme,
	storagev1beta1.AddToScheme,
	storagev1.AddToScheme,
	storagev1alpha1.AddToScheme,
}

// AddToScheme adds all types of this clientset into the given scheme. This allows composition
// of clientsets, like in:
//
//	import (
//	  "k8s.io/client-go/kubernetes"
//	  clientsetscheme "k8s.io/client-go/kubernetes/scheme"
//	  aggregatorclientsetscheme "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset/scheme"
//	)
//
//	kclientset, _ := kubernetes.NewForConfig(c)
//	_ = aggregatorclientsetscheme.AddToScheme(clientsetscheme.Scheme)
//
// After this, RawExtensions in Kubernetes types will serialize kube-aggregator types
// correctly.
var AddToScheme = localSchemeBuilder.AddToScheme

func init() {
	v1.AddToGroupVersion(Scheme, schema.GroupVersion{Version: "v1"})
	utilruntime.Must(AddToScheme(Scheme))
}
```

对于自定义 CRD 资源，需要通过 AddToScheme 对资源进行注册，例如：

```go
mgr, _ := manager.New(config.GetConfigOrDie(), manager.Options{})
taskv1alpha1.SchemeBuilder.AddToScheme(mgr.GetScheme())
```

## 查看 Manager 中的 Informer

Informer 创建流程：

```go
manager.New -->

// sigs.k8s.io/controller-runtime@v0.15.0/pkg/manager/manager.go
cluster.New -->

// sigs.k8s.io/controller-runtime@v0.15.0/pkg/cluster/cluster.go
// 186 行
options, err := setOptionsDefaults(options, config)

// 316 行
cach.New -->

if options.NewCache == nil {
    options.NewCache = cache.New
}

// sigs.k8s.io/controller-runtime@v0.15.0/pkg/cache/cache.go
// 225 行
internal.NewInformers -->

return &informerCache{
  scheme: opts.Scheme,
  Informers: internal.NewInformers(config, &internal.InformersOpts{
  HTTPClient:   opts.HTTPClient,
  Scheme:       opts.Scheme,
  Mapper:       opts.Mapper,
  ResyncPeriod: *opts.SyncPeriod,
  Namespace:    opts.Namespaces[0],
  ByGVK:        byGVK,
  }),
}, nil

// sigs.k8s.io/controller-runtime@v0.15.0/pkg/cache/internal/informers.go
// 60 行
func NewInformers(config *rest.Config, options *InformersOpts) *Informers {
  return &Informers{
      config:     config,
      httpClient: options.HTTPClient,
      scheme:     options.Scheme,
      mapper:     options.Mapper,
      tracker: tracker{
          Structured:   make(map[schema.GroupVersionKind]*Cache),
          Unstructured: make(map[schema.GroupVersionKind]*Cache),
          Metadata:     make(map[schema.GroupVersionKind]*Cache),
    },
    codecs:     serializer.NewCodecFactory(options.Scheme),
    paramCodec: runtime.NewParameterCodec(options.Scheme),
    resync:     options.ResyncPeriod,
    startWait:  make(chan struct{}),
    namespace:  options.Namespace,
    byGVK:      options.ByGVK,
    }
}

// sigs.k8s.io/controller-runtime@v0.15.0/pkg/cache/internal/informers.go
// 81 行
// 其中 Cache 中包含了真正的 SharedIndexInformer
// Cache contains the cached data for an Cache.
  type Cache struct {
  // Informer is the cached informer
  Informer cache.SharedIndexInformer
  
  // CacheReader wraps Informer and implements the CacheReader interface for a single type
  Reader CacheReader
}

// sigs.k8s.io/controller-runtime@v0.15.0/pkg/manager/manager.go
// 487 行
// 最后创建 runnables，并返回 controllerManager
runnables := newRunnables(options.BaseContext, errChan)

return &controllerManager{
  stopProcedureEngaged:          pointer.Int64(0),
  cluster:                       cluster,
  runnables:                     runnables,
  errChan:                       errChan,
  recorderProvider:              recorderProvider,
  resourceLock:                  resourceLock,
  metricsListener:               metricsListener,
  metricsExtraHandlers:          metricsExtraHandlers,
  controllerConfig:              options.Controller,
  logger:                        options.Logger,
  elected:                       make(chan struct{}),
  webhookServer:                 options.WebhookServer,
  leaderElectionID:              options.LeaderElectionID,
  leaseDuration:                 *options.LeaseDuration,
  renewDeadline:                 *options.RenewDeadline,
  retryPeriod:                   *options.RetryPeriod,
  healthProbeListener:           healthProbeListener,
  readinessEndpointName:         options.ReadinessEndpointName,
  livenessEndpointName:          options.LivenessEndpointName,
  pprofListener:                 pprofListener,
  gracefulShutdownTimeout:       *options.GracefulShutdownTimeout,
  internalProceduresStop:        make(chan struct{}),
  leaderElectionStopped:         make(chan struct{}),
  leaderElectionReleaseOnCancel: options.LeaderElectionReleaseOnCancel,
}, nil
```

Informer 启动流程：

```go
mgr.Start -->
	
// sigs.k8s.io/controller-runtime@v0.15.0/pkg/manager/internal.go
func (cm *controllerManager) Start(ctx context.Context) (err error) {
......
    // First start any webhook servers, which includes conversion, validation, and defaulting
    // webhooks that are registered.
    //
    // WARNING: Webhooks MUST start before any cache is populated, otherwise there is a race condition
    // between conversion webhooks and the cache sync (usually initial list) which causes the webhooks
    // to never start because no cache can be populated.
    if err := cm.runnables.Webhooks.Start(cm.internalCtx); err != nil {
        if err != nil {
            return fmt.Errorf("failed to start webhooks: %w", err)
        }
    }

	// Start and wait for caches.
	// 启动 Informer
	if err := cm.runnables.Caches.Start(cm.internalCtx); err != nil {
		if err != nil {
			return fmt.Errorf("failed to start caches: %w", err)
		}
	}

	// Start the non-leaderelection Runnables after the cache has synced.
	if err := cm.runnables.Others.Start(cm.internalCtx); err != nil {
		if err != nil {
			return fmt.Errorf("failed to start other runnables: %w", err)
		}
	}
......
}
```

## 半手工创建 Controller

```bash
// 会通过 mgr.Add(c) 将 controller 添加到 runnables 中
ctl, err := controller.New("abc", mgr, controller.Options{
    Reconciler: &lib.Ctl{}, // struct 需要实现 Reconcile 方法
})
lib.Check(err)

src := source.Kind(mgr.GetCache(), &v1.Pod{})
hdler := &handler.EnqueueRequestForObject{}
lib.Check(ctl.Watch(src, hdler))
```

实现 Reconcile 方法：

```go
type Ctl struct {}

func (*Ctl) Reconcile(ctx context.Context, req controllerruntime.Request) (controllerruntime.Result, error) {
	fmt.Println(req.NamespacedName)
	return controllerruntime.Result{}, nil
}
```

## 手工触发 Reconcile 函数

```go
cd controller-runtime@v0.15.0/pkg/test
go run 7.reconcile.go

# 访问 http://localhost:8081/add 后，日志输出
my-namespace/my-pod
```

浏览器访问 http://localhost:8081/add ，会调用 gin 函数，然后往 Queue 中添加 Event 来手动触发 Reconcile 函数。

```go
// 实现 Runnable 接口
func (m *MyWeb) Start(ctx context.Context) error {
	r := gin.New()
	r.GET("/add", func(c *gin.Context) {
		pod := &v1.Pod{}
		pod.Name = "my-pod"
		pod.Namespace = "my-namespace"
		// 往 Queue 中添加 Event 来手动触发 Reconcile 函数
		m.h.Create(context.TODO(), event.CreateEvent{Object: pod}, m.ctl.(*cc.Controller).Queue)
	})
	return r.Run(":8081")
}
```

## 工作队列

client-go 中 util/workqueue，特性：
- 1.Fair：先到先处理。
- 2.Stingy：同一事件在处理之前多次被添加，只处理一次。  
- 3.支持多消费者和生产者，支持运行时重新入队。
- 4.关闭通知。

EventHandler 将事件添加到 Queue 中从而触发 Reconcile 函数的执行。

```go
// sigs.k8s.io/controller-runtime@v0.15.0/pkg/handler/eventhandler.go
type EventHandler interface {
	// Create is called in response to an create event - e.g. Pod Creation.
	Create(context.Context, event.CreateEvent, workqueue.RateLimitingInterface)

	// Update is called in response to an update event -  e.g. Pod Updated.
	Update(context.Context, event.UpdateEvent, workqueue.RateLimitingInterface)

	// Delete is called in response to a delete event - e.g. Pod Deleted.
	Delete(context.Context, event.DeleteEvent, workqueue.RateLimitingInterface)

	// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
	// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
	Generic(context.Context, event.GenericEvent, workqueue.RateLimitingInterface)
}

// sigs.k8s.io//controller-runtime@v0.15.0/pkg/handler/enqueue.go
func (e *EnqueueRequestForObject) Create(ctx context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
    if evt.Object == nil {
        enqueueLog.Error(nil, "CreateEvent received with no metadata", "event", evt)
        return
    }
    q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
        Name:      evt.Object.GetName(),
        Namespace: evt.Object.GetNamespace(),
    }})
}
```

### 启动工作队列

启动工作队列：

```bash
go run 9.workqueue.go

# 输出结果
# 同一事件在处理之前多次被添加，只处理一次
default/abc
default/abc2
```

### 手工处理队列数据

上一小节中虽然我们插入多个 default/abc，但是实际上只会打印一次，这是因为我们并没有处理这个事件。workqueue 会保证同一事件在处理之前多次被添加，只处理一次。
原因是在源码中会判断如果 dirty 中有这个事件，直接跳过。

```go
// k8s.io/client-go@v0.27.4/util/workqueue/queue.go
func (q *Type) Add(item interface{}) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	if q.shuttingDown {
		return
	}

  // 如果 dirty 中有这个事件，直接跳过
	if q.dirty.has(item) {
		return
	}

	q.metrics.add(item)

	q.dirty.insert(item)
	if q.processing.has(item) {
		return
	}

	q.queue = append(q.queue, item)
	q.cond.Signal()
}
```

在执行 q.Get() 获取事件时，由于第一次的 default/abc 事件 Get 之后，queue 的长度会变成 0 (q.queue = q.queue[1:])，因此会阻塞在 423 行处。

```go
// Get blocks until it can return an item to be processed. If shutdown = true,
// the caller should end their goroutine. You must call Done with item when you
// have finished processing it.
func (q *Type) Get() (item interface{}, shutdown bool) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

  // 阻塞
	for len(q.queue) == 0 && !q.shuttingDown {
		q.cond.Wait()
	}
	if len(q.queue) == 0 {
		// We must be shutting down.
		return nil, true
	}

	item = q.queue[0]
	// The underlying array still exists and reference this object, so the object will not be garbage collected.
	q.queue[0] = nil
	q.queue = q.queue[1:]

	q.metrics.get(item)

	q.processing.insert(item)
	q.dirty.delete(item)

	return item, false
}
```

一旦执行 q.Done() 会从 dirty 中删除事件，并且唤醒阻塞的 gorouting。

```go
// Done marks item as done processing, and if it has been marked as dirty again
// while it was being processed, it will be re-added to the queue for
// re-processing.
func (q *Type) Done(item interface{}) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	q.metrics.done(item)

	q.processing.delete(item)
	if q.dirty.has(item) {
		q.queue = append(q.queue, item)
		q.cond.Signal()
	} else if q.processing.len() == 0 {
		q.cond.Signal()
	}
}
```

### 限速队列

在这个基本队列基础上，官方又扩展了 2 个队列：
- 1.限速队列，简单来说就是：通过一些限速算法(令牌桶 go 官方的) 让数据延迟 xxx 时间后再加入队列。
- 2.延迟队列，实现上面的 “延迟 xxx 时间后加入的功能”。

其中限速队列实现了延迟队列。

```go
// k8s.io/client-go@v0.27.4/util/workqueue/rate_limiting_queue.go
// RateLimitingInterface is an interface that rate limits items being added to the queue.
type RateLimitingInterface interface {
	DelayingInterface

	// AddRateLimited adds an item to the workqueue after the rate limiter says it's ok
	AddRateLimited(item interface{})

	// Forget indicates that an item is finished being retried.  Doesn't matter whether it's for perm failing
	// or for success, we'll stop the rate limiter from tracking it.  This only clears the `rateLimiter`, you
	// still have to call `Done` on the queue.
	Forget(item interface{})

	// NumRequeues returns back how many times the item was requeued
	NumRequeues(item interface{}) int
}


// k8s.io/client-go@v0.27.2/util/workqueue/rate_limiting_queue.go
// AddRateLimited AddAfter's the item based on the time when the rate limiter says it's ok
func (q *rateLimitingType) AddRateLimited(item interface{}) {
	q.DelayingInterface.AddAfter(item, q.rateLimiter.When(item))
}

// k8s.io/client-go@v0.27.2/util/workqueue/default_rate_limiters.go
// q.rateLimiter.When 方法其实最终调用了 Limiter.Reserve().Delay() 来等待时间
func (r *BucketRateLimiter) When(item interface{}) time.Duration {
    return r.Limiter.Reserve().Delay()
}

// k8s.io/client-go@v0.27.2/util/workqueue/delaying_queue.go
// 等待时间到了以后会执行 AddAfter 方法将事件添加到队列中 
// AddAfter adds the given item to the work queue after the given delay
func (q *delayingType) AddAfter(item interface{}, duration time.Duration) {
	// don't add if we're already shutting down
	if q.ShuttingDown() {
		return
	}

	q.metrics.retry()

	// immediately add things with no delay
	if duration <= 0 {
		q.Add(item)
		return
	}

	select {
	case <-q.stopCh:
		// unblock if ShutDown() is called
	case q.waitingForAddCh <- &waitFor{data: item, readyAt: q.clock.Now().Add(duration)}:
	}
}
```

### 控制器的限流队列

```go
// sigs.k8s.io/controller-runtime@v0.15.0/pkg/controller/controller.go
// RateLimiter 可以通过 controller.Options 来设置，如果没设置默认使用 DefaultControllerRateLimiter()
if options.RateLimiter == nil {
    options.RateLimiter = workqueue.DefaultControllerRateLimiter()
}

// client-go@v0.27.4/util/workqueue/default_rate_limiters.go
// NewMaxOfRateLimiter 是一个包装器，会使用所有 RateLimiter 中最大的延迟作为等待时间
// NewItemExponentialFailureRateLimiter 有一个计算公式，如果失败的次数越多，等待的时间越长
// 默认的 BucketRateLimiter 的设置是每秒放 10 个令牌，最多 100 个令牌
// DefaultControllerRateLimiter is a no-arg constructor for a default rate limiter for a workqueue.  It has
// both overall and per-item rate limiting.  The overall is a token bucket and the per-item is exponential
func DefaultControllerRateLimiter() RateLimiter {
	return NewMaxOfRateLimiter(
		NewItemExponentialFailureRateLimiter(5*time.Millisecond, 1000*time.Second),
		// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
		&BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
	)
}
```

## 控制器并发设置

默认情况下 Reconcile 是单线程执行的，如果想要并发执行，可以通过设置 Options 的 MaxConcurrentReconciles 来实现。

```go
ctl, err := controller.New("abc", mgr, controller.Options{
    Reconciler: &lib.Ctl{}, // struct 需要实现 Reconcile 方法
    MaxConcurrentReconciles: 2 // 2 个并发
})
``` 

如果是通过 builder 来创建控制器，可以通过 WithOptions 来设置。

```go
builder.ControllerManagedBy(mgr).
    For(&v1.Pod{}).
    WithOptions(controller.Options {
        MaxConcurrentReconciles: 2,
}).Complete(myController)
```

## Owner 资源监听

可以通过 controllerutil.SetOwnerReference 设置资源的 OwnerReference。

```go
func (m *MyWebOwner) Start(ctx context.Context) error {
	r := gin.New()
	r.GET("/add", func(c *gin.Context) {
		name := c.Query("name")
		cm := &v1.ConfigMap{}
		cm.Name = name
		cm.Namespace = "mytest"

		// 设置 Configmap 的 OwnerReference 为 Pod
		controllerutil.SetOwnerReference(m.ownObj.(metav1.Object), cm, m.scheme)

		m.hdler.Create(context.TODO(), event.CreateEvent{Object: cm},
			m.ctl.(*cc.Controller).Queue)
	})
	return r.Run(":8081")
}
```

如果是通过 builder 来创建控制器，可以通过 Owns 来设置。

```go
builder.ControllerManagedBy(mgr).
    For(&v1.Pod{}).
    Owns(&v1.ConfigMap{}).
    Complete(myController)
```

当通过 EventHandler.Create 方法往队列中添加事件时，先会判断该资源的 OwnerReference 是否为我们监听的资源，如果是则会添加到队列中。

```go
// Create implements EventHandler.
func (e *enqueueRequestForOwner) Create(ctx context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	reqs := map[reconcile.Request]empty{}
	e.getOwnerReconcileRequest(evt.Object, reqs)
	for req := range reqs {
		q.Add(req)
	}
}
```

执行以下命令自动我们控制器。

```bash
cd controller-runtime@v0.15.0/pkg/test
go run 15.ownerreference.go
```

浏览器输入 http://localhost:8081?name=mycm，可以看到日志输出：

```go
mytest/mycm
```

## 参考资料
- [Kubernetes Operator series - controller-runtime](https://nakamasato.medium.com/kubernetes-operator-series-1-controller-runtime-aa50d1d93c5c)
- [[K8S] controller-runtime 源码浅酌](https://juejin.cn/post/7136274018100838407)