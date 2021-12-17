package main

import (
	"os"

	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"hhiroshell.github.com/kubectl-realname-diff/pkg/cmd"
)

func main() {
	pflag.CommandLine = pflag.NewFlagSet("kubectl-realname-diff", pflag.ExitOnError)

	root := cmd.NewCmdRealnameDiff(genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	})
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
