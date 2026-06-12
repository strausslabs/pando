package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/guyStrauss/pando/internal/client"
	"github.com/guyStrauss/pando/internal/discovery"
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
	root.PersistentFlags().StringVar(&g.socket, "socket", "", "daemon socket path (default: per-repo, auto-discovered)")
	root.PersistentFlags().StringVarP(&g.config, "config", "f", "pando.config.ts", "path to config file")
	root.PersistentFlags().BoolVar(&g.json, "json", false, "emit machine-readable JSON (for scripts and agents)")
	_ = root.PersistentFlags().MarkHidden("socket")

	root.AddCommand(
		startCmd(g),
		daemonCmd(g),
		mcpCmd(g, version),
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

// newClient builds a client against the right daemon socket: an explicit
// --socket wins, otherwise the per-repo socket discovered from the current git
// repo.
func newClient(g *globalFlags) (*client.Client, error) {
	if g.socket != "" {
		return client.New(g.socket), nil
	}
	info, found, _ := discovery.Resolve(ctx())
	if !found {
		return nil, fmt.Errorf("no pando daemon for this repo; run `pando start` (or pass --socket)")
	}
	return client.New(info.Socket), nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
