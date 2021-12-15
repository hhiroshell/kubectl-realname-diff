package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/kubectl/pkg/cmd/apply"
	"k8s.io/kubectl/pkg/cmd/diff"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/scheme"
	"k8s.io/kubectl/pkg/util/openapi"
	"k8s.io/utils/exec"
)

const maxRetries = 4

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
	cmd.Flags().StringVarP(&options.selector, "selector", "l", options.selector, "Selector (label query) to filter on, supports '=', '==', and '!='.(e.g. -l key1=value1,key2=value2)")
	cmdutil.AddFilenameOptionFlags(cmd, &options.filenameOptions, "contains the configuration to diff")
	cmdutil.AddServerSideApplyFlags(cmd)
	cmdutil.AddFieldManagerFlagVar(cmd, &options.fieldManager, apply.FieldManagerClientSideApply)

	return cmd
}

type NattyDiffOptions struct {
	filenameOptions resource.FilenameOptions

	serverSideApply bool
	fieldManager    string
	forceConflicts  bool

	selector         string
	openAPISchema    openapi.Resources
	discoveryClient  discovery.DiscoveryInterface
	dynamicClient    dynamic.Interface
	dryRunVerifier   *resource.DryRunVerifier
	cmdNamespace     string
	enforceNamespace bool

	builder     *resource.Builder
	diffProgram *diff.DiffProgram
}

func NewNattyDiffOptions(streams genericclioptions.IOStreams) *NattyDiffOptions {
	return &NattyDiffOptions{
		diffProgram: &diff.DiffProgram{
			Exec:      exec.New(),
			IOStreams: streams,
		},
	}
}

func (o *NattyDiffOptions) Complete(factory cmdutil.Factory, cmd *cobra.Command) error {
	var err error

	err = o.filenameOptions.RequireFilenameOrKustomize()
	if err != nil {
		return err
	}

	o.serverSideApply = cmdutil.GetServerSideApplyFlag(cmd)
	o.fieldManager = apply.GetApplyFieldManagerFlag(cmd, o.serverSideApply)
	o.forceConflicts = cmdutil.GetForceConflictsFlag(cmd)
	if o.forceConflicts && !o.serverSideApply {
		return fmt.Errorf("--force-conflicts only works with --server-side")
	}

	if !o.serverSideApply {
		o.openAPISchema, err = factory.OpenAPISchema()
		if err != nil {
			return err
		}
	}

	o.discoveryClient, err = factory.ToDiscoveryClient()
	if err != nil {
		return err
	}

	o.dynamicClient, err = factory.DynamicClient()
	if err != nil {
		return err
	}

	o.dryRunVerifier = resource.NewDryRunVerifier(o.dynamicClient, factory.OpenAPIGetter())

	o.cmdNamespace, o.enforceNamespace, err = factory.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}

	o.builder = factory.NewBuilder()
	return nil
}

func (o *NattyDiffOptions) Run() error {
	differ, err := diff.NewDiffer("LIVE", "MERGED")
	if err != nil {
		return err
	}
	defer differ.TearDown()

	printer := diff.Printer{}

	r := o.builder.
		Unstructured().
		NamespaceParam(o.cmdNamespace).DefaultNamespace().
		FilenameParam(o.enforceNamespace, &o.filenameOptions).
		LabelSelectorParam(o.selector).
		Flatten().
		Do()
	if err := r.Err(); err != nil {
		return err
	}

	err = r.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}

		if err := o.dryRunVerifier.HasSupport(info.Mapping.GroupVersionKind); err != nil {
			return err
		}

		local := info.Object.DeepCopyObject()
		for i := 1; i <= maxRetries; i++ {
			if err = info.Get(); err != nil {
				if !errors.IsNotFound(err) {
					return err
				}
				info.Object = nil
			}

			force := i == maxRetries
			if force {
				fmt.Fprintf(
					o.diffProgram.ErrOut,
					"Object (%v: %v) keeps changing, diffing without lock",
					info.Object.GetObjectKind().GroupVersionKind(),
					info.Name,
				)
			}
			obj := diff.InfoObject{
				LocalObj:        local,
				Info:            info,
				Encoder:         scheme.DefaultJSONEncoder(),
				OpenAPI:         o.openAPISchema,
				Force:           force,
				ServerSideApply: o.serverSideApply,
				FieldManager:    o.fieldManager,
				ForceConflicts:  o.forceConflicts,
				IOStreams:       o.diffProgram.IOStreams,
			}

			err = differ.Diff(obj, printer)
			isConflict := func(err error) bool {
				return err != nil && errors.IsConflict(err)
			}
			if !isConflict(err) {
				break
			}

		}

		apply.WarnIfDeleting(info.Object, o.diffProgram.ErrOut)

		return nil
	})
	if err != nil {
		return err
	}

	return differ.Run(o.diffProgram)
}
