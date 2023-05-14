package main

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"log"
)

const (
	AccessApiVersion = "authorization.k8s.io/v1beta1"
	AccessKind       = "SubjectAccessReview"
)

func rsp(allowed bool, reason string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(AccessApiVersion)
	obj.SetKind(AccessKind)
	obj.Object["status"] = map[string]interface{}{
		"allowed": allowed,
		reason:    reason,
	}

	return obj
}

//TODO: 本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
//TODO 专注golang、k8s云原生、Rust技术栈

func main() {
	r := gin.New()

	r.POST("/authorize", func(c *gin.Context) {
		b, err := io.ReadAll(c.Request.Body)
		if err != nil {
			log.Println(err)
			c.AbortWithStatusJSON(400, rsp(false, err.Error()))
		}
		obj := &unstructured.Unstructured{}
		err = json.Unmarshal(b, obj)
		if err != nil {
			log.Println(err)
			c.AbortWithStatusJSON(400, rsp(false, err.Error()))
		}
		fmt.Println(string(b))
		c.JSON(200, rsp(true, "")) // 放行所有请求
	})
	r.RunTLS(":9090", "./certs/server.pem",
		"./certs/server-key.pem")

}

//TODO: 本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
//TODO 专注golang、k8s云原生、Rust技术栈
