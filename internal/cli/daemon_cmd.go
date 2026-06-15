package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/strausslabs/pando/internal/compose"
	"github.com/strausslabs/pando/internal/config"
	"github.com/strausslabs/pando/internal/daemon"
	"github.com/strausslabs/pando/internal/discovery"
	"github.com/strausslabs/pando/internal/engine"
	"github.com/strausslabs/pando/internal/executor"
	"github.com/strausslabs/pando/internal/logbuf"
	"github.com/strausslabs/pando/internal/resource"
	"github.com/strausslabs/pando/internal/scheduler"
	"github.com/strausslabs/pando/internal/selfupdate"
	"github.com/strausslabs/pando/internal/state"
	"github.com/strausslabs/pando/internal/watcher"
	"github.com/strausslabs/pando/internal/web"
	"github.com/strausslabs/pando/internal/worktree"
)

func daemonCmd(g *globalFlags, version string) *cobra.Command {
	var tcpAddr string
	var autoUp bool
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run the Pando daemon (low-level; prefer `pando start`)",
		RunE: func(c *cobra.Command, args []string) error {
			return runDaemonSignal(g, version, tcpAddr, autoUp)
		},
	}
	cmd.Flags().StringVar(&tcpAddr, "ui-addr", "auto", "loopback address for the web UI (\"auto\" = repo-derived port, empty to disable)")
	cmd.Flags().BoolVar(&autoUp, "auto-up", true, "bring every discovered worktree up automatically (--auto-up=false to disable)")
	return cmd
}

func startCmd(g *globalFlags, version string) *cobra.Command {
	var tcpAddr string
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start Pando: run the daemon and bring all worktrees up",
		RunE: func(c *cobra.Command, args []string) error {
			return runDaemonSignal(g, version, tcpAddr, true)
		},
	}
	cmd.Flags().StringVar(&tcpAddr, "ui-addr", "auto", "loopback address for the web UI (\"auto\" = repo-derived port, empty to disable)")
	return cmd
}

func stopCmd(g *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the Pando daemon for this repo",
		RunE: func(c *cobra.Command, args []string) error {
			return stopDaemon(ctx())
		},
	}
}

func runDaemonSignal(g *globalFlags, version, tcpAddr string, autoUp bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return runDaemon(ctx, g, version, tcpAddr, autoUp)
}

func stopDaemon(ctx context.Context) error {
	info, found, running := discovery.Resolve(ctx)
	if !found || !running {
		return fmt.Errorf("no running pando daemon for this repo")
	}
	if err := syscall.Kill(info.PID, syscall.SIGTERM); err != nil {
		return fmt.Errorf("stop daemon %d: %w", info.PID, err)
	}
	if err := waitForSocket(ctx, info.Socket, false, 10*time.Second); err != nil {
		return err
	}
	discovery.Remove(info.GitCommonDir)
	fmt.Printf("pando daemon stopped (pid %d)\n", info.PID)
	return nil
}

func runDaemon(ctx context.Context, g *globalFlags, version, tcpAddr string, autoUp bool) error {
	loader, err := config.NewLoader()
	if err != nil {
		return err
	}

	gitDir := discovery.GitCommonDir(ctx)
	socket := g.socket
	if socket == "" {
		if gitDir == "" {
			return fmt.Errorf("not inside a git repository; run pando from a repo or pass --socket")
		}
		socket = discovery.SocketPath(gitDir)
	}
	if err := discovery.EnsureDir(); err != nil {
		return err
	}

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

	if cb, err := compose.New(logs, time.Now); err != nil {
		fmt.Fprintf(os.Stderr, "compose disabled: %v\n", err)
	} else {
		execs[resource.KindCompose] = cb
		execers[resource.KindCompose] = cb
	}

	eng := engine.New(engine.Config{
		StackName: stack.Name,
		Allocator: worktree.DefaultAllocator(),
		Store:     store,
		Logs:      logs,
		Executors: execs,
		Execers:   execers,
	})

	if tcpAddr == "auto" {
		tcpAddr = ""
		if gitDir != "" {
			tcpAddr = fmt.Sprintf("127.0.0.1:%d", discovery.FreeUIPort(gitDir))
		}
	}
	if tcpAddr != "" {
		_ = os.Setenv("PANDO_UI_TARGET", "http://"+tcpAddr)
	}

	mgr := worktree.NewManager()
	rec, err := watcher.NewReconciler(eng, loader, mgr, gitDir, watcher.Options{
		ConfigName: filepath.Base(g.config),
		AutoUp:     autoUp,
		OnUp: func(upCtx context.Context, slug string) {
			if err := eng.Up(upCtx, slug, false); err != nil {
				fmt.Fprintf(os.Stderr, "auto-up %s: %v\n", slug, err)
			}
		},
		OnError: func(err error) { fmt.Fprintf(os.Stderr, "reconcile: %v\n", err) },
	})
	if err != nil {
		return err
	}
	var bg sync.WaitGroup
	bg.Add(1)
	go func() {
		defer bg.Done()
		if err := rec.Run(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "reconciler: %v\n", err)
		}
	}()

	srv := daemon.NewServer(eng, logs)
	if ui, ok := web.Handler(); ok {
		srv.MountUI(ui)
	}

	bg.Add(1)
	go func() {
		defer bg.Done()
		st := selfupdate.Check(ctx, version, filepath.Join(stateDir, "update.json"), time.Now())
		srv.SetUpdate(st)
		if st.Available {
			fmt.Fprintf(os.Stderr, updateAvailableMsg, st.Current, st.Latest)
		}
	}()

	if tcpAddr != "" {
		bg.Add(1)
		go func() {
			defer bg.Done()
			if err := srv.ServeTCP(ctx, tcpAddr); err != nil {
				fmt.Fprintf(os.Stderr, "ui server: %v\n", err)
			}
		}()
		fmt.Printf("pando ready → http://%s\n", tcpAddr)
	}

	if gitDir != "" {
		_ = discovery.Write(discovery.Info{
			Socket:       socket,
			PID:          os.Getpid(),
			UIAddr:       tcpAddr,
			GitCommonDir: gitDir,
			StartedUnix:  time.Now().Unix(),
		})
		defer discovery.Remove(gitDir)
	}

	fmt.Fprintf(os.Stderr, "watching for worktrees (socket %s)\n", socket)
	err = srv.Serve(ctx, socket)

	shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	eng.Shutdown(shutCtx)
	bg.Wait()
	return err
}
