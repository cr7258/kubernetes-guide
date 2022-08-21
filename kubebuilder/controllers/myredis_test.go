package controllers

import (
	"context"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	myappv1 "jtapp/api/v1"
)

var _ = Describe("test myredis", func() {
	redis := &myappv1.Redis{}
	redis.Name = "myredis" // 去掉一个触发测试报错
	redis.Namespace = "default"
	redis.Spec.Port = 2377
	redis.Spec.Num = 3

	It("create myredis", func() {
		Expect(k8sClient.Create(context.Background(), redis)).Should(Succeed())

	})

})
