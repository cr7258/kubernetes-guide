package lib

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"io"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"log"
	"os"
)

// k8s 可视化课程 中的代码。 一模一样
func execPod(ns, pod, container string) remotecommand.Executor {
	option := &v1.PodExecOptions{
		Container: container,
		Command:   []string{"sh"},
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       true,
	}
	req := client.CoreV1().RESTClient().Post().Resource("pods").
		Namespace(ns).
		Name(pod).
		SubResource("exec").
		Param("color", "false").
		VersionedParams(
			option,
			scheme.ParameterCodec,
		)

	exec, err := remotecommand.NewSPDYExecutor(restConfig, "POST",
		req.URL())
	if err != nil {
		panic(err)
	}
	return exec
}

type execModel struct {
	items   []v1.Container
	index   int
	cmd     *cobra.Command
	podName string
	ns      string
}

func (m *execModel) Init() tea.Cmd {
	//这里要根据podName取出 container 列表
	m.ns = getNameSpace(m.cmd)
	pod, err := client.CoreV1().Pods(m.ns).
		Get(context.Background(), m.podName, metav1.GetOptions{})
	if err != nil {
		return tea.Quit
	}
	m.items = pod.Spec.Containers

	return nil
}
func (m *execModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up":
			if m.index > 0 {
				m.index--
			}
		case "down":
			if m.index < len(m.items)-1 {
				m.index++
			}
		case "enter":
			err := execPod(m.ns, m.podName, m.items[m.index].Name).Stream(remotecommand.StreamOptions{
				Stdin:  os.Stdin,
				Stdout: os.Stdout,
				Stderr: os.Stderr,
				Tty:    true,
			})
			if err != nil {
				if err != io.EOF {
					log.Println(err)
				}
			}
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *execModel) View() string {
	s := "按上下键选择容器\n\n"

	for i, item := range m.items {
		selected := " "
		if m.index == i {
			selected = "»"
		}
		s += fmt.Sprintf("%s %s(镜像:%s)\n", selected,
			item.Name, item.Image)
	}

	s += "\n按Q退出\n"
	return s
}

// 本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
func runteaExec(args []string, cmd *cobra.Command) {
	if len(args) == 0 {
		log.Println("podname is required")
		return
	}
	var execmodel = &execModel{
		cmd:     cmd,
		podName: args[0],
	}

	//本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
	teaCmd := tea.NewProgram(execmodel)
	if err := teaCmd.Start(); err != nil {
		fmt.Println("start failed:", err)
		os.Exit(1)
	}
}
