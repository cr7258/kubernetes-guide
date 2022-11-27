package lib

import (
	"context"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
)

var listCmd= &cobra.Command{
	Use:          "list",
	Short:        "list pods ",
	Example:      "kubectl pods list [flags]",
	SilenceUsage: true,
	RunE: func(c *cobra.Command, args []string) error {
		ns,err:=c.Flags().GetString("namespace")
		if err!=nil{return err}
		if ns==""{ns="default"}

		var list =&corev1.PodList{}

		list,err=  client.CoreV1().Pods(ns).List(context.Background(),
			v1.ListOptions{LabelSelector: Labels, FieldSelector: Fields,})
		if err!=nil{return err}

		 FilterListByJSON(list)
		//本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
		table := tablewriter.NewWriter(os.Stdout)
		//设置头

		table.SetHeader(InitHeader(table))
		for _,pod:=range list.Items {
			podRow:=[]string{pod.Name,pod.Namespace,pod.Status.PodIP,
				string(pod.Status.Phase)}
			if ShowLabels {
				podRow=append(podRow, Map2String(pod.Labels))
			}
			table.Append(podRow)
		}
		setTable(table)
		table.Render()
		return nil
	},

}

func addListCmdFlags()  {
	//用来支持 是否 显示标签
	listCmd.Flags().BoolVar(&ShowLabels,"show-labels",false,"kubectl pods --show-lables")
	listCmd.Flags().BoolVar(&Cache,"cache",false,"kubectl pods --cache")
	listCmd.Flags().StringVar(&Labels,"labels","","kubectl pods --lables app=ngx or kubectl pods --lables=\"app=ngx,version=v1\"")
	listCmd.Flags().StringVar(&Fields,"fields","","kubectl pods --fields=\"status.phase=Running\"")
	listCmd.Flags().StringVar(&Search_PodName,"name","","kubectl pods --name=\"^my\"")
}