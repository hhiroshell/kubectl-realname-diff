package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func NewCmdNattyDiff(streams genericclioptions.IOStreams) *cobra.Command {
	options := NewNattyDiffOptions(streams)

	configFlags := genericclioptions.NewConfigFlags(true)
	factory := cmdutil.NewFactory(configFlags)

	cmd := &cobra.Command{
		Use:          "",
		Short:        "",
		Example:      "",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := options.Complete(factory, cmd); err != nil {
				return err
			}
			if err := options.Run(); err != nil {
				return err
			}
			return nil
		},
	}

	configFlags.AddFlags(cmd.Flags())
	cmdutil.AddFilenameOptionFlags(cmd, &options.filenameOptions, "contains the configuration to diff")

	return cmd
}

type NattyDiffOptions struct {
	filenameOptions  resource.FilenameOptions
	cmdNamespace     string
	enforceNamespace bool
	builder          *resource.Builder
	genericclioptions.IOStreams
}

func NewNattyDiffOptions(streams genericclioptions.IOStreams) *NattyDiffOptions {
	return &NattyDiffOptions{
		IOStreams: streams,
	}
}

func (o *NattyDiffOptions) Complete(factory cmdutil.Factory, cmd *cobra.Command) error {
	var err error

	err = o.filenameOptions.RequireFilenameOrKustomize()
	if err != nil {
		return err
	}

	o.cmdNamespace, o.enforceNamespace, err = factory.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}

	o.builder = factory.NewBuilder()
	return nil
}

func (o *NattyDiffOptions) Run() error {
	r := o.builder.
		Unstructured().
		NamespaceParam(o.cmdNamespace).DefaultNamespace().
		FilenameParam(o.enforceNamespace, &o.filenameOptions).
		Flatten().
		Do()
	if err := r.Err(); err != nil {
		return err
	}

	err := r.Visit(func(info *resource.Info, err error) error {
		kind := info.Object.GetObjectKind()
		gvk := kind.GroupVersionKind()
		fmt.Fprint(o.Out, gvk.Kind)

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
