package lib

import (
	"fmt"
	"github.com/c-bata/go-prompt"
	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"log"
	"os"
	"sort"
	"strings"
)
func executorCmd(cmd *cobra.Command) func(in string) {
	return func(in string) {
		// in :  get abc
		in = strings.TrimSpace(in)
		blocks := strings.Split(in, " ")
		args:=[]string{}
		if len(blocks)>1{
			args=blocks[1:]
		}
		switch blocks[0] {
		case "exit":
			fmt.Println("Bye!")
			ResetSTTY() //这步要执行 ，否则无法打印命令
			os.Exit(0)
		case "list":
			err:=cacheCmd.RunE(cmd,args)
			if err!=nil{
				log.Fatalln(err)
			}
		case "get":
			clearConsole()
			runteaInfo(args,cmd)
		case "del":
			delPod(args,cmd) //删除POD
		case "ns":
			showNameSpace(cmd) //打印当前命名空间
		case "use":
			setNameSpace(args,cmd) //切换命名空间
		case "exec":  // 远程登录POD
			runteaExec(args,cmd)
		case "top": //指标
			getPodMetric(getNameSpace(cmd))
		case "clear":
			//以下代码官方抄的
			clearConsole()
		}
	}

}
func clearConsole()  {
	MyConsoleWriter.EraseScreen()
	MyConsoleWriter.CursorGoTo(0,0)
	MyConsoleWriter.Flush()
}
var suggestions = []prompt.Suggest{
	// Command
	{"top", "显示当前POD列表的指标数据"},
	{"exec", "pod的shell操作"},
	{"get", "获取POD详细"},
	{"use", "设置当前namespace,请填写名称"},
	{"ns", "显示当前命名空间"},
	{"del", "删除某个POD"},
	{"list", "显示Pods列表"},
	{"clear", "清除屏幕"},
	{"exit", "退出交互式窗口"},
}
type CoreV1POD []*corev1.Pod
func(this CoreV1POD) Len() int{
	return len(this)
}
func(this CoreV1POD) Less(i, j int) bool{
	//根据时间排序    倒排序
	return this[i].CreationTimestamp.Time.After(this[j].CreationTimestamp.Time)
}
func(this CoreV1POD) Swap(i, j int){
	this[i],this[j]=this[j],this[i]
}
func getPodsList(cmd *cobra.Command) (ret []prompt.Suggest) {
	pods,err:=  fact.Core().V1().Pods().Lister().
		Pods(getNameSpace(cmd)).List(labels.Everything())
	sort.Sort(CoreV1POD(pods)) //排序， 不然是乱的
	if err!=nil{return }
	for _,pod:=range pods{
		ret=append(ret,prompt.Suggest{
			Text: pod.Name,
			Description:"节点:"+pod.Spec.NodeName+" 状态:"+
				string(pod.Status.Phase)+" IP:"+pod.Status.PodIP,
		})
	}
	return
}
func completerCmd(c *cobra.Command) func( prompt.Document) []prompt.Suggest{
	return func( in prompt.Document) []prompt.Suggest {
		w := in.GetWordBeforeCursor()
		cmd,opt:=parseCmd(in.TextBeforeCursor())
		if inArray([]string{"get","del","exec"},cmd){
			return prompt.FilterHasPrefix(getPodsList(c),opt, true)
		}
		if w == "" {
			return []prompt.Suggest{}
		}
		return prompt.FilterHasPrefix(suggestions, w, true)
	}
}

var MyConsoleWriter=  prompt.NewStdoutWriter()  //定义一个自己的writer
var promptCmd= &cobra.Command{
	Use:          "prompt",
	Short:        "prompt pods ",
	Example:      "kubectl pods prompt",
	SilenceUsage: true,
	RunE: func(c *cobra.Command, args []string) error {
		InitCache() //初始化缓存
		p := prompt.New(
			executorCmd(c),
			completerCmd(c),
			prompt.OptionTitle("程序员在囧途"),
			prompt.OptionPrefix(">>> "),
			prompt.OptionWriter(MyConsoleWriter), //设置自己的writer
		)

		p.Run()

		return nil
	},

}