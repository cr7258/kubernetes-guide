package cmds

import (
	"context"
	"depplugin/pkg/cache"
	"depplugin/pkg/utils"
	"depplugin/pkg/webui/steps"
	"fmt"
	"github.com/c-bata/go-prompt"
	"github.com/gdamore/tcell/v2"
	"github.com/olekukonko/tablewriter"
	"github.com/rivo/tview"
	"github.com/spf13/cobra"
	appv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"log"
	"os"
	"sigs.k8s.io/yaml"
	"sort"
	"strconv"
	"unicode"
)
// 根据POD 反推 所属的Deployment -------------------------
func getDeployByUID(uid types.UID,ns string ) *appv1.Deployment{
	depCacheList,_:=cache.Factory.Apps().V1().Deployments().Lister().
		Deployments(ns).List(labels.Everything())
	for _,dep:=range depCacheList{
		if dep.UID==uid{
			return dep
		}
	}
	return nil
}
func getRsByUID(uid types.UID,ns string ) *appv1.ReplicaSet{
	rsCachelist,_:=cache.Factory.Apps().V1().ReplicaSets().Lister().
		ReplicaSets(ns).List(labels.Everything())
	for _,rs:=range rsCachelist{
		if rs.UID==uid{
			return rs
		}
	}
	return nil
}
func getDeployByPod(pod *v1.Pod) []*appv1.Deployment{
	ret:=[]*appv1.Deployment{}
	for _,podRef:=range pod.OwnerReferences{
		if getRs:=getRsByUID(podRef.UID,pod.Namespace);getRs!=nil{
			for _,rsRef:=range getRs.OwnerReferences{
				if getDeploy:=getDeployByUID(rsRef.UID,pod.Namespace);getDeploy!=nil{
					ret=append(ret,getDeploy)
				}
			}
		}
	}
	return ret
}
//根据POD 反推 所属的Deployment -------------------------

type V1Deployment []*appv1.Deployment
func(this V1Deployment) Len() int{
	return len(this)
}
func(this V1Deployment) Less(i, j int) bool{
	//根据时间排序    倒排序
	//return this[i].CreationTimestamp.Time.After(this[j].CreationTimestamp.Time)
	//改成了 按名称(首字母排序)
	return []rune(this[i].Name)[0]<[]rune(this[j].Name)[0]
}
func(this V1Deployment) Swap(i, j int){
	this[i],this[j]=this[j],this[i]
}

//取出deploy列表
func listDeploys(ns string ) []*appv1.Deployment {
	list,err:= cache.Factory.Apps().V1().Deployments().Lister().Deployments(ns).
		List(labels.Everything())
	if err!=nil{
		log.Println(err)
		return nil
	}
	sort.Sort(V1Deployment(list)) // 排序
	return list
}
//用于提示 用  ----后面在做
func RecommendDeployment(ns string) (ret []prompt.Suggest)  {
	depList:= listDeploys(ns)
	if 	depList==nil{
		return
	}

	for _,dep:=range depList{
		ret=append(ret,prompt.Suggest{
			Text: dep.Name,
			Description:fmt.Sprintf("副本:%d/%d",dep.Status.ReadyReplicas,
				dep.Status.Replicas),
		})
	}
	return
}

//渲染 deploys 列表
func RenderDeploy(args []string,cmd *cobra.Command )  {
	ns:=utils.GetNameSpace(cmd)
	deplist:= listDeploys(ns)
	if deplist==nil{
		return
	}
	table := tablewriter.NewWriter(os.Stdout)
	//设置头
	table.SetHeader([]string{"名称","副本数","创建时间","最新事件"})
	for _,dep:=range deplist {
		depRow:=[]string{dep.Name,
				fmt.Sprintf("%d/%d",dep.Status.ReadyReplicas,dep.Status.Replicas),
				dep.CreationTimestamp.Format("2006/01/02 15:04"),
				getLatestDeployEvent(dep.UID,ns),
			}

		table.Append(depRow)
	}
	utils.SetTable(table)
	table.Render()
	steps.StepsData.SetStep(steps.StepDeployList)
}

