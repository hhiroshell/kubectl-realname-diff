package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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

	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

var (
	diffLong = `Diffs live and local resources ignoring Kustomize hash-suffixes.

Normally, "kubectl realname-diff" works the same as "kubectl diff", but if you
set "real name" as a label, local and live resources with the same label will be
compared.`

	diffExample = `  # Make sure you have already labeled the resources with
  # "realname-diff/realname: [real name]". For a complete example, see:
  # https://github.com/hhiroshell/kubectl-realname-diff/tree/main/example 

  # Diff resources included in the result of kustomize build
  kustomize build ./example | kubectl realname-diff -f -

  # Also you can use kubectl's built-in kustomize
  kubectl realname-diff -k ./example`
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
		Use:                   "kubectl realname-diff -f FILENAME",
		DisableFlagsInUseLine: true,
		Short:                 "Diff live and local resources ignoring Kustomize hash-suffixes.",
		Long:                  diffLong,
		Example:               diffExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckDiffErr(options.Complete(factory, cmd))
			cmdutil.CheckDiffErr(validateArgs(cmd, args))

			if err := options.Run(); err != nil {
				if exitErr := diffError(err); exitErr != nil {
					os.Exit(exitErr.ExitStatus())
				}
				cmdutil.CheckDiffErr(err)
			}
		},
	}

	configFlags.AddFlags(cmd.Flags())
	cmd.Flags().StringVarP(&options.selector, "selector", "l", options.selector, "Selector (label query) to filter on, supports '=', '==', and '!='.(e.g. -l key1=value1,key2=value2)")
	cmdutil.AddFilenameOptionFlags(cmd, &options.filenameOptions, "contains the configuration to diff")
	cmdutil.AddServerSideApplyFlags(cmd)
	cmdutil.AddFieldManagerFlagVar(cmd, &options.fieldManager, apply.FieldManagerClientSideApply)

	return cmd
}

func validateArgs(cmd *cobra.Command, args []string) error {
	if len(args) != 0 {
		return cmdutil.UsageErrorf(cmd, "Unexpected args: %v", args)
	}
	return nil
}

// diffError returns the ExitError if the status code is less than 1,
// nil otherwise.
func diffError(err error) exec.ExitError {
	if err, ok := err.(exec.ExitError); ok && err.ExitStatus() <= 1 {
		return err
	}
	return nil
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

// RealnameDiffInfoObject is an implementation of the diff.Object interface.
// It has all the information from the diff.InfoObject and whether the object has a real name label.
type RealnameDiffInfoObject struct {
	infoObj diff.InfoObject
}

var _ diff.Object = &RealnameDiffInfoObject{}

// Live Returns the live version of the object
func (obj RealnameDiffInfoObject) Live() runtime.Object {
	if !obj.nameWillBeChanged() {
		return obj.infoObj.Live()
	}

	unstructured := obj.infoObj.Live().(*unstructured.Unstructured)
	if unstructured.GetKind() == "Secret" {
		annotations := unstructured.GetAnnotations()
		if _, ok := annotations["kubectl.kubernetes.io/last-applied-configuration"]; ok {
			annotations["kubectl.kubernetes.io/last-applied-configuration"] = "***"
		}
		unstructured.SetAnnotations(annotations)
	}
	return unstructured
}

// Merged returns the "merged" object, as it would look like if applied or created.
func (obj RealnameDiffInfoObject) Merged() (runtime.Object, error) {
	if !obj.nameWillBeChanged() {
		return obj.infoObj.Merged()
	}

	helper := resource.NewHelper(obj.infoObj.Info.Client, obj.infoObj.Info.Mapping).
		DryRun(true).
		WithFieldManager(obj.infoObj.FieldManager)
	return helper.CreateWithOptions(
		obj.infoObj.Info.Namespace,
		true,
		obj.infoObj.LocalObj,
		&metav1.CreateOptions{},
	)
}

func (obj RealnameDiffInfoObject) nameWillBeChanged() bool {
	if obj.infoObj.Live() == nil {
		return false
	}

	live := obj.infoObj.Live().(*unstructured.Unstructured).GetName()
	local := obj.infoObj.LocalObj.(*unstructured.Unstructured).GetName()
	return local != live
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

// getWithRealName retrieves the object from the "realname-diff/realname" label.
// If the object is not found, it will try to retrieve it from the name.
func getWithRealName(info *resource.Info, name string) error {
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
	if len(list.Items) > 1 {
		return fmt.Errorf("more than two objects have same realname label")
	}

	if len(list.Items) == 1 {
		unstructured := list.Items[0]
		info.Object = unstructured.DeepCopyObject()
		info.ResourceVersion = unstructured.GetResourceVersion()
	} else if len(list.Items) == 0 {
		obj, err := resource.NewHelper(info.Client, info.Mapping).Get(info.Namespace, name)
		if isNotFound(err) {
			return errors.NewNotFound(schema.GroupResource{
				Group:    gvk.Group,
				Resource: gvk.Kind,
			}, name)
		}
		info.Object = obj
		info.ResourceVersion, err = meta.NewAccessor().ResourceVersion(obj)
		if err != nil {
			return err
		}
	}

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
			if on := realName(local); len(on) > 0 {
				err = getWithRealName(info, on)
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
