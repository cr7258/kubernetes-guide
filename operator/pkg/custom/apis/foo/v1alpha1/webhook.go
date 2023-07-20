package v1alpha1

import (
	"fmt"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	_ webhook.Defaulter = &Foo{}
	_ webhook.Validator = &Foo{}
)

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

// 实现 Mutation Webhook 逻辑
func (f *Foo) Default() {
	if f.Spec.Replicas == nil {
		f.Spec.Replicas = new(int32)
		*f.Spec.Replicas = 1
	}
}
