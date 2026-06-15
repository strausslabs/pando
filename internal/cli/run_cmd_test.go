package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/strausslabs/pando/internal/api"
	"github.com/strausslabs/pando/internal/daemon"
	"github.com/strausslabs/pando/internal/logbuf"
)

type stubOps struct {
	mu       sync.Mutex
	ups      []string
	downs    []string
	restarts [][2]string
	lastExec api.ExecRequest
}

func (s *stubOps) Status(context.Context) ([]api.WorktreeStatus, error) {
	return []api.WorktreeStatus{{Worktree: "main", Resources: []api.ResourceStatus{
		{Name: "api", Kind: "compose", Phase: "healthy", Port: 8080},
	}}}, nil
}
func (s *stubOps) Logs(context.Context, api.LogQuery) ([]api.LogLine, error) {
	return []api.LogLine{{Text: "hello", Resource: "api"}}, nil
}
func (s *stubOps) Exec(_ context.Context, req api.ExecRequest) (api.ExecResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastExec = req
	return api.ExecResult{Stdout: "out", ExitCode: 0}, nil
}
func (s *stubOps) Up(_ context.Context, wt string, _ bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ups = append(s.ups, wt)
	return nil
}
func (s *stubOps) Down(_ context.Context, wt string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.downs = append(s.downs, wt)
	return nil
}
func (s *stubOps) Restart(_ context.Context, wt, res string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restarts = append(s.restarts, [2]string{wt, res})
	return nil
}
func (s *stubOps) Rebuild(context.Context, string, string) error { return nil }
func (s *stubOps) Trigger(context.Context, string, string) error { return nil }
func (s *stubOps) ListWorktrees(context.Context) ([]api.WorktreeInfo, error) {
	return []api.WorktreeInfo{{Slug: "main", Branch: "main", Path: "/repo", Ports: map[string]int{"api": 8080}}}, nil
}

func liveDaemon(t *testing.T, ops api.StackOps) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "cd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")
	srv := daemon.NewServer(ops, logbuf.NewStore(100))
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Serve(ctx, sock) }()
	t.Cleanup(cancel)

	if err := waitForSocket(context.Background(), sock, true, 3*time.Second); err != nil {
		t.Fatalf("daemon never came up: %v", err)
	}
	return sock
}

func TestCommandsAgainstLiveDaemon(t *testing.T) {
	ops := &stubOps{}
	sock := liveDaemon(t, ops)
	g := &globalFlags{socket: sock}

	if err := runCmd(upCmd(g), "--worktree", "main"); err != nil {
		t.Errorf("up: %v", err)
	}
	if err := runCmd(downCmd(g), "--worktree", "main"); err != nil {
		t.Errorf("down: %v", err)
	}
	if err := runCmd(restartCmd(g), "--worktree", "main", "api"); err != nil {
		t.Errorf("restart: %v", err)
	}
	out := captureStdout(t, func() {
		if err := runCmd(statusCmd(g)); err != nil {
			t.Errorf("status: %v", err)
		}
	})
	if !strings.Contains(out, "api") {
		t.Errorf("status output missing resource: %s", out)
	}
	out = captureStdout(t, func() {
		if err := runCmd(logsCmd(g), "--worktree", "main", "api"); err != nil {
			t.Errorf("logs: %v", err)
		}
	})
	if !strings.Contains(out, "hello") {
		t.Errorf("logs output missing line: %s", out)
	}
	out = captureStdout(t, func() {
		if err := runCmd(worktreesCmd(g)); err != nil {
			t.Errorf("worktrees: %v", err)
		}
	})
	if !strings.Contains(out, "main") {
		t.Errorf("worktrees output missing slug: %s", out)
	}

	ops.mu.Lock()
	defer ops.mu.Unlock()
	if len(ops.ups) != 1 || ops.ups[0] != "main" {
		t.Errorf("up not routed: %v", ops.ups)
	}
	if len(ops.downs) != 1 {
		t.Errorf("down not routed: %v", ops.downs)
	}
	if len(ops.restarts) != 1 || ops.restarts[0] != [2]string{"main", "api"} {
		t.Errorf("restart not routed: %v", ops.restarts)
	}
}

func TestJSONOutput(t *testing.T) {
	ops := &stubOps{}
	sock := liveDaemon(t, ops)
	g := &globalFlags{socket: sock, json: true}
	out := captureStdout(t, func() {
		c := statusCmd(g)
		c.SetArgs(nil)
		if err := c.Execute(); err != nil {
			t.Errorf("status --json: %v", err)
		}
	})
	if !strings.Contains(out, "\"worktree\": \"main\"") {
		t.Errorf("json status missing field:\n%s", out)
	}
}
