package main

import (
	"context"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kubectl-plugins/lib"
	"os"
	"regexp"
)

var client = lib.InitClient()

func main() {
	lib.RunCmd(run)
}

func run(c *cobra.Command, args []string) error {
	ns, err := c.Flags().GetString("namespace")
	if err != nil {
		return err
	}
	if ns == "" {
		ns = "default"
	}

	list, err := client.CoreV1().Pods(ns).List(context.Background(), v1.ListOptions{
		LabelSelector: lib.Labels,
		FieldSelector: lib.Fields,
	})
	if err != nil {
		return err
	}
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader(lib.InitHeader(table))

	for _, pod := range list.Items {
		podRow := []string{pod.Name, pod.Namespace, pod.Status.PodIP,
			string(pod.Status.Phase)}
		if lib.ShowLabels {
			podRow = append(podRow, lib.Map2String(pod.Labels))
		}
		if lib.Name != "" {
			if m, err := regexp.MatchString(lib.Name, pod.Name); err == nil && !m {
				continue
			}
		}
		table.Append(podRow)
	}
	table.Render()
	return nil
}
