package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/strausslabs/pando/internal/api"
	"github.com/strausslabs/pando/internal/resource"
	"github.com/strausslabs/pando/internal/scheduler"
)

func liveStack(t *testing.T) (*Engine, *activeStack, *resource.Resource) {
	t.Helper()
	eng, _, _ := testEngine(t)
	r := &resource.Resource{Name: "api", Kind: resource.KindLocal, Local: &resource.LocalSpec{Cmd: "true"}}
	as := &activeStack{
		info: api.WorktreeInfo{Path: t.TempDir(), Slug: "main"},
		env:  scheduler.Env{Worktree: "main", Project: "pando"},
	}
	return eng, as, r
}

func TestLiveRunReportsExitCode(t *testing.T) {
	eng, as, r := liveStack(t)
	if err := eng.liveRun(context.Background(), as, r, "exit 3"); err == nil {
		t.Error("liveRun should return error on non-zero exit")
	}
}

func TestLiveLocalRunRunsOnHostInWorktree(t *testing.T) {
	eng, logs, _ := testEngine(t)
	dir := t.TempDir()
	r := &resource.Resource{Name: "api", Kind: resource.KindCompose}
	as := &activeStack{
		info: api.WorktreeInfo{Path: dir, Slug: "main"},
		env:  scheduler.Env{Worktree: "main", Project: "pando", Ports: map[string]int{"api": 8123}},
	}

	if err := eng.liveLocalRun(context.Background(), as, r, "pwd; echo port=$PORT_api"); err != nil {
		t.Fatalf("liveLocalRun: %v", err)
	}

	lines := strings.Join(logs.Text("main", "api"), "\n")
	if !strings.Contains(lines, dir) {
		t.Errorf("local_run should run in the worktree dir %q:\n%s", dir, lines)
	}
	if !strings.Contains(lines, "port=8123") {
		t.Errorf("local_run should expand $PORT_api:\n%s", lines)
	}
}

func TestLiveLocalRunReportsExitCode(t *testing.T) {
	eng, as, r := liveStack(t)
	if err := eng.liveLocalRun(context.Background(), as, r, "exit 4"); err == nil {
		t.Error("liveLocalRun should return error on non-zero exit")
	}
}

func TestLiveSyncNoSyncerIsNoop(t *testing.T) {
	eng, as, r := liveStack(t)
	if err := eng.liveSync(context.Background(), as, r, &resource.SyncRule{Local: "x", Container: "/y"}); err != nil {
		t.Errorf("liveSync with a non-Syncer executor should no-op, got %v", err)
	}
}

func TestLiveSyncUnknownKind(t *testing.T) {
	eng, as, _ := liveStack(t)
	r := &resource.Resource{Name: "api", Kind: resource.KindCompose}
	if err := eng.liveSync(context.Background(), as, r, &resource.SyncRule{Local: "x", Container: "/y"}); err == nil {
		t.Error("liveSync should error when no executor is registered for the kind")
	}
}

type fakeRestarter struct {
	scheduler.Executor
	restarted int
}

func (f *fakeRestarter) RestartContainer(context.Context, *resource.Resource, scheduler.Env) error {
	f.restarted++
	return nil
}

func TestLiveRestartUsesRestarterForCompose(t *testing.T) {
	eng, _, _ := testEngine(t)
	fr := &fakeRestarter{}
	eng.cfg.Executors[resource.KindCompose] = fr
	r := &resource.Resource{Name: "api", Kind: resource.KindCompose}
	as := &activeStack{
		info: api.WorktreeInfo{Path: t.TempDir(), Slug: "main"},
		env:  scheduler.Env{Worktree: "main", Project: "pando"},
	}

	if err := eng.liveRestart(context.Background(), as, r); err != nil {
		t.Fatalf("liveRestart: %v", err)
	}
	if fr.restarted != 1 {
		t.Errorf("compose live-update restart should bounce the container in place via RestartContainer, calls=%d", fr.restarted)
	}
}
