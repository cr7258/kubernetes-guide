package cmds

import (
	"depplugin/pkg/utils"
	"fmt"
	"github.com/c-bata/go-prompt"
	"github.com/spf13/cobra"
	"os"
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
			utils.ResetSTTY() //这步要执行 ，否则无法打印命令
			os.Exit(0)
		case "use":
			utils.SetNameSpace(args,cmd) //切换命名空间
		case "ns":
			fmt.Println("您当前所处的namespace是：",utils.GetNameSpace(cmd))
		case "list":
			RenderDeploy(args,cmd) //渲染Ingress列表
		case "scale": //伸缩副本
		     ScaleDeploy(args,cmd)
		case "get":
			clearConsole()
			runDeployInfo(args,cmd)
		case "clear":
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
	{"list", "显示Deployment列表"},
	{"clear", "清除屏幕"},
	{"use", "设置当前namespace,请填写名称"},
	{"ns", "显示当前命名空间"},
	{"scale", "伸缩副本"},
	{"exit", "退出交互式窗口"},
}

func completerCmd(c *cobra.Command) func( prompt.Document) []prompt.Suggest{
	return func( in prompt.Document) []prompt.Suggest {
		w := in.GetWordBeforeCursor()
		cmd,opt:=utils.ParseCmd(in.TextBeforeCursor())
		if utils.InArray([]string{"get","scale"},cmd){
			return prompt.FilterHasPrefix(RecommendDeployment(utils.GetNameSpace(c)),opt, true)
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
	Example:      "kubectl ingress prompt",
	SilenceUsage: true,
	RunE: func(c *cobra.Command, args []string) error {
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