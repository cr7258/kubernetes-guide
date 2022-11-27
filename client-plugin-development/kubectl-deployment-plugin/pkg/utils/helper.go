package utils

import (
	"fmt"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
)
// item is in []string{}
func InArray(arr []string,item string ) bool  {
	for _,p:=range arr{
		if p==item{
			return true
		}
	}
	return false
}
//设置table的样式，不重要 。看看就好
func SetTable(table *tablewriter.Table){
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetBorder(false)
	table.SetTablePadding("\t") // pad with tabs
	table.SetNoWhiteSpace(true)
}
//两个返回值， 一个是 命令 第二个是options
func ParseCmd(w string) (string,string){
	w=regexp.MustCompile("\\s+").ReplaceAllString(w," ")
	l:=strings.Split(w," ")
	if len(l)>=2{
		return l[0],strings.Join(l[1:]," ")
	}
	return w,""
}

func SetNameSpace(args []string,cmd *cobra.Command){
	if len(args)==0{
		log.Println("namespace name is required")
		return
	}
	err:=cmd.Flags().Set("namespace",args[0])
	if err!=nil{
		log.Println("设置namespace失败:",err.Error())
		return
	}
	fmt.Println("设置namespace成功")
}
const DefaultNameSpace="default"
func GetNameSpace(cmd *cobra.Command) string{
	ns,err:=cmd.Flags().GetString("namespace")
	if err!=nil{
		log.Println("error ns param")
		return  DefaultNameSpace}
	if ns==""{ns=DefaultNameSpace}
	return ns
}
//不懂 。个人提问
func ResetSTTY()  {
	cc:=exec.Command("stty", "-F", "/dev/tty", "echo")
	cc.Stdout=os.Stdout
	cc.Stderr=os.Stderr
	if err:=cc.Run();err!=nil{
		log.Println(err)
	}
}
//事件要显示的头
var eventHeaders=[]string{"事件类型", "REASON", "所属对象","消息"}
func PrintEvent(events []*v1.Event){
	table := tablewriter.NewWriter(os.Stdout)
	//设置头
	table.SetHeader(eventHeaders)
	for _,e:=range events {
		podRow:=[]string{e.Type,e.Reason,
			fmt.Sprintf("%s/%s",e.InvolvedObject.Kind,e.InvolvedObject.Name),e.Message}

		table.Append(podRow)
	}
	SetTable(table)
	table.Render()
}

var podHeaders=[]string{"POD名称","IP","状态","节点"}
func PrintPods(pods []*v1.Pod){
	table := tablewriter.NewWriter(os.Stdout)
	//设置头
	table.SetHeader(podHeaders)
	for _,pod:=range pods {
		podRow:=[]string{pod.Name,pod.Status.PodIP,
			string(pod.Status.Phase),pod.Spec.NodeName}
		table.Append(podRow)
	}
	SetTable(table)
	table.Render()
}