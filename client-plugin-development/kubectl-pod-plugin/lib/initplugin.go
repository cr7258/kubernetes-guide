package lib

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/metrics/pkg/client/clientset/versioned"

	"log"
)
//本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
var cfgFlags *genericclioptions.ConfigFlags
//上节课代码 做了。封装
func InitClient()  {
	cfgFlags =genericclioptions.NewConfigFlags(true)
	config,err:= cfgFlags.ToRawKubeConfigLoader().ClientConfig()
	if err!=nil{log.Fatalln(err)}
	c,err:=kubernetes.NewForConfig(config)

	if err!=nil{log.Fatalln(err)}
	restConfig=config//设置了 config 。后面要用到

	client=c  //设置k8sclient

	//新增的代码
	metricClient=versioned.NewForConfigOrDie(config)

}

//如不懂，请私人提问
func MergeFlags(cmds ...*cobra.Command){
	for _,cmd:=range cmds{
		cfgFlags.AddFlags(cmd.Flags())
	}
}

var ShowLabels bool
var Labels string
var Fields string
var Search_PodName string
var Cache bool
//初始化 client放这里  了
var  client  *kubernetes.Clientset //这是 clientset
var metricClient *versioned.Clientset  //metric相关的client
var restConfig *rest.Config
//本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
func RunCmd( ) {
	cmd := &cobra.Command{
		Use:          "kubectl pods [flags]",
		Short:        "list pods ",
		Example:      "kubectl pods [flags]",
		SilenceUsage: true,
	}
	//执行初始化  ---- 改动
	InitClient()
	//合并主命令的参数
	MergeFlags(cmd, listCmd, promptCmd, testCmd)

	//本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
	addListCmdFlags()

	//加入子命令
	cmd.AddCommand(listCmd, promptCmd, testCmd)
	err:=cmd.Execute()
	if err!=nil{
		log.Fatalln(err)
	}
}
//本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334