// 装饰器函数。 用于渲染deployment列表
func ListerCallbak(ns,depName string,app *tview.Application) func(){
	return func() {
		global.Detail.SetText("")
		dep,err:= cache.Factory.Apps().V1().Deployments().Lister().
			Deployments(ns).Get(depName)
		if err!=nil {
			global.Detail.SetText(err.Error()) // 详细框
			return
		}
		b,_:=yaml.Marshal(dep)
		global.Detail.SetText(string(b))

		global.POD.Clear()
		getPods:=getPodsByDeploy(dep)  //渲染POD列表框
		for _,pod:=range getPods{
			podName:=pod.Name
			global.POD.AddItem(podName,fmt.Sprintf("%s/%s",pod.Spec.NodeName,pod.Status.Phase),[]rune(podName)[0],nil)
		}

		app.SetFocus(global.Detail) //关键部分 设置焦点

	}
}
//渲染tview deploys列表
func RenderTViewDeploy(cmd *cobra.Command,app *tview.Application) *tview.List{
	ns:=utils.GetNameSpace(cmd)

	list:=tview.NewList()
	list.SetBorder(true)
	list.SetBorderPadding(1,1,1,1)

	list.SetBlurFunc(func() {
		list.SetBorderColor(tcell.Color103)
	})
	list.SetFocusFunc(func() {
		list.SetBorderColor(tcell.Color29)
	})
	list.SetTitle("deploys列表(0)")
	//第二种做法。 重新染整个列表. 如果数据不多，情愿使用这一种
	go func() {
		for _=range utils.DeployChan{
			deplist:= listDeploys(ns)  //每次都要从缓存取一次
			// 这一部分视频里是故意不做讲解的。 VIP学员如果不懂  请一对1 提问
			//这一步注意：要先获取当前 选中的dep 名称,一旦重新渲染后要定位
			oldItem:=""
			if list.GetItemCount()>0{
				oldItem,_=list.GetItemText(list.GetCurrentItem())
			}
			newIndex:=-1 // 重新渲染后，选中的索引为止。 默认是-1 。下面遍历时匹配
			list.Clear()  //这一步别忘了,必须要清空列表
			for index,dep:=range deplist{
				depName:=dep.Name
				if oldItem==depName{
					newIndex=index
				}
				list.AddItem(depName,"",([]rune(depName))[0],ListerCallbak(ns,depName,app))
			}
			list.SetTitle(fmt.Sprintf("deploys列表(%d)",len(deplist)))

			//这里要判断newIndex的值， 如果是 -1 。 说明之前选中的deploy被删掉了 则要清掉detail 和podlist的内容
			if newIndex==-1{
				global.Detail.Clear()
				global.POD.Clear()
				newIndex=0 //默认让其选中第一个
				app.SetFocus(global.List) //焦点设置到 List框
			}
			list.SetCurrentItem(newIndex)  //重新定位 选中项

			app.ForceDraw() //强制重画 ,否则可能会 在失去焦点没有数据显示
		}
	}()

	//  处理POD 。 思路如下：
	//  当POD发生修改时 反推他所属的Deployment，如果找到，则匹配当前选中的deploy
	// 如果是，则重新渲染POD列表
	go func() {
		for pod:=range utils.PodChan{
			if pod.Namespace==ns && list.GetItemCount()>0{
				// 获取当前选中 的deploy名称
				getSelected,_:=list.GetItemText(list.GetCurrentItem())
				if depList:=getDeployByPod(pod);depList!=nil{
					for _,dep:=range depList{
						if dep.Name==getSelected{
							ListerCallbak(ns,getSelected,app)()
							// abc !=abc()
						}
					}
					app.ForceDraw()
				}
			}
		}

	}()

	return list

}

func RenderTViewPod(app *tview.Application) *tview.List{
	list:=tview.NewList()
	list.SetBorder(true)
	list.SetBorderPadding(1,1,1,1)


	list.SetBlurFunc(func() {
		list.SetBorderColor(tcell.Color103)
	})
	list.SetFocusFunc(func() {
		list.SetBorderColor(tcell.Color29)
	})
	//ECS 取消建
	list.SetDoneFunc(func() {
		app.SetFocus(global.Detail) //切换到详细框
	})
	return list
}


func ScaleDeployForView(cmd *cobra.Command,app *tview.Application){
	ns:=utils.GetNameSpace(cmd)
	depName,_:=global.List.GetItemText(global.List.GetCurrentItem())
	ctx:=context.Background()
	scale,_:=cache.Client.AppsV1().Deployments(ns).GetScale(ctx,depName,
		metav1.GetOptions{})

	form := tview.NewForm().
		AddInputField("副本数", strconv.Itoa(int(scale.Spec.Replicas)), 20,
		func(textToCheck string, lastChar rune) bool {
			return unicode.IsNumber(lastChar)
		}, func(text string) {
			 if rc,err:=strconv.Atoi(text);err==nil{
				 scale.Spec.Replicas=int32(rc)
			 }
		}).
		AddButton("确定", func() {
		cache.Client.AppsV1().Deployments(ns).UpdateScale(ctx,depName,scale,metav1.UpdateOptions{})
		app.SetRoot(global.Flex,true)
		app.SetFocus(global.List)
	}).
		AddButton("取消", func() {
				app.SetRoot(global.Flex,true)
				app.SetFocus(global.List)
		})
	form.SetBorder(true).
		SetTitle("伸缩副本").
		SetTitleAlign(tview.AlignCenter)
	app.SetRoot(form,true)
}
// 删掉POD
func DeletePOD(cmd *cobra.Command,app *tview.Application)  {
	modal := tview.NewModal().
		SetText("是否要删除该POD?").
		AddButtons([]string{"确定", "取消"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "确定" {
				if global.POD.GetItemCount()>0{
					ns:=utils.GetNameSpace(cmd)
					podName,_:=global.POD.GetItemText(global.POD.GetCurrentItem())
					cache.Client.CoreV1().Pods(ns).Delete(context.Background(),podName,metav1.DeleteOptions{})
				}
			}
			app.SetRoot(global.Flex,true)
			app.SetFocus(global.POD)

		})
	   app.SetRoot(modal,false)

}

type V1Events []*v1.Event
func(this V1Events) Len() int{
	return len(this)
}
func(this V1Events) Less(i, j int) bool{
	//根据时间排序    倒排序
	return this[i].CreationTimestamp.Time.After(this[j].CreationTimestamp.Time)
}
func(this V1Events) Swap(i, j int){
	this[i],this[j]=this[j],this[i]
}

//获取deployment的最新事件
func getLatestDeployEvent(uid types.UID ,ns string) string   {
	  list,err:=cache.Factory.Core().V1().Events().Lister().Events(ns).
	  	List(labels.Everything())
	  if err!=nil{
	  	return ""
	  }
	  sort.Sort(V1Events(list)) //排序
	  for _,e:=range list{
	  	if e.InvolvedObject.UID==uid {
			return e.Message
		}
	  }
	  return ""
}