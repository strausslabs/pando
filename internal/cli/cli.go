package cli

import (
	"context"

	"github.com/guyStrauss/pando/internal/daemon"
	"github.com/spf13/cobra"
)

type globalFlags struct {
	socket string
	config string
}

func Execute(version string) error {
	g := &globalFlags{}
	root := &cobra.Command{
		Use:           "pando",
		Short:         "Pando — fast multi-worktree dev environments",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&g.socket, "socket", daemon.DefaultSocketPath(), "daemon socket path")
	root.PersistentFlags().StringVarP(&g.config, "config", "f", "pando.config.ts", "path to config file")

	root.AddCommand(
		daemonCmd(g),
		upCmd(g),
		downCmd(g),
		statusCmd(g),
		logsCmd(g),
		execCmd(g),
		restartCmd(g),
		worktreesCmd(g),
	)
	return root.Execute()
}

func ctx() context.Context { return context.Background() }
