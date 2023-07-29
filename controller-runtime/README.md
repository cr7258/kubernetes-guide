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