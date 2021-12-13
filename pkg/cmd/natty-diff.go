package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func NewCmdNattyDiff(streams genericclioptions.IOStreams) *cobra.Command {
	// TODO: implement
	o := NewNattyDiffOptions(streams)

	cmd := &cobra.Command{
		Use:          "",
		Short:        "",
		Example:      "",
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Run(); err != nil {
				return err
			}
			return nil
		},
	}

	return cmd
}

type NattyDiffOptions struct {
	genericclioptions.IOStreams
}

func NewNattyDiffOptions(streams genericclioptions.IOStreams) *NattyDiffOptions {
	return &NattyDiffOptions{
		IOStreams: streams,
	}
}

func (o *NattyDiffOptions) Run() error {
	fmt.Fprint(o.Out, "implement me.")

	return nil
}
