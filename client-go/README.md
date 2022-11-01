# client-go 四种类型
## RestClient
RestClient 是最基础的客户端，RestClient 基于 HTTP request 进行了封装，实现了 Restful 的 API，可以直接通过 RESTClient 提供的 RESTful 方法如 Get()，Put()，Post()，Delete() 进行交互,同时支持 Json 和 protobuf,支持所有原生资源和 CRD。一般来说，为了更为优雅的处理，需要进一步封装，通过 Clientset 封装 RESTClient，然后再对外提供接口和服务。

## ClientSet
ClientSet 在 RestClient 的基础上封装了对 Resouorce 和 Version 的管理方法。一个 Resource 可以理解为一个客户端，而 ClientSet 是多个客户端的集合。
其操作资源对象时需要指定 Group、指定 Version，然后根据 Resource 获取，但是 ClientSet 不支持自定义 CRD。

## DynamicClient
DynamicClient 是一种动态客户端它可以对任何资源进行 Restful 操作，包括 CRD 自定义资源，不同于 ClientSet，DynamicClient 返回的对象是一个 map[string]interface{}，如果一个 controller 中需要控制所有的 API，可以使用 DynamicClient，目前它在 garbage collector 和 namespace controller 中被使用。
DynamicClient 的处理过程将 Resource，例如 podlist 转换为 unstructured 类型，k8s 的所有 Resource 都可以转换为这个结构类型，处理完之后再转换为 podlist，整个转换过程类似于接口转换就是通过 interface{} 的断言。
DynamicClient 是一种动态的 client，它能处理 kubernetes 所有的资源，只支持 JSON。

## DiscoveryClient
DiscoveryClient 是发现客户端，主要用于发现 Api Server 支持的资源组，资源版本和资源信息。kubectl 的 api-version 和 api-resource 也是通过 DiscoveryClient 来实现的，还可以将信息缓存在本地 cache，以减轻 API Server 的访问压力，默认在 ./kube/cache 和 ./kube/http-cache 下。
