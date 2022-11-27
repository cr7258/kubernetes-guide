package cmds

import (
	"depplugin/pkg/cache"
	"github.com/spf13/cobra"
	"log"
)
//如不懂，请私人提问
func MergeFlags(cmds ...*cobra.Command){
	for _,cmd:=range cmds{
		cache.CfgFlags.AddFlags(cmd.Flags())
	}
}
//本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
func RunCmd( ) {
	cmd := &cobra.Command{
		Use:          "kubectl ingress prompt",
		Short:        "list ingress ",
		Example:      "kubectl ingress prompt",
		SilenceUsage: true,
	}
	cache.InitClient() //初始化k8s client
	cache.InitCache() //初始化本地 缓存---informer
	//合并主命令的参数
	MergeFlags(cmd, promptCmd,viewCmd)


	//加入子命令
	cmd.AddCommand( promptCmd,viewCmd)
	err:=cmd.Execute()
	if err!=nil{
		log.Fatalln(err)
	}
}