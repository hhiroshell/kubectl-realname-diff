package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/dynamic"
	"k8s.io/kubectl/pkg/cmd/apply"
	"k8s.io/kubectl/pkg/cmd/diff"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/scheme"
	"k8s.io/kubectl/pkg/util/openapi"
	"k8s.io/utils/exec"
)

const (
	maxRetries    = 4
	realNameLabel = "realname-diff/realname"
)

func NewCmdRealnameDiff(streams genericclioptions.IOStreams) *cobra.Command {
	options := NewRealnameDiffOptions(streams)

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

type RealnameDiffOptions struct {
	filenameOptions resource.FilenameOptions

	serverSideApply bool
	fieldManager    string
	forceConflicts  bool

	selector         string
	openAPISchema    openapi.Resources
	dynamicClient    dynamic.Interface
	dryRunVerifier   *resource.DryRunVerifier
	cmdNamespace     string
	enforceNamespace bool

	builder     *resource.Builder
	diffProgram *diff.DiffProgram
}

func NewRealnameDiffOptions(streams genericclioptions.IOStreams) *RealnameDiffOptions {
	return &RealnameDiffOptions{
		diffProgram: &diff.DiffProgram{
			Exec:      exec.New(),
			IOStreams: streams,
		},
	}
}

// InfoObject is an implementation of the Object interface. It gets all the information from the Info object.
type RealnameDiffInfoObject struct {
	infoObj     diff.InfoObject
	hasRealName bool
}

var _ diff.Object = &RealnameDiffInfoObject{}

// Returns the live version of the object
func (obj RealnameDiffInfoObject) Live() runtime.Object {
	return obj.infoObj.Live()
}

// Returns the "merged" object, as it would look like if applied or created.
func (obj RealnameDiffInfoObject) Merged() (runtime.Object, error) {
	if obj.hasRealName {
		return obj.infoObj.LocalObj, nil
	}
	return obj.infoObj.Merged()
}

func (obj RealnameDiffInfoObject) Name() string {
	return obj.infoObj.Name()
}

func realName(obj runtime.Object) string {
	labels := obj.(*unstructured.Unstructured).GetLabels()

	for k, v := range labels {
		if k == realNameLabel {
			return v
		}
	}
	return ""
}

// Get retrieves the object from the Namespace and Name fields
func update(info *resource.Info, name string) error {
	gvk := info.Object.GetObjectKind().GroupVersionKind()
	res, err := resource.NewHelper(info.Client, info.Mapping).List(info.Namespace, info.ResourceVersion, &metav1.ListOptions{
		TypeMeta: metav1.TypeMeta{
			Kind:       gvk.Kind,
			APIVersion: gvk.GroupVersion().String(),
		},
		LabelSelector: realNameLabel + "=" + name,
	})
	if err != nil {
		return err
	}

	list := res.(*unstructured.UnstructuredList)
	if len(list.Items) == 0 {
		return errors.NewNotFound(schema.GroupResource{
			Group:    gvk.Group,
			Resource: gvk.Kind,
		}, name)
	}
	if len(list.Items) > 1 {
		return fmt.Errorf("more than two objects have same realname label")
	}

	item := list.Items[0]
	info.Object = item.DeepCopyObject()
	info.Name = item.GetName()
	info.ResourceVersion = item.GetResourceVersion()
	return nil
}

func isNotFound(err error) bool {
	return err != nil && errors.IsNotFound(err)
}

func isConflict(err error) bool {
	return err != nil && errors.IsConflict(err)
}

func (o *RealnameDiffOptions) Complete(factory cmdutil.Factory, cmd *cobra.Command) error {
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

func (o *RealnameDiffOptions) Run() error {
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
			hasOriginalName := false
			if on := realName(local); len(on) > 0 {
				err = update(info, on)
				hasOriginalName = true
			} else {
				err = info.Get()
			}
			if isNotFound(err) {
				info.Object = nil
			} else if err != nil {
				return err
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
			obj := RealnameDiffInfoObject{
				infoObj: diff.InfoObject{
					LocalObj:        local,
					Info:            info,
					Encoder:         scheme.DefaultJSONEncoder(),
					OpenAPI:         o.openAPISchema,
					Force:           force,
					ServerSideApply: o.serverSideApply,
					FieldManager:    o.fieldManager,
					ForceConflicts:  o.forceConflicts,
					IOStreams:       o.diffProgram.IOStreams,
				},
				hasRealName: hasOriginalName,
			}

			err = differ.Diff(obj, printer)
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
