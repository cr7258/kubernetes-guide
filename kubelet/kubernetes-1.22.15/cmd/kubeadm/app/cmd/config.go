/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"sort"
	"strings"

	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	kubeadmscheme "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/scheme"
	kubeadmapiv1 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta3"
	outputapischeme "k8s.io/kubernetes/cmd/kubeadm/app/apis/output/scheme"
	outputapiv1alpha1 "k8s.io/kubernetes/cmd/kubeadm/app/apis/output/v1alpha1"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/options"
	cmdutil "k8s.io/kubernetes/cmd/kubeadm/app/cmd/util"
	"k8s.io/kubernetes/cmd/kubeadm/app/componentconfigs"
	"k8s.io/kubernetes/cmd/kubeadm/app/constants"
	"k8s.io/kubernetes/cmd/kubeadm/app/features"
	"k8s.io/kubernetes/cmd/kubeadm/app/images"
	configutil "k8s.io/kubernetes/cmd/kubeadm/app/util/config"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/output"
	utilruntime "k8s.io/kubernetes/cmd/kubeadm/app/util/runtime"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	utilsexec "k8s.io/utils/exec"

	"github.com/lithammer/dedent"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
)

// newCmdConfig returns cobra.Command for "kubeadm config" command
func newCmdConfig(out io.Writer) *cobra.Command {
	var kubeConfigFile string

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration for a kubeadm cluster persisted in a ConfigMap in the cluster",
		Long: fmt.Sprintf(dedent.Dedent(`
			There is a ConfigMap in the %s namespace called %q that kubeadm uses to store internal configuration about the
			cluster. kubeadm CLI v1.8.0+ automatically creates this ConfigMap with the config used with 'kubeadm init', but if you
			initialized your cluster using kubeadm v1.7.x or lower, you must use the 'config upload' command to create this
			ConfigMap. This is required so that 'kubeadm upgrade' can configure your upgraded cluster correctly.
		`), metav1.NamespaceSystem, constants.KubeadmConfigConfigMap),
		// Without this callback, if a user runs just the "upload"
		// command without a subcommand, or with an invalid subcommand,
		// cobra will print usage information, but still exit cleanly.
		// We want to return an error code in these cases so that the
		// user knows that their command was invalid.
		RunE: cmdutil.SubCmdRunE("config"),
	}

	options.AddKubeConfigFlag(cmd.PersistentFlags(), &kubeConfigFile)

	kubeConfigFile = cmdutil.GetKubeConfigPath(kubeConfigFile)
	cmd.AddCommand(newCmdConfigPrint(out))
	cmd.AddCommand(newCmdConfigMigrate(out))
	cmd.AddCommand(newCmdConfigImages(out))
	return cmd
}

// newCmdConfigPrint returns cobra.Command for "kubeadm config print" command
func newCmdConfigPrint(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "print",
		Short: "Print configuration",
		Long: dedent.Dedent(`
			This command prints configurations for subcommands provided.
			For details, see: https://pkg.go.dev/k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm#section-directories`),
		RunE: cmdutil.SubCmdRunE("print"),
	}
	cmd.AddCommand(newCmdConfigPrintInitDefaults(out))
	cmd.AddCommand(newCmdConfigPrintJoinDefaults(out))
	return cmd
}

// newCmdConfigPrintInitDefaults returns cobra.Command for "kubeadm config print init-defaults" command
func newCmdConfigPrintInitDefaults(out io.Writer) *cobra.Command {
	return newCmdConfigPrintActionDefaults(out, "init", getDefaultInitConfigBytes)
}

// newCmdConfigPrintJoinDefaults returns cobra.Command for "kubeadm config print join-defaults" command
func newCmdConfigPrintJoinDefaults(out io.Writer) *cobra.Command {
	return newCmdConfigPrintActionDefaults(out, "join", getDefaultNodeConfigBytes)
}

