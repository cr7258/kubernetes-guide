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