package lib

import (
	"fmt"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"k8s.io/apimachinery/pkg/labels"
	"os"
)

var cacheCmd= &cobra.Command{
	Use:          "cache",
	Short:        "pods by cache",
	Hidden:true,
	RunE: func(c *cobra.Command, args []string) error {

		ns,err:=c.Flags().GetString("namespace")
		if err!=nil{return err}
		if ns==""{ns="default"}

		pods,err:= fact.Core().V1().Pods().Lister().Pods(ns).
			List(labels.Everything())
		if err!=nil{return err}
		fmt.Println("从缓存取")
		table := tablewriter.NewWriter(os.Stdout)
		//设置头
		table.SetHeader(InitHeader(table))
		for _,pod:=range pods {
			podRow:=[]string{pod.Name,pod.Namespace,pod.Status.PodIP,
				string(pod.Status.Phase)}
			table.Append(podRow)
		}
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
		table.Render()
		return nil
	},

}