func newCmdConfigPrintActionDefaults(out io.Writer, action string, configBytesProc func() ([]byte, error)) *cobra.Command {
	kinds := []string{}
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("%s-defaults", action),
		Short: fmt.Sprintf("Print default %s configuration, that can be used for 'kubeadm %s'", action, action),
		Long: fmt.Sprintf(dedent.Dedent(`
			This command prints objects such as the default %s configuration that is used for 'kubeadm %s'.

			Note that sensitive values like the Bootstrap Token fields are replaced with placeholder values like %q in order to pass validation but
			not perform the real computation for creating a token.
		`), action, action, configutil.PlaceholderToken.Token.String()),
		RunE: func(cmd *cobra.Command, args []string) error {
			groups, err := mapLegacyKindsToGroups(kinds)
			if err != nil {
				return err
			}
			return runConfigPrintActionDefaults(out, groups, configBytesProc)
		},
		Args: cobra.NoArgs,
	}
	cmd.Flags().StringSliceVar(&kinds, "component-configs", kinds,
		fmt.Sprintf("A comma-separated list for component config API objects to print the default values for. Available values: %v. If this flag is not set, no component configs will be printed.", getSupportedComponentConfigKinds()))
	return cmd
}

func runConfigPrintActionDefaults(out io.Writer, componentConfigs []string, configBytesProc func() ([]byte, error)) error {
	initialConfig, err := configBytesProc()
	if err != nil {
		return err
	}

	allBytes := [][]byte{initialConfig}
	for _, componentConfig := range componentConfigs {
		cfgBytes, err := getDefaultComponentConfigBytes(componentConfig)
		if err != nil {
			return err
		}
		allBytes = append(allBytes, cfgBytes)
	}

	fmt.Fprint(out, string(bytes.Join(allBytes, []byte(constants.YAMLDocumentSeparator))))
	return nil
}

func getDefaultComponentConfigBytes(group string) ([]byte, error) {
	defaultedInitConfig, err := configutil.DefaultedStaticInitConfiguration()
	if err != nil {
		return []byte{}, err
	}

	componentCfg, ok := defaultedInitConfig.ComponentConfigs[group]
	if !ok {
		return []byte{}, errors.Errorf("cannot get defaulted config for component group %q", group)
	}

	return componentCfg.Marshal()
}

// legacyKindToGroupMap maps between the old API object types and the new way of specifying component configs (by group)
var legacyKindToGroupMap = map[string]string{
	"KubeletConfiguration":   componentconfigs.KubeletGroup,
	"KubeProxyConfiguration": componentconfigs.KubeProxyGroup,
}

// getSupportedComponentConfigKinds returns all currently supported component config API object names
func getSupportedComponentConfigKinds() []string {
	objects := []string{}
	for componentType := range legacyKindToGroupMap {
		objects = append(objects, string(componentType))
	}
	sort.Strings(objects)
	return objects
}

func mapLegacyKindsToGroups(kinds []string) ([]string, error) {
	groups := []string{}
	for _, kind := range kinds {
		group, ok := legacyKindToGroupMap[kind]
		if ok {
			groups = append(groups, group)
		} else {
			return nil, errors.Errorf("--component-configs needs to contain some of %v", getSupportedComponentConfigKinds())
		}
	}
	return groups, nil
}

func getDefaultInitConfigBytes() ([]byte, error) {
	internalcfg, err := configutil.DefaultedStaticInitConfiguration()
	if err != nil {
		return []byte{}, err
	}

	return configutil.MarshalKubeadmConfigObject(internalcfg)
}

func getDefaultNodeConfigBytes() ([]byte, error) {
	internalcfg, err := configutil.DefaultedJoinConfiguration(&kubeadmapiv1.JoinConfiguration{
		Discovery: kubeadmapiv1.Discovery{
			BootstrapToken: &kubeadmapiv1.BootstrapTokenDiscovery{
				Token:                    configutil.PlaceholderToken.Token.String(),
				APIServerEndpoint:        "kube-apiserver:6443",
				UnsafeSkipCAVerification: true, // TODO: UnsafeSkipCAVerification: true needs to be set for validation to pass, but shouldn't be recommended as the default
			},
		},
		NodeRegistration: kubeadmapiv1.NodeRegistrationOptions{
			CRISocket: constants.DefaultDockerCRISocket, // avoid CRI detection
		},
	})
	if err != nil {
		return []byte{}, err
	}

	return configutil.MarshalKubeadmConfigObject(internalcfg)
}

