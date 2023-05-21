package lib

import "time"

const (
	CSR_DURATION              = time.Second * 3600 * 24 * 365 //CSR的过期时间
	PRIVATEKEY_FILE_NAME      = "kubelet.key"
	PEM_FILE_NAME             = "kubelet.pem"
	BOOTSTRAP_PRIVATEKEY_FILE = "./.kube/" + PRIVATEKEY_FILE_NAME
	BOOTSTRAP_PEM_FILE        = "./.kube/" + PEM_FILE_NAME
	BOOTSTRAP_PRIVATEKEY_TYPE = "RSA PRIVATE KEY"
	CSR_WAITING_TIMEOUT       = time.Second * 60 //默认60秒 超时
)
