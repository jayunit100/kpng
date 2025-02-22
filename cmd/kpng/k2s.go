package main

import (
	"context"
	"fmt"
	"os"
	kube2store2 "sigs.k8s.io/kpng/server/jobs/kube2store"
//	storecmds2 "sigs.k8s.io/kpng/server/pkg/cmd/storecmds"
	proxystore2 "sigs.k8s.io/kpng/server/pkg/proxystore"

	"github.com/spf13/cobra"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// FIXME separate package
var (
	kubeConfig string
	kubeServer string
	k2sCfg     = &kube2store2.Config{}
)

func kube2storeCmd() *cobra.Command {
	// kube to * command
	k2sCmd := &cobra.Command{
		Use:   "kube",
		Short: "watch Kubernetes API to the global state",
	}

	flags := k2sCmd.PersistentFlags()
	flags.StringVar(&kubeConfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster. Defaults to envvar KUBECONFIG.")
	flags.StringVar(&kubeServer, "server", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")

	k2sCfg.BindFlags(k2sCmd.PersistentFlags())
//	k2sCmd.AddCommand(storecmds2.Commands(setupKube2store)...)

	return k2sCmd
}

func setupKube2store() (ctx context.Context, store *proxystore2.Store, err error) {
	ctx = setupGlobal()

	// setup k8s client
	if kubeConfig == "" {
		kubeConfig = os.Getenv("KUBECONFIG")
	}

	cfg, err := clientcmd.BuildConfigFromFlags(kubeServer, kubeConfig)
	if err != nil {
		err = fmt.Errorf("Error building kubeconfig: %w", err)
		return
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		err = fmt.Errorf("Error building kubernetes clientset: %w", err)
		return
	}

	// create the store
	store = proxystore2.New()

	// start kube2store
	go kube2store2.Job{
		Kube:   kubeClient,
		Store:  store,
		Config: k2sCfg,
	}.Run(ctx)

	return
}
