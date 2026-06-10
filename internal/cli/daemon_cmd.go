package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/guyStrauss/pando/internal/config"
	"github.com/guyStrauss/pando/internal/daemon"
	"github.com/guyStrauss/pando/internal/engine"
	"github.com/guyStrauss/pando/internal/executor"
	"github.com/guyStrauss/pando/internal/logbuf"
	"github.com/guyStrauss/pando/internal/resource"
	"github.com/guyStrauss/pando/internal/scheduler"
	"github.com/guyStrauss/pando/internal/state"
	"github.com/guyStrauss/pando/internal/worktree"
	"github.com/spf13/cobra"
)

func daemonCmd(g *globalFlags) *cobra.Command {
	var tcpAddr string
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run the Pando daemon",
		RunE: func(c *cobra.Command, args []string) error {
			return runDaemon(g, tcpAddr)
		},
	}
	cmd.Flags().StringVar(&tcpAddr, "ui-addr", "127.0.0.1:7420", "loopback address for the web UI (empty to disable)")
	return cmd
}

func runDaemon(g *globalFlags, tcpAddr string) error {
	loader, err := config.NewLoader()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	stack, err := loader.LoadFile(ctx, g.config)
	if err != nil {
		return err
	}

	stateDir := ".pando"
	store, err := state.Open(filepath.Join(stateDir, "state.json"))
	if err != nil {
		return err
	}

	logs := logbuf.NewStore(5000)
	proc := executor.NewEngine(logs, time.Now)

	execs := map[resource.Kind]scheduler.Executor{
		resource.KindTask:  proc,
		resource.KindLocal: proc,
	}
	execers := map[resource.Kind]engine.Execer{
		resource.KindTask:  proc,
		resource.KindLocal: proc,
	}

	eng := engine.New(engine.Config{
		StackName: stack.Name,
		Allocator: worktree.DefaultAllocator(),
		Store:     store,
		Logs:      logs,
		Executors: execs,
		Execers:   execers,
	})

	wts, err := discoverWorktrees(ctx)
	if err != nil {
		return err
	}
	for _, wt := range wts {
		if err := eng.Register(wt, stack); err != nil {
			fmt.Fprintf(os.Stderr, "warning: register %s: %v\n", wt.Slug, err)
		}
	}

	srv := daemon.NewServer(eng, logs)

	if tcpAddr != "" {
		go func() {
			if err := srv.ServeTCP(ctx, tcpAddr); err != nil {
				fmt.Fprintf(os.Stderr, "ui server: %v\n", err)
			}
		}()
		fmt.Printf("pando daemon: ui on http://%s\n", tcpAddr)
	}

	fmt.Printf("pando daemon: socket %s, %d worktree(s)\n", g.socket, len(wts))
	return srv.Serve(ctx, g.socket)
}

// discoverWorktrees lists git worktrees, falling back to the current directory
// as a single implicit worktree when not inside a git repo.
func discoverWorktrees(ctx context.Context) ([]worktree.Worktree, error) {
	mgr := worktree.NewManager()
	wts, err := mgr.List(ctx)
	if err != nil || len(wts) == 0 {
		cwd, _ := os.Getwd()
		slug := worktree.Slug("main", cwd)
		return []worktree.Worktree{{Path: cwd, Branch: "main", Slug: slug}}, nil
	}
	return wts, nil
}
