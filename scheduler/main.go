package main

import (
	"fmt"
	"k8s.io/component-base/logs"
	"k8s.io/kubernetes/cmd/kube-scheduler/app"
	"myschedular/lib"
	"os"
)
func main() {
	command := app.NewSchedulerCommand(
		app.WithPlugin(lib.TestSchedulingName,lib.NewTestScheduling),
	)
	logs.InitLogs()
	defer logs.FlushLogs()

	if err := command.Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

}