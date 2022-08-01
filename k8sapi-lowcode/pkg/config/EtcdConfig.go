package config

import (
	"k8sapi-lowcode/pkg/store"
)

type EtcdConfig struct {
}

func NewEtcdConfig() *EtcdConfig {
	return &EtcdConfig{}
}

// 注入 etcd 操作类
func (*EtcdConfig) InitStore() *store.EtcdStore {
	return store.NewEtcdStore()
}
