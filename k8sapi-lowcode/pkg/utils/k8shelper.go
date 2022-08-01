package utils

import (
	"bytes"
	"fmt"
	"io"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/util"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	syaml "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/cli-runtime/pkg/resource"
	"log"
)

func setDefaultNamespaceIfScopedAndNoneSet(u *unstructured.Unstructured, helper *resource.Helper) {
	namespace := u.GetNamespace()
	if helper.NamespaceScoped && namespace == "" {
		namespace = "default"
		u.SetNamespace(namespace)
	}
}
func newRestClient(restConfig *rest.Config, gv schema.GroupVersion) (rest.Interface, error) {
	restConfig.ContentConfig = resource.UnstructuredPlusDefaultContentConfig()
	restConfig.GroupVersion = &gv
	if len(gv.Group) == 0 {
		restConfig.APIPath = "/api"
	} else {
		restConfig.APIPath = "/apis"
	}

	return rest.RESTClientFor(restConfig)
}

//模拟kubectl apply 功能
func K8sApply(json []byte, restConfig *rest.Config, mapper meta.RESTMapper) error {
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(json),
		len(json))
	for {
		var rawObj runtime.RawExtension
		err := decoder.Decode(&rawObj)
		if err != nil {
			if err == io.EOF {
				break
			} else {
				return err
			}
		}
		// 得到gvk
		obj, gvk, err := syaml.NewDecodingSerializer(unstructured.
			UnstructuredJSONScheme).Decode(rawObj.Raw, nil, nil)
		if err != nil {
			log.Fatal(err)
		}
		//把obj 变成map[string]interface{}
		unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			log.Fatal(err)
		}
		unstructuredObj := &unstructured.Unstructured{Object: unstructuredMap}

		restMapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			return err
		}
		//这里不能使用 传统的clientset 必须要使用这个函数
		restClient, err := newRestClient(restConfig, gvk.GroupVersion())

		helper := resource.NewHelper(restClient, restMapping)

		setDefaultNamespaceIfScopedAndNoneSet(unstructuredObj, helper)

		objInfo := &resource.Info{
			Client:          restClient,
			Mapping:         restMapping,
			Namespace:       unstructuredObj.GetNamespace(),
			Name:            unstructuredObj.GetName(),
			Object:          unstructuredObj,
			ResourceVersion: restMapping.Resource.Version,
		}

		// kubectl 封装 的一个 patcher
		patcher, err := NewPatcher(objInfo, helper)
		if err != nil {
			return err
		}

		//获取更改的 数据
		modified, err := util.GetModifiedConfiguration(objInfo.Object, true, unstructured.UnstructuredJSONScheme)
		if err != nil {
			return err
		}

		if err := objInfo.Get(); err != nil {
			if !errors.IsNotFound(err) { //资源不存在
				return err
			}

			//这里是kubectl的一些注解增加， 不管了。 直接加进去
			if err := util.CreateApplyAnnotation(objInfo.Object, unstructured.UnstructuredJSONScheme); err != nil {
				return err
			}

			// 直接创建
			obj, err := helper.Create(objInfo.Namespace, true, objInfo.Object)
			if err != nil {

				fmt.Println("有错")
				return err
			}
			objInfo.Refresh(obj, true)
		}

		_, patchedObject, err := patcher.Patch(objInfo.Object, modified, objInfo.Namespace, objInfo.Name)
		if err != nil {
			return err
		}

		objInfo.Refresh(patchedObject, true)

	}
	return nil
}
