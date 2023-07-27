* [Informer 深入学习篇](#informer-深入学习篇)
    * [Informer 架构图](#informer-架构图)
    * [DeltaFIFO](#deltafifo)
    * [Reflector](#reflector)
        * [List &amp; Watch](#list--watch)
        * [手工创建 Reflector 和队列取值](#手工创建-reflector-和队列取值)
        * [Indexer](#indexer)
    * [实现简单的 Informer](#实现简单的-informer)
    * [SharedInformer](#sharedinformer)
        * [手工模拟简单 SharedInformer](#手工模拟简单-sharedinformer)
        * [手工模拟简单 SharedInformer，加入 List &amp; Watch](#手工模拟简单-sharedinformer加入-list--watch)
        * [Indexer 索引构建](#indexer-索引构建)
        * [将 Indexer 集成到自己的 Informer 中](#将-indexer-集成到自己的-informer-中)
    * [SharedInformerFactory](#sharedinformerfactory)
    * [参考资料](#参考资料)

# Informer 深入学习篇

## Informer 架构图

![](https://chengzw258.oss-cn-beijing.aliyuncs.com/Article/20230727100407.png)

参考资料：
- [client-go under the hood](https://github.com/kubernetes/sample-controller/blob/master/docs/controller-client-go.md)

## DeltaFIFO

```go
go run deltafifo/main.go

# 返回结果，只可以取到最新的对象 pod1，旧值需要去 Indexer 里取
Added pod1  value: 1
执行新增回调
Updated pod1  value: 1.1
执行修改回调
Deleted pod1  value: 1.1
执行删除回调
```

## Reflector

### List & Watch

```go
go run reflector/listwatch.go

# List
quickstart-es-default-0
robusta-forwarder-5f65fd6fbb-s2trs
robusta-runner-567655bf5b-v4xvj
wiremock-84d49c989c-zg4ls

# Watch，一开始会打印所有的 Pod
ADDED : robusta-forwarder-5f65fd6fbb-s2trs , Status:  Running
ADDED : wiremock-84d49c989c-zg4ls , Status:  Running
ADDED : quickstart-es-default-0 , Status:  Running
ADDED : robusta-runner-567655bf5b-v4xvj , Status:  Running

# 创建一个新的 Pod，然后删除
# kubectl run nettool --image=cr7258/nettool:v1
# kubectl delete pod nettool

ADDED : nettool , Status:  Pending
MODIFIED : nettool , Status:  Pending
MODIFIED : nettool , Status:  Pending
MODIFIED : nettool , Status:  Pending
MODIFIED : nettool , Status:  Running
MODIFIED : nettool , Status:  Running
MODIFIED : nettool , Status:  Running
MODIFIED : nettool , Status:  Running
MODIFIED : nettool , Status:  Running
DELETED : nettool , Status:  Running
```

### 手工创建 Reflector 和队列取值

```go
go run reflector/reflector.go

# 创建一个新的 Pod，然后删除
# kubectl run nettool --image=cr7258/nettool:v1
# kubectl delete pod nettool
# 注意删除 Pod 时并没有出现 Deleted 事件
# 在事件 Added, Updated 和 Deleted 时，informer 会从 DeltaFIFO 中 Pop 出对象，
# 同时还会把缓存同时存一份到 KnowObjects（indexer） 里，当更新或者删除时会取出 KnowObjects 的对象进行判断
# KnowObjects 是可以不设置的，一旦不设置，Deleted 事件就获取不到了

Added : nettool : Pending
Updated : nettool : Pending
Updated : nettool : Pending
Updated : nettool : Pending
Updated : nettool : Running

# 删除只出现 Updated
Updated : nettool : Running
Updated : nettool : Running
Updated : nettool : Running
Updated : nettool : Running
```

相关判断的代码在 client-go/tools/cache/delta_fifo.go 中：

```go
// Delete is just like Add, but makes a Deleted Delta. If the given
// object does not already exist, it will be ignored. (It may have
// already been deleted by a Replace (re-list), for example.)  In this
// method `f.knownObjects`, if not nil, provides (via GetByKey)
// _additional_ objects that are considered to already exist.
func (f *DeltaFIFO) Delete(obj interface{}) error {
	id, err := f.KeyOf(obj)
	if err != nil {
		return KeyError{obj, err}
	}
	f.lock.Lock()
	defer f.lock.Unlock()
	f.populated = true
	if f.knownObjects == nil {
		// 当从 DeltaFIFO 中 Pop 出对象时，items 就没有该对象了，如果没有存到 knownObjects 中，就会忽略该事件
		if _, exists := f.items[id]; !exists {
			// Presumably, this was deleted when a relist happened.
			// Don't provide a second report of the same deletion.
			return nil
		}
	} else {
		// We only want to skip the "deletion" action if the object doesn't
		// exist in knownObjects and it doesn't have corresponding item in items.
		// Note that even if there is a "deletion" action in items, we can ignore it,
		// because it will be deduped automatically in "queueActionLocked"
		_, exists, err := f.knownObjects.GetByKey(id)
		_, itemsExist := f.items[id]
		if err == nil && !exists && !itemsExist {
			// Presumably, this was deleted when a relist happened.
			// Don't provide a second report of the same deletion.
			return nil
		}
	}

	// exist in items and/or KnownObjects
	return f.queueActionLocked(Deleted, obj)
}
```

### Indexer

上面小节提到在删除 Pod 时并没有出现 Deleted 事件，要想获取 Deleted 事件，需要在 DeltaFIFO 设置 KnownObjects。

```go
df := cache.NewDeltaFIFOWithOptions(cache.DeltaFIFOOptions{
    KeyFunction:  cache.MetaNamespaceKeyFunc,
    KnownObjects: store, // 实现了 indexer
})
```

并且在消费 DeltaFIFO 时，把缓存添加到 Indexer 中。

```go
switch delta.Type {
case cache.Sync, cache.Added:
    store.Add(delta.Object)
case cache.Updated:
    store.Update(delta.Object)
case cache.Deleted:
    store.Delete(delta.Object)
}
```

运行程序。

```go
go run reflector/indexer.go

# 创建一个新的 Pod，然后删除
# kubectl run nettool --image=cr7258/nettool:v1
# kubectl delete pod nettool

Added : nettool : Pending
Updated : nettool : Pending
Updated : nettool : Pending
Updated : nettool : Pending
Updated : nettool : Running

# 删除事件
Updated : nettool : Running
Updated : nettool : Running
Updated : nettool : Running
Updated : nettool : Running
Deleted : nettool : Running
```

## 实现简单的 Informer

```go
go run myinformer.go

# 创建一个新的 Pod，然后删除
# kubectl run nettool --image=cr7258/nettool:v1
# kubectl delete pod nettool

OnAdd: nettool
OnUpdate: nettool
OnUpdate: nettool
OnUpdate: nettool
OnUpdate: nettool

# 删除事件
OnUpdate: nettool
OnUpdate: nettool
OnUpdate: nettool
OnDelete: nettool
```

## SharedInformer

![](https://chengzw258.oss-cn-beijing.aliyuncs.com/Article/20230727222809.png)

sharedIndexInformer 相比普通的 informer 来说, 可以共享 reflector 反射器, 业务代码可以注册多个 resourceEventHandler 方法, 无需重复创建 informer 做监听及事件注册.
如果相同资源实例化多个 informer, 那么每个 informer 都有一个 reflector 和 store. 不仅会有数据序列化的开销, 而且缓存 store 不能复用, 可能一个对象存在多个 informer 的 store 里.

SharedInformer（或者叫SharedIndexInformer）有以下特点：
- 1.支持多个 EventHandler，可以认为是支持多个消费者。
- 2.内置一个 Indexer，有一个叫做 threadSafeMap 的 struct 来实现 （源码位置 cache/thread_safe_store.go）
- 3.多个消费者之间共享了 Indexer。

SharedIndexInformer 的结构如下：

```go
// `*sharedIndexInformer` implements SharedIndexInformer and has three
// main components.  One is an indexed local cache, `indexer Indexer`.
// The second main component is a Controller that pulls
// objects/notifications using the ListerWatcher and pushes them into
// a DeltaFIFO --- whose knownObjects is the informer's local cache
// --- while concurrently Popping Deltas values from that fifo and
// processing them with `sharedIndexInformer::HandleDeltas`.  Each
// invocation of HandleDeltas, which is done with the fifo's lock
// held, processes each Delta in turn.  For each Delta this both
// updates the local cache and stuffs the relevant notification into
// the sharedProcessor.  The third main component is that
// sharedProcessor, which is responsible for relaying those
// notifications to each of the informer's clients.
type sharedIndexInformer struct {
    indexer    Indexer
    controller Controller
    
    processor             *sharedProcessor
    cacheMutationDetector MutationDetector
    
    listerWatcher ListerWatcher
    
    // objectType is an example object of the type this informer is
    // expected to handle.  Only the type needs to be right, except
    // that when that is `unstructured.Unstructured` the object's
    // `"apiVersion"` and `"kind"` must also be right.
    objectType runtime.Object
    
    // resyncCheckPeriod is how often we want the reflector's resync timer to fire so it can call
    // shouldResync to check if any of our listeners need a resync.
    resyncCheckPeriod time.Duration
    // defaultEventHandlerResyncPeriod is the default resync period for any handlers added via
    // AddEventHandler (i.e. they don't specify one and just want to use the shared informer's default
    // value).
    defaultEventHandlerResyncPeriod time.Duration
    // clock allows for testability
    clock clock.Clock
    
    started, stopped bool
    startedLock      sync.Mutex
    
    // blockDeltas gives a way to stop all event distribution so that a late event handler
    // can safely join the shared informer.
    blockDeltas sync.Mutex
    
    // Called whenever the ListAndWatch drops the connection with an error.
    watchErrorHandler WatchErrorHandler
}
```

其中包含 sharedProcessor，用于协调和管理处理器对象 processorListener (这是真正干活的对象)，负责将事件通知转发给相应的 informer client。

```go
// sharedProcessor has a collection of processorListener and can
// distribute a notification object to its listeners.  There are two
// kinds of distribute operations.  The sync distributions go to a
// subset of the listeners that (a) is recomputed in the occasional
// calls to shouldResync and (b) every listener is initially put in.
// The non-sync distributions go to every listener.
type sharedProcessor struct {
	listenersStarted bool
	listenersLock    sync.RWMutex
	listeners        []*processorListener
	syncingListeners []*processorListener
	clock            clock.Clock
	wg               wait.Group
}
```

processorListener 包含：
- 1.run  --- 阻塞运行
- 2.pop()  --- 好比不断从队列里取输出
- 3.lis.addCh ---   插入数据

```go
// 可以看到添加事件很简单，直接通过 addCh 这个通道接收，notification 就是我们所说的事件，也就是前面我们常说的 DeltaFIFO 输出的 Deltas。
//上面我们可以看到 addCh 是定义成的一个无缓冲通道，所以这个 add() 函数就是一个事件分发器，从 DeltaFIFO 中弹出的对象要逐一送到多个处理器，如果处理器没有及时处理 addCh 则会阻塞住
func (p *processorListener) add(notification interface{}) {
	p.addCh <- notification
}
```

cache 包中有些方法只能内部调用，因此我们将 client-go/tools/cache 目录拷贝到当前项目的根目录方便调用。

### 手工模拟简单 SharedInformer

执行以下代码，启动手工模拟的 SharedInformer。

```go
go run sharedinformer/simulate.go

# 输出结果
OnAdd:second pod0
OnAdd: pod0
OnAdd: pod1
OnAdd:second pod1
OnAdd: pod2
OnAdd:second pod2
OnAdd:second pod3
OnAdd: pod3
```

### 手工模拟简单 SharedInformer，加入 List & Watch

执行以下代码，启动手工模拟的 SharedInformer。

```go
go run sharedinformer/list_watch.go

# 输出结果
OnAdd:second robusta-runner-567655bf5b-v4xvj
OnAdd: robusta-runner-567655bf5b-v4xvj
OnAdd: wiremock-84d49c989c-zg4ls
OnAdd: quickstart-es-default-0
OnAdd:second wiremock-84d49c989c-zg4ls
OnAdd: robusta-forwarder-5f65fd6fbb-s2trs
OnAdd:second quickstart-es-default-0
OnAdd:second robusta-forwarder-5f65fd6fbb-s2trs
```

### Indexer 索引构建

Indexer 的构建函数如下：

```go
// 构建索引，参考 MetaNamespaceIndexFunc
func LabelsIndexFunc(obj interface{}) ([]string, error) {
	meta, err := meta.Accessor(obj)
	if err != nil {
		return []string{""}, fmt.Errorf("object has no meta: %v", err)
	}
	return []string{meta.GetLabels()["app"]}, nil
}

// 最终怎么展示 key 取决于这个函数
func myKeyFunc(obj interface{}) (string, error) {
	if key, ok := obj.(ExplicitKey); ok {
		return string(key), nil
	}
	meta, err := meta.Accessor(obj)
	if err != nil {
		return "", fmt.Errorf("object has no meta: %v", err)
	}
	if len(meta.GetNamespace()) > 0 {
		// 这里参考 MetaNamespaceKeyFunc 函数
		return meta.GetNamespace() + "---" + meta.GetName(), nil
	}
	return meta.GetName(), nil
}
```

创建 Indexer 的代码如下：

```go
indexers := Indexers{"app": LabelsIndexFunc}
myindex := NewIndexer(myKeyFunc, indexers)
```

执行以下代码，启动手工模拟的 SharedInformer。

```go
go run sharedinformer/indexer.go

# 输出结果
打印索引
map[app:map[l1:map[ns1---pod1:{}] l2:map[ns2---pod2:{}]]]
[ns1---pod1] <nil>
```

### 将 Indexer 集成到自己的 Informer 中

执行以下代码，启动手工模拟的 SharedInformer。

```go
go run sharedinformer/indexer_run.go
```

访问 http://localhost:8080 可以得到 label 为 app=robusta-forwarder 的 Pod。

```bash
curl http://localhost:8080

# 返回结果
["default/robusta-forwarder-5f65fd6fbb-s2trs"]
```

## SharedInformerFactory

执行以下代码，启动手工模拟的 SharedInformerFactory。

```go
go run sharedinformer/sharedinformerfactory.go
```

## 参考资料

- [在使用 SharedInformerFactory 时一些很小但值得注意的问题](https://xinzhao.me/posts/some-basic-but-noteworthy-points-when-using-sharedinformerfactory/)
- [client-go中的SharedInformerFactory机制](https://zhuanlan.zhihu.com/p/554834659)
- [Shared Informer 源码分析](https://www.notion.so/Shared-Informer-259412bd006144c59ca93f4944b842b4)
- [深入源码分析 kubernetes client-go sharedIndexInformer 和 SharedInformerFactory 的实现原理](https://xiaorui.cc/archives/7359)
- [深入了解 Kubernetes Informer](https://cloudnative.to/blog/client-go-informer-source-code/)
- [client-go SharedInformer](https://940504.top/posts/client-go-sharedinformer/)