package util

import (
	"errors"
	"os"
)

const (
	//为了简化。写死的 config文件
	KUBLET_CONFIG = "./.kube/kubelet.config"
)

// 是否要请求csr 证书
func NeedRequestCSR() bool {
	if _, err := os.Stat(KUBLET_CONFIG); errors.Is(err, os.ErrNotExist) {
		return true
	}
	return false
}