// newCmdConfigMigrate returns cobra.Command for "kubeadm config migrate" command
func newCmdConfigMigrate(out io.Writer) *cobra.Command {
	var oldCfgPath, newCfgPath string
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Read an older version of the kubeadm configuration API types from a file, and output the similar config object for the newer version",
		Long: fmt.Sprintf(dedent.Dedent(`
			This command lets you convert configuration objects of older versions to the latest supported version,
			locally in the CLI tool without ever touching anything in the cluster.
			In this version of kubeadm, the following API versions are supported:
			- %s

			Further, kubeadm can only write out config of version %q, but read both types.
			So regardless of what version you pass to the --old-config parameter here, the API object will be
			read, deserialized, defaulted, converted, validated, and re-serialized when written to stdout or
			--new-config if specified.

			In other words, the output of this command is what kubeadm actually would read internally if you
			submitted this file to "kubeadm init"
		`), kubeadmapiv1.SchemeGroupVersion, kubeadmapiv1.SchemeGroupVersion),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(oldCfgPath) == 0 {
				return errors.New("the --old-config flag is mandatory")
			}

			oldCfgBytes, err := ioutil.ReadFile(oldCfgPath)
			if err != nil {
				return err
			}

			outputBytes, err := configutil.MigrateOldConfig(oldCfgBytes)
			if err != nil {
				return err
			}

			if newCfgPath == "" {
				fmt.Fprint(out, string(outputBytes))
			} else {
				if err := ioutil.WriteFile(newCfgPath, outputBytes, 0644); err != nil {
					return errors.Wrapf(err, "failed to write the new configuration to the file %q", newCfgPath)
				}
			}
			return nil
		},
		Args: cobra.NoArgs,
	}
	cmd.Flags().StringVar(&oldCfgPath, "old-config", "", "Path to the kubeadm config file that is using an old API version and should be converted. This flag is mandatory.")
	cmd.Flags().StringVar(&newCfgPath, "new-config", "", "Path to the resulting equivalent kubeadm config file using the new API version. Optional, if not specified output will be sent to STDOUT.")
	return cmd
}

// newCmdConfigImages returns the "kubeadm config images" command
func newCmdConfigImages(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "images",
		Short: "Interact with container images used by kubeadm",
		RunE:  cmdutil.SubCmdRunE("images"),
	}
	cmd.AddCommand(newCmdConfigImagesList(out, nil))
	cmd.AddCommand(newCmdConfigImagesPull())
	return cmd
}

// newCmdConfigImagesPull returns the `kubeadm config images pull` command
func newCmdConfigImagesPull() *cobra.Command {
	externalClusterCfg := &kubeadmapiv1.ClusterConfiguration{}
	kubeadmscheme.Scheme.Default(externalClusterCfg)
	externalInitCfg := &kubeadmapiv1.InitConfiguration{}
	kubeadmscheme.Scheme.Default(externalInitCfg)
	var cfgPath, featureGatesString string
	var err error

	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull images used by kubeadm",
		RunE: func(_ *cobra.Command, _ []string) error {
			externalClusterCfg.FeatureGates, err = features.NewFeatureGate(&features.InitFeatureGates, featureGatesString)
			if err != nil {
				return err
			}
			internalcfg, err := configutil.LoadOrDefaultInitConfiguration(cfgPath, externalInitCfg, externalClusterCfg)
			if err != nil {
				return err
			}
			containerRuntime, err := utilruntime.NewContainerRuntime(utilsexec.New(), internalcfg.NodeRegistration.CRISocket)
			if err != nil {
				return err
			}
			return PullControlPlaneImages(containerRuntime, &internalcfg.ClusterConfiguration)
		},
		Args: cobra.NoArgs,
	}
	AddImagesCommonConfigFlags(cmd.PersistentFlags(), externalClusterCfg, &cfgPath, &featureGatesString)
	cmdutil.AddCRISocketFlag(cmd.PersistentFlags(), &externalInitCfg.NodeRegistration.CRISocket)

	return cmd
}

// ImagesPull is the struct used to hold information relating to image pulling
type ImagesPull struct {
	runtime utilruntime.ContainerRuntime
	images  []string
}

// NewImagesPull initializes and returns the `kubeadm config images pull` command
func NewImagesPull(runtime utilruntime.ContainerRuntime, images []string) *ImagesPull {
	return &ImagesPull{
		runtime: runtime,
		images:  images,
	}
}

// PullControlPlaneImages pulls all images that the ImagesPull knows about
func PullControlPlaneImages(runtime utilruntime.ContainerRuntime, cfg *kubeadmapi.ClusterConfiguration) error {
	images := images.GetControlPlaneImages(cfg)
	for _, image := range images {
		if err := runtime.PullImage(image); err != nil {
			return errors.Wrapf(err, "failed to pull image %q", image)
		}
		fmt.Printf("[config/images] Pulled %s\n", image)
	}
	return nil
}

