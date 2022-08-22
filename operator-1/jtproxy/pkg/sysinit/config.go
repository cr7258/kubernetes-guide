package sysinit

import (
	"io/ioutil"
	v1 "k8s.io/api/networking/v1"
	"log"
	"sigs.k8s.io/yaml"
)

type Server struct {
	Port int // 代理启动端口
}

type SysConfigStruct struct {
	Server  Server
	Ingress v1.IngressSpec
}

var SysConfig = new(SysConfigStruct)

func InitConfig() {
	config, err := ioutil.ReadFile("./app.yaml")
	if err != nil {
		log.Fatal(err)
	}
	err = yaml.Unmarshal(config, SysConfig)
	if err != nil {
		log.Fatal(err)
	}
	ParseRule()
}
