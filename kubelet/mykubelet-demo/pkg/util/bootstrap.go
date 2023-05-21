package util

import (
	"errors"
	"os"
)

// TODO 本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
const (
	//为了简化。写死的 config文件
	KUBLET_CONFIG = "./.kube/kubelet.config"
)

// TODO 本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
// 是否要请求csr 证书
func NeedRequestCSR() bool {
	if _, err := os.Stat(KUBLET_CONFIG); errors.Is(err, os.ErrNotExist) {
		return true
	}
	return false
}
