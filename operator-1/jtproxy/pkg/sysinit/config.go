package sysinit

import (
	"github.com/gorilla/mux"
	"io/ioutil"
	"k8s.io/api/networking/v1"
	"os"
	"sigs.k8s.io/yaml"
)

type Server struct {
	Port int //代表是代理启动端口
}
type SysConfigStruct struct {
	Server  Server
	Ingress []v1.Ingress
}

var SysConfig = new(SysConfigStruct)

func InitConfig() error {
	config, err := ioutil.ReadFile("./app.yaml")
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(config, SysConfig)
	if err != nil {
		return err
	}
	ParseRule()
	return nil

}
func saveConfigToFile() error {
	b, _ := yaml.Marshal(SysConfig)
	appYamlFile, err := os.OpenFile("./app.yaml", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer appYamlFile.Close()
	_, err = appYamlFile.Write(b)
	if err != nil {
		return err
	}
	return nil
}

//删除配置
func DeleteConfig(name, namespace string) error {
	isEdit := false
	for index, config := range SysConfig.Ingress {
		if config.Name == name && config.Namespace == namespace {
			SysConfig.Ingress = append(SysConfig.Ingress[:index], SysConfig.Ingress[index+1:]...)
			isEdit = true
			break
		}
	}
	if isEdit {
		if err := saveConfigToFile(); err != nil {
			return err
		}
		return ReloadConfig()
	}
	return nil
}

//更新 ingress对象 配置 ,更新内存配置 和 配置持久化
func ApplyConfig(ingress *v1.Ingress) error {
	isEdit := false
	for index, config := range SysConfig.Ingress {
		if config.Name == ingress.Name && config.Namespace == ingress.Namespace {
			SysConfig.Ingress[index] = *ingress
			isEdit = true
			break
		}
	}
	if !isEdit {
		SysConfig.Ingress = append(SysConfig.Ingress, *ingress)
	}

	//增加了一个 保存配置的 函数
	if err := saveConfigToFile(); err != nil {
		return err
	}
	return ReloadConfig()
}

//重载配置
func ReloadConfig() error {
	MyRouter = mux.NewRouter()
	return InitConfig()
}
