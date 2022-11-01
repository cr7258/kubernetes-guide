package lib

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"log"
)

var cfgFlags *genericclioptions.ConfigFlags

func InitClient() *kubernetes.Clientset {
	cfgFlags = genericclioptions.NewConfigFlags(true)
	config, err := cfgFlags.ToRawKubeConfigLoader().ClientConfig()
	if err != nil {
		log.Fatalln(err)
	}
	c, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalln(err)
	}
	return c
}

func MergeFlags(cmd *cobra.Command) {
	cfgFlags.AddFlags(cmd.Flags())
}

var ShowLabels bool
var Labels string
var Fields string
var Name string

func RunCmd(f func(c *cobra.Command, args []string) error) {
	cmd := &cobra.Command{
		Use:          "kubectl pods [flags]",
		Short:        "list pods ",
		Example:      "kubectl pods [flags]",
		SilenceUsage: true,
		RunE:         f,
	}
	MergeFlags(cmd)

	cmd.Flags().BoolVar(&ShowLabels, "show-labels", false, "kubectl pods --show-lables")
	cmd.Flags().StringVar(&Labels, "labels", "", "kubectl pods --labels app=ngx or kubectl pods --labels=\"app=ngx\"")
	cmd.Flags().StringVar(&Fields, "fields", "", "kubectl pods --fields=\"status.phase=Running\"")
	cmd.Flags().StringVar(&Name, "name", "", "kubectl pods --name=\"^my*\"")

	err := cmd.Execute()
	if err != nil {
		log.Fatalln(err)
	}
}
