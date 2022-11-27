package cmds

import (
	"depplugin/pkg/cache"
	"depplugin/pkg/utils"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/spf13/cobra"
	"github.com/tidwall/gjson"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	"k8s.io/apimachinery/pkg/util/json"
	"log"
	"os"
	"sigs.k8s.io/yaml"
)
const REVISION="deployment.kubernetes.io/revision"
// 根据指定的RS 无脑 加载POD列表
func getPodsBySpecRs(rs *v1.ReplicaSet) []*corev1.Pod{
	pods:=make([]*corev1.Pod,0)
	podList,err:=cache.Factory.Core().V1().Pods().Lister().
		Pods(rs.Namespace).List(labels.Everything())
	if err!=nil{
		fmt.Println(pods)
		return pods
	}
	for _,pod:=range podList{
		for _,ref:=range pod.OwnerReferences{
			if ref.UID==rs.UID{
				pods=append(pods,pod)
			}
		}
	}
	return pods
}

//根据 rs和deployment 加载POD
func getPodsByRs(dep *v1.Deployment,rs *v1.ReplicaSet) []*corev1.Pod{
	pods:=make([]*corev1.Pod,0)
	if rs.Annotations[REVISION]==dep.Annotations[REVISION]{
		for _,ref:=range rs.OwnerReferences{
			if ref.UID==dep.UID     {
				pods=append(pods,getPodsBySpecRs(rs)...)
				break
			}
		}
	}

	return pods
}
func getPodsByDeploy(dep *v1.Deployment) []*corev1.Pod  {
	 //第一步：取 rs
	pods:=make([]*corev1.Pod,0)
	rsList,err:=cache.Factory.Apps().V1().ReplicaSets().Lister().ReplicaSets(dep.Namespace).List(labels.Everything())
	if err!=nil{
		fmt.Println(err)
		return pods
	}
	for _,rs:=range rsList{
		//判断rs 是否属于 该deployment
		pods=append(pods,getPodsByRs(dep,rs)...)
	}
	return pods
}



//获取  deployment 根据JSON
func getDeployDetailByJSON(name,path string,cmd *cobra.Command){
	ns:=utils.GetNameSpace(cmd)
	dep,err:= cache.Factory.Apps().V1().Deployments().Lister().
		Deployments(ns).Get(name)
	if err!=nil {
		log.Println(err)
		return
	}
	//事件获取
	if path==DeployEventType{ //代表 是取 dep事件
		eventsList,err:=  cache.Factory.Core().V1().Events().
			Lister().List(labels.Everything())
		if err!=nil{
			log.Println(err)
			return
		}
		podEvents:=[]*corev1.Event{}
		for _,e:=range eventsList{
			if e.InvolvedObject.UID==dep.UID{
				podEvents=append(podEvents,e)
			}
		}
		utils.PrintEvent(podEvents)
		return
	}

	//取POD列表
	if path==DeployPodsType { //代表 是取 POD列表
		pods:=getPodsByDeploy(dep)  //根据deployment 获取POD列表
		utils.PrintPods(pods)
		return
	}
	jsonStr,_:=json.Marshal(dep)
	ret:=gjson.Get(string(jsonStr),path)
	if !ret.Exists(){
		log.Println("无法找到对应的内容:"+path)
		return
	}
	if !ret.IsObject() && !ret.IsArray(){ //不是对象不是 数组，直接打印
		fmt.Println(ret.Raw)
		return
	}
	var tempMap interface{}
	if ret.IsObject(){
		tempMap=make(map[string]interface{})
	}
	if ret.IsArray(){
		tempMap=[]interface{}{}
	}
	err=yaml.Unmarshal([]byte(ret.Raw),&tempMap)
	if err!=nil{
		log.Println(err)
		return
	}
	b,_:=yaml.Marshal(tempMap)
	fmt.Println(string(b))

}
type deployjson struct {
	title string
	path string

}
type deploymodel struct {
	items    []*deployjson
	index   int
	cmd *cobra.Command
	podName string
}
func (m deploymodel) Init() tea.Cmd {
	return nil
}
func (m deploymodel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up":
			if m.index > 0 {
				m.index--
			}else{
				m.index=len(m.items)-1
			}
		case "down":
			if m.index < len(m.items)-1 {
				m.index++
			}else{
				m.index=0
			}
		case "enter":
			getDeployDetailByJSON(m.podName,m.items[m.index].path,m.cmd)
			return m,tea.Quit
		}
	}
	return m, nil
}

func (m deploymodel) View() string {
	s := "本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334\n按上下键选择要查看POD的内容\n\n"
	for i, item := range m.items {
		selected := " "
		if m.index == i {
			selected = "»"
		}
		s += fmt.Sprintf("%s %s\n", selected, item.title)
	}

	s += "\n按Q退出\n"
	return s
}

const (
	DeployEventType = "__event__"
	DeployPodsType= "__pod__"
)

//本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
func runDeployInfo(args []string,cmd *cobra.Command)  {
	if len(args)==0{
		log.Println("deployname is required")
		return
	}
	var depModel=deploymodel{
		items:    []*deployjson{},
		cmd: cmd,
		podName: args[0],
	}
   //v1.Deployment{}
	depModel.items=append(depModel.items,
		&deployjson{title:"标签", path: "metadata.labels"},
		&deployjson{title:"注解", path: "metadata.annotations"},
		&deployjson{title:"标签选择器", path: "spec.selector"},
		&deployjson{title:"POD模板", path: "spec.template"},
		&deployjson{title:"状态", path: "status"},
		&deployjson{title:"全部", path: "@this"},
		&deployjson{title:"*事件*", path: DeployEventType},
		&deployjson{title:"*查看POD*", path: DeployPodsType},

	)
	//本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
	teaCmd := tea.NewProgram(depModel)
	if err := teaCmd.Start(); err != nil {
		fmt.Println("start failed:", err)
		os.Exit(1)
	}
}

// tview下的中间信息展示。 后面 可能还需改
func DetailText(app *tview.Application) *tview.TextView  {
	textView := tview.NewTextView().SetWordWrap(true)
	textView.SetBorder(true)
	textView.SetBorderPadding(1,1,1,1)
	textView.SetBlurFunc(func() {
		textView.SetBorderColor(tcell.Color103)
	})
	textView.SetFocusFunc(func() {
		textView.SetBorderColor(tcell.Color29)
	})
	//关键部分： 设置焦点
	textView.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyESC{
			app.SetFocus(global.List)
		}
		if key == tcell.KeyEnter{
			app.SetFocus(global.POD)
		}

	})
	return textView

}