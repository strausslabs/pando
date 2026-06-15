package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/strausslabs/pando/internal/client"
	"github.com/strausslabs/pando/internal/discovery"
)

const updateAvailableMsg = "a newer pando is available: %s → %s · brew upgrade pando\n"

const defaultConfig = "pando.star"

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
	root.PersistentFlags().StringVarP(&g.config, "config", "f", defaultConfig, "path to config file")
	root.PersistentFlags().BoolVar(&g.json, "json", false, "emit machine-readable JSON (for scripts and agents)")
	_ = root.PersistentFlags().MarkHidden("socket")

	root.AddCommand(
		startCmd(g, version),
		stopCmd(g),
		daemonCmd(g, version),
		mcpCmd(g, version),
		setupCmd(g),
		upCmd(g),
		downCmd(g),
		statusCmd(g),
		logsCmd(g),
		execCmd(g),
		restartCmd(g),
		worktreesCmd(g),
	)
	err := root.Execute()
	if err != nil && g.json {
		_ = printJSON(map[string]string{"error": err.Error()})
		return Handled{err}
	}
	return err
}

type Handled struct{ error }

func ctx() context.Context { return context.Background() }

func newClient(g *globalFlags) (*client.Client, error) {
	if g.config != "" && g.config != defaultConfig {
		fmt.Fprintf(os.Stderr, "warning: --config %q ignored; the running daemon uses its own config (restart it to change)\n", g.config)
	}
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
