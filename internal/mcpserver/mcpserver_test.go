package mcpserver

import (
	"context"
	"fmt"
	"testing"

	"github.com/guyStrauss/pando/internal/api"
	"github.com/guyStrauss/pando/internal/discovery"
)

type fakeDaemon struct {
	worktrees []api.WorktreeInfo
	status    []api.WorktreeStatus
	logs      []api.LogLine
	execResp  api.ExecResult
	ups       []string
	restarts  [][2]string
	err       error
}

func (f *fakeDaemon) Status(context.Context) ([]api.WorktreeStatus, error) {
	return f.status, f.err
}
func (f *fakeDaemon) ListWorktrees(context.Context) ([]api.WorktreeInfo, error) {
	return f.worktrees, f.err
}
func (f *fakeDaemon) Logs(context.Context, api.LogQuery) ([]api.LogLine, error) {
	return f.logs, f.err
}
func (f *fakeDaemon) Up(_ context.Context, wt string, _ bool) error {
	f.ups = append(f.ups, wt)
	return f.err
}
func (f *fakeDaemon) Down(context.Context, string) error { return f.err }
func (f *fakeDaemon) Restart(_ context.Context, wt, res string) error {
	f.restarts = append(f.restarts, [2]string{wt, res})
	return f.err
}
func (f *fakeDaemon) Exec(context.Context, api.ExecRequest) (api.ExecResult, error) {
	return f.execResp, f.err
}

func deps(d *fakeDaemon, found, running bool) Deps {
	return Deps{
		Resolve: func(context.Context) (discovery.Info, bool, bool) {
			return discovery.Info{Socket: "/x.sock"}, found, running
		},
		Dial: func(string) Daemon { return d },
	}
}

func line(text string) api.LogLine { return api.LogLine{Text: text} }

func TestRunningReportsState(t *testing.T) {
	ctx := context.Background()

	out := callRunning(t, deps(&fakeDaemon{}, true, true))
	if !out.Running {
		t.Error("should report running when found and socket alive")
	}

	out = callRunning(t, deps(&fakeDaemon{}, true, false))
	if out.Running {
		t.Error("stale socket must report not running")
	}

	out = callRunning(t, deps(&fakeDaemon{}, false, false))
	if out.Running {
		t.Error("absent daemon must report not running")
	}
	_ = ctx
}

func TestConnectGatesOnDaemonState(t *testing.T) {
	ctx := context.Background()

	// Not found → status tool returns an error result, not a panic.
	res, _, err := statusTool(deps(&fakeDaemon{}, false, false))(ctx, nil, struct{}{})
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Error("status with no daemon should be an error result")
	}

	// Stale → also an error result.
	res, _, _ = statusTool(deps(&fakeDaemon{}, true, false))(ctx, nil, struct{}{})
	if res == nil || !res.IsError {
		t.Error("status with stale daemon should be an error result")
	}
}

func TestStatusPassesThrough(t *testing.T) {
	d := &fakeDaemon{status: []api.WorktreeStatus{{Worktree: "main"}}}
	res, out, _ := statusTool(deps(d, true, true))(context.Background(), nil, struct{}{})
	if res != nil {
		t.Fatalf("unexpected error result: %+v", res)
	}
	if len(out.Worktrees) != 1 || out.Worktrees[0].Worktree != "main" {
		t.Errorf("status not passed through: %+v", out)
	}
}

func TestResolveWorktreeDefaultsToSingleton(t *testing.T) {
	d := &fakeDaemon{worktrees: []api.WorktreeInfo{{Slug: "only"}}}
	res, out, _ := upTool(deps(d, true, true))(context.Background(), nil, WorktreeIn{})
	if res != nil {
		t.Fatalf("unexpected error: %+v", res)
	}
	if out.Worktree != "only" {
		t.Errorf("should default to the single worktree, got %q", out.Worktree)
	}
	if len(d.ups) != 1 || d.ups[0] != "only" {
		t.Errorf("Up not called for the resolved worktree: %v", d.ups)
	}
}

func TestResolveWorktreeAmbiguousErrors(t *testing.T) {
	d := &fakeDaemon{worktrees: []api.WorktreeInfo{{Slug: "a"}, {Slug: "b"}}}
	res, _, _ := upTool(deps(d, true, true))(context.Background(), nil, WorktreeIn{})
	if res == nil || !res.IsError {
		t.Error("ambiguous worktree (>1, none given) should be an error result")
	}
	if len(d.ups) != 0 {
		t.Error("Up must not fire when the worktree is ambiguous")
	}
}

func TestExplicitWorktreeWins(t *testing.T) {
	d := &fakeDaemon{worktrees: []api.WorktreeInfo{{Slug: "a"}, {Slug: "b"}}}
	_, out, _ := restartTool(deps(d, true, true))(context.Background(), nil, RestartIn{Resource: "api", Worktree: "b"})
	if out.Worktree != "b" {
		t.Errorf("explicit worktree ignored, got %q", out.Worktree)
	}
	if len(d.restarts) != 1 || d.restarts[0] != [2]string{"b", "api"} {
		t.Errorf("restart routed wrong: %v", d.restarts)
	}
}

func TestLogsSearchRegexAndTail(t *testing.T) {
	d := &fakeDaemon{
		worktrees: []api.WorktreeInfo{{Slug: "main"}},
		logs: []api.LogLine{
			line("INFO start"), line("ERROR boom 1"), line("INFO tick"),
			line("ERROR boom 2"), line("ERROR boom 3"),
		},
	}
	res, out, _ := logsSearchTool(deps(d, true, true))(context.Background(), nil,
		LogsSearchIn{Resource: "api", Pattern: `ERROR boom \d`, Tail: 2})
	if res != nil {
		t.Fatalf("unexpected error: %+v", res)
	}
	if out.Matched != 3 {
		t.Errorf("matched count wrong: got %d, want 3", out.Matched)
	}
	if len(out.Lines) != 2 {
		t.Fatalf("tail not applied: got %d lines, want 2", len(out.Lines))
	}
	// Tail keeps the LAST matches.
	if out.Lines[0].Text != "ERROR boom 2" || out.Lines[1].Text != "ERROR boom 3" {
		t.Errorf("tail kept the wrong matches: %v", []string{out.Lines[0].Text, out.Lines[1].Text})
	}
}

func TestLogsSearchInvalidRegexErrors(t *testing.T) {
	d := &fakeDaemon{worktrees: []api.WorktreeInfo{{Slug: "main"}}}
	res, _, _ := logsSearchTool(deps(d, true, true))(context.Background(), nil,
		LogsSearchIn{Resource: "api", Pattern: "("})
	if res == nil || !res.IsError {
		t.Error("invalid regex should yield an error result")
	}
}

func TestExecEmptyCmdErrors(t *testing.T) {
	d := &fakeDaemon{worktrees: []api.WorktreeInfo{{Slug: "main"}}}
	res, _, _ := execTool(deps(d, true, true))(context.Background(), nil, ExecIn{Resource: "api"})
	if res == nil || !res.IsError {
		t.Error("empty cmd should error")
	}
}

func TestDaemonErrorBecomesToolError(t *testing.T) {
	d := &fakeDaemon{err: fmt.Errorf("connection refused")}
	res, _, _ := statusTool(deps(d, true, true))(context.Background(), nil, struct{}{})
	if res == nil || !res.IsError {
		t.Error("a daemon error should surface as a tool error result, not a hard failure")
	}
}

func callRunning(t *testing.T, d Deps) RunningOut {
	t.Helper()
	_, out, err := runningTool(d)(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("running tool hard error: %v", err)
	}
	return out
}
