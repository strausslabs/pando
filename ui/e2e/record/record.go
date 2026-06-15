//go:build ignore

// Records live daemon responses into ui/e2e/fixtures so the Playwright suite
// replays real wire data instead of hand-written mocks. Run from repo root:
//
//	go run ./ui/e2e/record
//
// It boots a real engine over an in-memory host-process stack (echo/sleep — no
// docker), drives one worktree up, then snapshots each endpoint and a few live
// /events frames.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/guyStrauss/pando/internal/api"
	"github.com/guyStrauss/pando/internal/client"
	"github.com/guyStrauss/pando/internal/daemon"
	"github.com/guyStrauss/pando/internal/engine"
	"github.com/guyStrauss/pando/internal/executor"
	"github.com/guyStrauss/pando/internal/logbuf"
	"github.com/guyStrauss/pando/internal/resource"
	"github.com/guyStrauss/pando/internal/scheduler"
	"github.com/guyStrauss/pando/internal/selfupdate"
	"github.com/guyStrauss/pando/internal/worktree"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "record:", err)
		os.Exit(1)
	}
}

func run() error {
	outDir := "ui/e2e/fixtures"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	logs := logbuf.NewStore(1000)
	proc := executor.NewEngine(logs, time.Now)
	eng := engine.New(engine.Config{
		StackName: "pando",
		Allocator: worktree.DefaultAllocator(),
		Logs:      logs,
		Executors: map[resource.Kind]scheduler.Executor{resource.KindTask: proc, resource.KindLocal: proc},
		Execers:   map[resource.Kind]engine.Execer{resource.KindTask: proc, resource.KindLocal: proc},
	})

	mainStack := &resource.Stack{Name: "pando", Resources: []*resource.Resource{
		{Name: "setup", Kind: resource.KindTask, Task: &resource.TaskSpec{Cmd: "echo setting-up"}, RunWhen: resource.RunOnce},
		{Name: "api", Kind: resource.KindLocal, Local: &resource.LocalSpec{Cmd: "echo api listening on 8080; sleep 30"}, Deps: []string{"setup"}},
		{Name: "sync", Kind: resource.KindTask, Task: &resource.TaskSpec{Cmd: "echo periodic tick"}, Every: 30 * time.Second},
	}}
	if err := eng.Register(worktree.Worktree{Path: "/repo", Branch: "main", Head: "abc", Slug: "main"}, mainStack); err != nil {
		return err
	}
	featStack := &resource.Stack{Name: "pando", Resources: []*resource.Resource{
		{Name: "api", Kind: resource.KindLocal, Local: &resource.LocalSpec{Cmd: "echo feat api up; sleep 30"}},
	}}
	if err := eng.Register(worktree.Worktree{Path: "/repo-feat", Branch: "feat/login", Head: "def", Slug: "feat-login"}, featStack); err != nil {
		return err
	}

	srv := daemon.NewServer(eng, logs)
	srv.SetUpdate(selfupdate.Status{Current: "v1.0.0", Latest: "v1.0.0", Available: false})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sock := filepath.Join(os.TempDir(), "pando-record.sock")
	_ = os.Remove(sock)
	go func() { _ = srv.Serve(ctx, sock) }()

	cl := client.New(sock)
	if err := waitHealthy(cl); err != nil {
		return err
	}
	if err := eng.Up(ctx, "main", false); err != nil {
		return err
	}
	if err := eng.Up(ctx, "feat-login", false); err != nil {
		return err
	}
	defer func() {
		shut, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		eng.Shutdown(shut)
	}()

	// Let the local process emit its startup log before snapshotting.
	time.Sleep(1500 * time.Millisecond)

	status, err := cl.Status(ctx)
	if err != nil {
		return err
	}
	wts, err := cl.ListWorktrees(ctx)
	if err != nil {
		return err
	}
	ver, err := cl.Version(ctx)
	if err != nil {
		return err
	}
	apiLogs, err := cl.Logs(ctx, api.LogQuery{Worktree: "main", Resource: "api", Tail: 200})
	if err != nil {
		return err
	}
	featLogs, err := cl.Logs(ctx, api.LogQuery{Worktree: "feat-login", Resource: "api", Tail: 200})
	if err != nil {
		return err
	}

	events, err := captureEvents(ctx, sock, func() { _ = eng.Restart(ctx, "main", "api") })
	if err != nil {
		return err
	}

	for name, v := range map[string]any{
		"status.json":    status,
		"worktrees.json": wts,
		"version.json":   ver,
		"logs.json":      apiLogs,
		"logs-feat.json": featLogs,
		"events.json":    events,
	} {
		if err := writeJSON(filepath.Join(outDir, name), v); err != nil {
			return err
		}
		fmt.Println("wrote", filepath.Join(outDir, name))
	}
	return nil
}

func waitHealthy(cl *client.Client) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		err := cl.Health(ctx)
		cancel()
		if err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("daemon never became healthy")
}

func captureEvents(parent context.Context, sock string, trigger func()) ([]json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(parent, 6*time.Second)
	defer cancel()
	c, resp, err := websocket.Dial(ctx, "ws://unix/events", &websocket.DialOptions{
		HTTPClient: unixHTTPClient(sock),
	})
	if err != nil {
		return nil, err
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	defer func() { _ = c.CloseNow() }()

	go func() {
		time.Sleep(300 * time.Millisecond)
		trigger()
	}()

	var out []json.RawMessage
	for len(out) < 3 {
		var raw json.RawMessage
		if err := wsjson.Read(ctx, c, &raw); err != nil {
			break
		}
		out = append(out, raw)
	}
	return out, nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func unixHTTPClient(sock string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", sock)
			},
		},
	}
}
