package cmds

import (
	"depplugin/pkg/cache"
	"depplugin/pkg/utils"
	"fmt"
	"github.com/c-bata/go-prompt"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	appv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/labels"
	"log"
	"os"
	"sort"
)
type V1Deployment []*appv1.Deployment
func(this V1Deployment) Len() int{
	return len(this)
}
func(this V1Deployment) Less(i, j int) bool{
	//根据时间排序    倒排序
	return this[i].CreationTimestamp.Time.After(this[j].CreationTimestamp.Time)
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