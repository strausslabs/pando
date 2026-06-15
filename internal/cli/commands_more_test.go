package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guyStrauss/pando/internal/api"
	"github.com/guyStrauss/pando/internal/client"
	"github.com/guyStrauss/pando/internal/daemon"
	"github.com/guyStrauss/pando/internal/logbuf"
	"github.com/guyStrauss/pando/internal/selfupdate"
)

type errOps struct{ stubOps }

func (*errOps) Status(context.Context) ([]api.WorktreeStatus, error) {
	return nil, context.DeadlineExceeded
}
func (*errOps) Logs(context.Context, api.LogQuery) ([]api.LogLine, error) {
	return nil, context.DeadlineExceeded
}
func (*errOps) Exec(context.Context, api.ExecRequest) (api.ExecResult, error) {
	return api.ExecResult{}, context.DeadlineExceeded
}
func (*errOps) Up(context.Context, string, bool) error { return context.DeadlineExceeded }
func (*errOps) Down(context.Context, string) error     { return context.DeadlineExceeded }
func (*errOps) Restart(context.Context, string, string) error {
	return context.DeadlineExceeded
}
func (*errOps) ListWorktrees(context.Context) ([]api.WorktreeInfo, error) {
	return nil, context.DeadlineExceeded
}

func runCmd(c interface {
	SetArgs([]string)
	Execute() error
}, args ...string) error {
	c.SetArgs(args)
	return c.Execute()
}

func TestExecCmdRoutesAndPrints(t *testing.T) {
	ops := &stubOps{}
	g := &globalFlags{socket: liveDaemon(t, ops)}
	out := captureStdout(t, func() {
		if err := runCmd(execCmd(g), "--worktree", "main", "api", "echo", "hi"); err != nil {
			t.Errorf("exec: %v", err)
		}
	})
	if !strings.Contains(out, "out") {
		t.Errorf("exec stdout not printed: %q", out)
	}
	ops.mu.Lock()
	defer ops.mu.Unlock()
	if ops.lastExec.Resource != "api" || strings.Join(ops.lastExec.Cmd, " ") != "echo hi" {
		t.Errorf("exec request not routed: %+v", ops.lastExec)
	}
}

func TestStatusReportsAvailableUpdate(t *testing.T) {
	g := &globalFlags{socket: updateDaemon(t)}
	out := captureStderr(t, func() {
		if err := runCmd(statusCmd(g)); err != nil {
			t.Errorf("status: %v", err)
		}
	})
	if !strings.Contains(out, "newer pando is available") {
		t.Errorf("update notice missing from stderr: %q", out)
	}
}

func TestCommandsSurfaceDaemonErrors(t *testing.T) {
	g := &globalFlags{socket: liveDaemon(t, &errOps{})}
	cases := []struct {
		name string
		cmd  interface {
			SetArgs([]string)
			Execute() error
		}
		args []string
	}{
		{"up", upCmd(g), []string{"--worktree", "main"}},
		{"down", downCmd(g), []string{"--worktree", "main"}},
		{"restart", restartCmd(g), []string{"--worktree", "main", "api"}},
		{"status", statusCmd(g), nil},
		{"logs", logsCmd(g), []string{"--worktree", "main", "api"}},
		{"exec", execCmd(g), []string{"--worktree", "main", "api", "true"}},
		{"worktrees", worktreesCmd(g), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := runCmd(tc.cmd, tc.args...); err == nil {
				t.Error("expected daemon error to propagate")
			}
		})
	}
}

func TestJSONOutputPaths(t *testing.T) {
	g := &globalFlags{socket: liveDaemon(t, &stubOps{}), json: true}
	cases := []struct {
		name string
		cmd  interface {
			SetArgs([]string)
			Execute() error
		}
		args []string
		want string
	}{
		{"logs", logsCmd(g), []string{"--worktree", "main", "api"}, `"text": "hello"`},
		{"exec", execCmd(g), []string{"--worktree", "main", "api", "echo"}, `"stdout": "out"`},
		{"worktrees", worktreesCmd(g), nil, `"slug": "main"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := captureStdout(t, func() {
				if err := runCmd(tc.cmd, tc.args...); err != nil {
					t.Errorf("%s --json: %v", tc.name, err)
				}
			})
			if !strings.Contains(out, tc.want) {
				t.Errorf("%s --json missing %q:\n%s", tc.name, tc.want, out)
			}
		})
	}
}

func TestNewClientNoDaemon(t *testing.T) {
	runtimeDir, err := os.MkdirTemp("", "nc-rt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDir) })
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Chdir(t.TempDir())

	if _, err := newClient(&globalFlags{}); err == nil {
		t.Error("newClient should error when no daemon is recorded")
	}
}

func TestEnsureClientNotInRepo(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := ensureClient(&globalFlags{}); err == nil {
		t.Error("ensureClient should error outside a git repository")
	}
}

func TestEnsureClientReusesRunningDaemon(t *testing.T) {
	g := &globalFlags{socket: liveDaemon(t, &stubOps{})}
	cl, err := ensureClient(g)
	if err != nil || cl == nil {
		t.Fatalf("ensureClient with explicit socket: cl=%v err=%v", cl, err)
	}
}

func updateDaemon(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "cd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")
	srv := daemon.NewServer(&stubOps{}, logbuf.NewStore(100))
	srv.SetUpdate(selfupdate.Status{Current: "v1.0.0", Latest: "v1.1.0", Available: true})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Serve(ctx, sock) }()
	t.Cleanup(cancel)

	cl := client.New(sock)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		hctx, hcancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		err := cl.Health(hctx)
		hcancel()
		if err == nil {
			return sock
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("update daemon never came up")
	return ""
}
