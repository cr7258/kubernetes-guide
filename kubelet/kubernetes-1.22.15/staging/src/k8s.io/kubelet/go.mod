// This is a generated file. Do not edit directly.

module k8s.io/kubelet

go 1.16

require (
	github.com/gogo/protobuf v1.3.2
	golang.org/x/net v0.0.0-20211209124913-491a49abca63
	google.golang.org/grpc v1.38.0
	k8s.io/api v0.0.0
	k8s.io/apimachinery v0.0.0
	k8s.io/component-base v0.0.0
)

replace (
	k8s.io/api => ../api
	k8s.io/apimachinery => ../apimachinery
	k8s.io/client-go => ../client-go
	k8s.io/component-base => ../component-base
	k8s.io/kubelet => ../kubelet
)
