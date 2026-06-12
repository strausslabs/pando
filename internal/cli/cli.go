package cli

import (
	"context"
	"encoding/json"
	"os"

	"github.com/guyStrauss/pando/internal/daemon"
	"github.com/spf13/cobra"
)

type globalFlags struct {
	socket string
	config string
	json   bool
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
	root.PersistentFlags().BoolVar(&g.json, "json", false, "emit machine-readable JSON (for scripts and agents)")
	_ = root.PersistentFlags().MarkHidden("socket")

	root.AddCommand(
		startCmd(g),
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

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
