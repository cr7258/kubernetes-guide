## 生成 Webhook 证书

Webhook 只能通过 HTTPS 进行调用，因此需要让 Kubernetes 信任 Webhook Server 的证书，需要将签发证书的 CA 添加到  WebhookConfiguration 的 caBundle 配置中。

```bash
make cert
```

构建镜像并推送至镜像仓库。

```bash
make docker-push
```

部署 Webhook Server。

```bash
make deploy
```

## 部署 Foo 应用

当 replicas 指定为负数时，ValidatingWebhookConfiguration 会拒绝。

```bash
kubectl apply -f example-validate.yaml
```

```go
func (f *Foo) ValidateCreate() (warnings admission.Warnings, err error) {
	if f.Spec.Replicas != nil && *f.Spec.Replicas < 0 {
		return nil, fmt.Errorf("replicas should be non-negative")
	}
	return nil, nil
}

func (f *Foo) ValidateUpdate(old runtime.Object) (warnings admission.Warnings, err error) {
	if f.Spec.Replicas != nil && *f.Spec.Replicas < 0 {
		return nil, fmt.Errorf("replicas should be non-negative")
	}
	return nil, nil
}

func (f *Foo) ValidateDelete() (warnings admission.Warnings, err error) {
	return nil, nil
}
```

当没有指定 replicas 时，MutatingWebhookConfiguration 会自动将其设置为 1。

```bash
kubectl apply -f example-mutate.yaml
```

```go
// 实现 Mutation Webhook 逻辑
func (f *Foo) Default() {
	if f.Spec.Replicas == nil {
		f.Spec.Replicas = new(int32)
		*f.Spec.Replicas = 1
	}
}
```

代码参见 pkg/custom/apis/foo/v1alpha1/webhook.go。

## 参考资料

- [setupadmissionwebhook](https://gist.github.com/tirumaraiselvan/b7eb1831d25dd9d59a785c11bd46c84b)
- [Writing a very basic kubernetes mutating admission webhook](Writing a very basic kubernetes mutating admission webhook)