// newCmdConfigImagesList returns the "kubeadm config images list" command
func newCmdConfigImagesList(out io.Writer, mockK8sVersion *string) *cobra.Command {
	externalcfg := &kubeadmapiv1.ClusterConfiguration{}
	kubeadmscheme.Scheme.Default(externalcfg)
	var cfgPath, featureGatesString string
	var err error

	// This just sets the Kubernetes version for unit testing so kubeadm won't try to
	// lookup the latest release from the internet.
	if mockK8sVersion != nil {
		externalcfg.KubernetesVersion = *mockK8sVersion
	}

	outputFlags := output.NewOutputFlags(&imageTextPrintFlags{}).WithTypeSetter(outputapischeme.Scheme).WithDefaultOutput(output.TextOutput)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Print a list of images kubeadm will use. The configuration file is used in case any images or image repositories are customized",
		RunE: func(_ *cobra.Command, _ []string) error {
			externalcfg.FeatureGates, err = features.NewFeatureGate(&features.InitFeatureGates, featureGatesString)
			if err != nil {
				return err
			}

			printer, err := outputFlags.ToPrinter()
			if err != nil {
				return err
			}

			imagesList, err := NewImagesList(cfgPath, externalcfg)
			if err != nil {
				return err
			}

			return imagesList.Run(out, printer)
		},
		Args: cobra.NoArgs,
	}
	outputFlags.AddFlags(cmd)
	AddImagesCommonConfigFlags(cmd.PersistentFlags(), externalcfg, &cfgPath, &featureGatesString)
	return cmd
}

// NewImagesList returns the underlying struct for the "kubeadm config images list" command
func NewImagesList(cfgPath string, cfg *kubeadmapiv1.ClusterConfiguration) (*ImagesList, error) {
	initcfg, err := configutil.LoadOrDefaultInitConfiguration(cfgPath, cmdutil.DefaultInitConfiguration(), cfg)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert cfg to an internal cfg")
	}

	return &ImagesList{
		cfg: initcfg,
	}, nil
}

// ImagesList defines the struct used for "kubeadm config images list"
type ImagesList struct {
	cfg *kubeadmapi.InitConfiguration
}

// imageTextPrinter prints image info in a text form
type imageTextPrinter struct {
	output.TextPrinter
}

// PrintObj is an implementation of ResourcePrinter.PrintObj for plain text output
func (itp *imageTextPrinter) PrintObj(obj runtime.Object, writer io.Writer) error {
	var err error
	if imgs, ok := obj.(*outputapiv1alpha1.Images); ok {
		_, err = fmt.Fprintln(writer, strings.Join(imgs.Images, "\n"))
	} else {
		err = errors.New("unexpected object type")
	}
	return err
}

// imageTextPrintFlags provides flags necessary for printing image in a text form.
type imageTextPrintFlags struct{}

// ToPrinter returns kubeadm printer for the text output format
func (ipf *imageTextPrintFlags) ToPrinter(outputFormat string) (output.Printer, error) {
	if outputFormat == output.TextOutput {
		return &imageTextPrinter{}, nil
	}
	return nil, genericclioptions.NoCompatiblePrinterError{OutputFormat: &outputFormat, AllowedFormats: []string{output.TextOutput}}
}

// Run runs the images command and writes the result to the io.Writer passed in
func (i *ImagesList) Run(out io.Writer, printer output.Printer) error {
	imgs := images.GetControlPlaneImages(&i.cfg.ClusterConfiguration)

	if err := printer.PrintObj(&outputapiv1alpha1.Images{Images: imgs}, out); err != nil {
		return errors.Wrap(err, "unable to print images")
	}

	return nil
}

// AddImagesCommonConfigFlags adds the flags that configure kubeadm (and affect the images kubeadm will use)
func AddImagesCommonConfigFlags(flagSet *flag.FlagSet, cfg *kubeadmapiv1.ClusterConfiguration, cfgPath *string, featureGatesString *string) {
	options.AddKubernetesVersionFlag(flagSet, &cfg.KubernetesVersion)
	options.AddFeatureGatesStringFlag(flagSet, featureGatesString)
	options.AddImageMetaFlags(flagSet, &cfg.ImageRepository)
	options.AddConfigFlag(flagSet, cfgPath)
}
