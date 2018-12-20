package main

import (
	goflag "flag"
	"fmt"
	"os"

	"github.com/golang/glog"
	"github.com/inwinstack/pa-svc-syncker/pkg/config"
	"github.com/inwinstack/pa-svc-syncker/pkg/operator"
	"github.com/inwinstack/pa-svc-syncker/pkg/version"
	flag "github.com/spf13/pflag"
)

var (
	kubeconfig string
	namespaces []string
	retry      int
	logSetting string
	group      string
	ver        bool
)

func parserFlags() {
	flag.StringVarP(&kubeconfig, "kubeconfig", "", "", "Absolute path to the kubeconfig file.")
	flag.StringSliceVarP(&namespaces, "ignore-namespaces", "", nil, "Set ignore namespaces for Kubernetes service.")
	flag.IntVarP(&retry, "retry", "", 5, "Number of retry for PA failed job.")
	flag.StringVarP(&logSetting, "log-setting", "", "", "Set security policy log setting name.")
	flag.StringVarP(&group, "group", "", "", "Set security policy group name.")
	flag.BoolVarP(&ver, "version", "", false, "Display the version.")
	flag.CommandLine.AddGoFlagSet(goflag.CommandLine)
	flag.Parse()
}

func main() {
	defer glog.Flush()
	parserFlags()

	glog.Infof("Starting PA Kubernetes service controller...")

	if ver {
		fmt.Fprintf(os.Stdout, "%s\n", version.GetVersion())
		os.Exit(0)
	}

	conf := &config.OperatorConfig{
		Kubeconfig:       kubeconfig,
		IgnoreNamespaces: namespaces,
		Retry:            retry,
		GroupName:        group,
		LogSettingName:   logSetting,
	}

	op := operator.NewMainOperator(conf)
	if err := op.Initialize(); err != nil {
		glog.Fatalf("Error initing operator instance: %v.", err)
	}

	if err := op.Run(); err != nil {
		glog.Fatalf("Error serving operator instance: %s.", err)
	}
}