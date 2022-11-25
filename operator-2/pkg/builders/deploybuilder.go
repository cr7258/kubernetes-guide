package builders

import (
	"bytes"
	"context"
	configv1 "github.com/shenyisyn/dbcore/pkg/apis/dbconfig/v1"
	appv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"text/template"
)

// 本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
type DeployBuilder struct {
	deploy *appv1.Deployment
	config *configv1.DbConfig //新增属性 。保存 config 对象
	client.Client
}

// 目前软件的 命名规则
func deployName(name string) string {
	return "dbcore-" + name
}

// 构建 deploy 创建器
func NewDeployBuilder(config *configv1.DbConfig, client client.Client) (*DeployBuilder, error) {
	deploy := &appv1.Deployment{}
	err := client.Get(context.Background(), types.NamespacedName{
		Namespace: config.Namespace, Name: deployName(config.Name), //这里做了改动
	}, deploy)
	if err != nil { //没取到
		deploy.Name, deploy.Namespace = config.Name, config.Namespace
		tpl, err := template.New("deploy").Parse(deptpl)
		var tplRet bytes.Buffer
		if err != nil {
			return nil, err
		}
		err = tpl.Execute(&tplRet, deploy)
		if err != nil {
			return nil, err
		}
		// 解析  deploy模板----仅仅是模板

		err = yaml.Unmarshal(tplRet.Bytes(), deploy)
		if err != nil {
			return nil, err
		}

	}
	return &DeployBuilder{deploy: deploy, Client: client, config: config}, nil
}

func (this *DeployBuilder) apply() *DeployBuilder {
	// 同步副本
	*this.deploy.Spec.Replicas = int32(this.config.Spec.Replicas)
	return this
}

func (this *DeployBuilder) setOwner() *DeployBuilder {
	this.deploy.OwnerReferences = append(this.deploy.OwnerReferences,
		v1.OwnerReference{
			APIVersion: this.config.APIVersion,
			Kind:       this.config.Kind,
			Name:       this.config.Name,
			UID:        this.config.UID,
		})
	return this
}

// 构建出  deployment  ..有可能是新建， 有可能是update
func (this *DeployBuilder) Build(ctx context.Context) error {

	if this.deploy.CreationTimestamp.IsZero() {
		this.apply().setOwner() //同步  所需要的属性 如 副本数 , 并且设置OwnerReferences
		err := this.Create(ctx, this.deploy)
		if err != nil {
			return err
		}
	} else {
		patch := client.MergeFrom(this.deploy.DeepCopy())
		this.apply() //同步  所需要的属性 如 副本数
		err := this.Patch(ctx, this.deploy, patch)
		if err != nil {
			return err
		}
	}
	return nil
}
