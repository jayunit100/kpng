package storecmds

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"sigs.k8s.io/kpng/backends/ipvs"
	ipvssink "sigs.k8s.io/kpng/backends/ipvs-as-sink"
	"sigs.k8s.io/kpng/backends/nft"
	"sigs.k8s.io/kpng/jobs/store2api"
	"sigs.k8s.io/kpng/jobs/store2file"
	"sigs.k8s.io/kpng/jobs/store2localdiff"
	"sigs.k8s.io/kpng/localsink"
	"sigs.k8s.io/kpng/localsink/fullstate"
	"sigs.k8s.io/kpng/pkg/proxystore"
)

type SetupFunc func() (ctx context.Context, store *proxystore.Store, err error)

func Commands(setup SetupFunc) []*cobra.Command {
	return []*cobra.Command{
		setup.ToAPICmd(),
		setup.ToFileCmd(),
		setup.ToLocalCmd(),
	}
}

func (c SetupFunc) ToAPICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "to-api",
	}

	cfg := &store2api.Config{}
	cfg.BindFlags(cmd.Flags())

	cmd.RunE = func(_ *cobra.Command, _ []string) (err error) {
		ctx, store, err := c()
		if err != nil {
			return
		}

		j := &store2api.Job{
			Store:  store,
			Config: cfg,
		}
		return j.Run(ctx)
	}

	return cmd
}

func (c SetupFunc) ToFileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "to-file",
		Short: "dump global state to a yaml db file",
	}

	cfg := &store2file.Config{}
	cfg.BindFlags(cmd.Flags())

	cmd.RunE = func(_ *cobra.Command, _ []string) (err error) {
		ctx, store, err := c()
		if err != nil {
			return
		}

		j := &store2file.Job{
			Store:  store,
			Config: cfg,
		}
		return j.Run(ctx)
	}

	return cmd
}

func (c SetupFunc) ToLocalCmd() (cmd *cobra.Command) {
	cmd = &cobra.Command{
		Use: "to-local",
	}

	var ctx context.Context
	job := &store2localdiff.Job{}

	cmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) (err error) {
		ctx, job.Store, err = c()
		return
	}

	cmd.AddCommand(LocalCmds(func(sink localsink.Sink) error {
		job.Sink = sink
		return job.Run(ctx)
	})...)

	return
}

func LocalCmds(run func(sink localsink.Sink) error) (cmds []*cobra.Command) {
	// classic backends
	cfg := &localsink.Config{}
	sink := fullstate.New(cfg)

	for _, cmd := range BackendCmds(sink, func() error { return run(sink) }) {
		cfg.BindFlags(cmd.Flags())
		cmds = append(cmds, cmd)
	}

	// sink backends
	ipvsBackend := ipvssink.New()

	cmd := &cobra.Command{
		Use: "to-ipvs2",
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(ipvsBackend.Sink())
		},
	}

	ipvsBackend.BindFlags(cmd.Flags())

	cmds = append(cmds, cmd)

	return
}

func BackendCmds(sink *fullstate.Sink, run func() error) []*cobra.Command {
	return []*cobra.Command{
		{Use: "to-iptables", RunE: unimplemented},
		ipvsCommand(sink, run),
		nftCommand(sink, run),
	}
}

func unimplemented(_ *cobra.Command, _ []string) error {
	return errors.New("not implemented")
}

func nftCommand(sink *fullstate.Sink, run func() error) *cobra.Command {
	cmd := &cobra.Command{
		Use: "to-nft",
	}

	nft.BindFlags(cmd.Flags())

	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		nft.PreRun()
		sink.Callback = nft.Callback
		return run()
	}

	return cmd
}

func ipvsCommand(sink *fullstate.Sink, run func() error) *cobra.Command {
	cmd := &cobra.Command{
		Use: "to-ipvs",
	}

	ipvs.BindFlags(cmd.Flags())

	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		ipvs.PreRun()
		sink.Callback = ipvs.Callback
		return run()
	}

	return cmd
}
