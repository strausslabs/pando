package engine

import (
	"context"
	"testing"

	"github.com/guyStrauss/pando/internal/api"
	"github.com/guyStrauss/pando/internal/resource"
	"github.com/guyStrauss/pando/internal/scheduler"
)

func stackWith(rs ...*resource.Resource) *resource.Stack {
	return &resource.Stack{Name: "pando", Resources: rs}
}

func localR(name, cmd string, deps ...string) *resource.Resource {
	return &resource.Resource{Name: name, Kind: resource.KindLocal, Local: &resource.LocalSpec{Cmd: cmd}, Deps: deps}
}

func TestReloadRerunsOnlyChanged(t *testing.T) {
	eng, logs, _ := testEngine(t)
	first := stackWith(
		localR("a", "echo A-v1; sleep 30"),
		localR("b", "echo B-v1; sleep 30", "a"),
	)
	if err := eng.Register(wt(), first); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := eng.Up(ctx, "main", false); err != nil {
		t.Fatal(err)
	}
	defer eng.Down(ctx, "main")

	// Wait for a's startup output to flush before snapshotting, so a delayed
	// first-run log is not mistaken for a re-run.
	if !waitForLine(logs, "main", "a", "A-v1") {
		t.Fatal("resource a never produced startup output")
	}
	if !waitForLine(logs, "main", "b", "B-v1") {
		t.Fatal("resource b never produced startup output")
	}
	aBefore := countLines(logs, "main", "a", "A-v1")

	// Change only b's command; a must not re-run.
	next := stackWith(
		localR("a", "echo A-v1; sleep 30"),
		localR("b", "echo B-v2; sleep 30", "a"),
	)
	if err := eng.Reload(ctx, "main", next); err != nil {
		t.Fatal(err)
	}

	if !waitForLine(logs, "main", "b", "B-v2") {
		t.Error("changed resource b did not re-run with new command")
	}
	if got := countLines(logs, "main", "a", "A-v1"); got != aBefore {
		t.Errorf("unchanged resource a re-ran on reload: %d -> %d", aBefore, got)
	}
}

func TestReloadStopsRemovedResource(t *testing.T) {
	eng, _, _ := testEngine(t)
	first := stackWith(
		localR("a", "sleep 30"),
		localR("b", "sleep 30"),
	)
	_ = eng.Register(wt(), first)
	ctx := context.Background()
	_ = eng.Up(ctx, "main", false)
	defer eng.Down(ctx, "main")

	next := stackWith(localR("a", "sleep 30"))
	if err := eng.Reload(ctx, "main", next); err != nil {
		t.Fatal(err)
	}
	st, _ := eng.Status(ctx)
	for _, r := range st[0].Resources {
		if r.Name == "b" {
			t.Error("removed resource b still present after reload")
		}
	}
}

func TestReloadAddsNewResource(t *testing.T) {
	eng, _, _ := testEngine(t)
	_ = eng.Register(wt(), stackWith(localR("a", "sleep 30")))
	ctx := context.Background()
	_ = eng.Up(ctx, "main", false)
	defer eng.Down(ctx, "main")

	next := stackWith(localR("a", "sleep 30"), localR("c", "echo C-new; sleep 30"))
	if err := eng.Reload(ctx, "main", next); err != nil {
		t.Fatal(err)
	}
	st, _ := eng.Status(ctx)
	found := false
	for _, r := range st[0].Resources {
		if r.Name == "c" {
			found = true
			if r.Phase != string(scheduler.PhaseHealthy) && r.Phase != string(scheduler.PhaseRunning) {
				t.Errorf("new resource c should be running, got %q", r.Phase)
			}
		}
	}
	if !found {
		t.Error("new resource c not registered after reload")
	}
}

func TestReloadNoChangeIsNoop(t *testing.T) {
	eng, logs, _ := testEngine(t)
	s := stackWith(localR("a", "echo A; sleep 30"))
	_ = eng.Register(wt(), s)
	ctx := context.Background()
	_ = eng.Up(ctx, "main", false)
	defer eng.Down(ctx, "main")
	before := countLines(logs, "main", "a", "A")

	same := stackWith(localR("a", "echo A; sleep 30"))
	if err := eng.Reload(ctx, "main", same); err != nil {
		t.Fatal(err)
	}
	if after := countLines(logs, "main", "a", "A"); after != before {
		t.Errorf("identical reload should not re-run: %d -> %d", before, after)
	}
}

func TestDeregisterStopsAndForgets(t *testing.T) {
	eng, _, _ := testEngine(t)
	_ = eng.Register(wt(), stackWith(localR("a", "sleep 30")))
	ctx := context.Background()
	_ = eng.Up(ctx, "main", false)
	if !eng.Registered("main") {
		t.Fatal("should be registered")
	}
	if err := eng.Deregister(ctx, "main"); err != nil {
		t.Fatal(err)
	}
	if eng.Registered("main") {
		t.Error("should be forgotten after deregister")
	}
	st, _ := eng.Status(ctx)
	if len(st) != 0 {
		t.Errorf("status should be empty after deregister, got %d", len(st))
	}
}

func TestReloadUnknownWorktreeErrors(t *testing.T) {
	eng, _, _ := testEngine(t)
	if err := eng.Reload(context.Background(), "ghost", stackWith(localR("a", "x"))); err == nil {
		t.Error("reload of unknown worktree should error")
	}
}

func TestReloadKeepsSharedHoistedAndOutOfWorktree(t *testing.T) {
	eng, _, _ := testEngine(t)
	if err := eng.Register(wt(), sharedStack()); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_ = eng.Up(ctx, "main", false)
	defer eng.Down(ctx, "main")
	defer eng.Down(ctx, sharedSlug)

	// A hot-reload feeds the FULL stack back in (shared auth + local api) — the
	// same shape the loader produced at Register. Reload must partition shared
	// the same way Register does: auth stays in the shared stack, never the
	// worktree.
	next := &resource.Stack{Name: "pando", Resources: []*resource.Resource{
		{Name: "auth", Kind: resource.KindTask, Task: &resource.TaskSpec{Cmd: "echo authed"}, Shared: true, RunWhen: resource.RunOnce},
		{Name: "api", Kind: resource.KindLocal, Local: &resource.LocalSpec{Cmd: "echo api-v2; sleep 30"}, Deps: []string{"auth"}},
	}}
	if err := eng.Reload(ctx, "main", next); err != nil {
		t.Fatalf("reload: %v", err)
	}

	st, _ := eng.Status(ctx)
	if findResource(st, "main", "auth") != nil {
		t.Error("shared 'auth' leaked into the worktree after reload")
	}
	if findResource(st, sharedSlug, "auth") == nil {
		t.Error("shared 'auth' missing from the shared stack after reload")
	}
}

func TestConfigErrorSurfacesForUnregisteredWorktree(t *testing.T) {
	eng, logs, _ := testEngine(t)
	ctx := context.Background()

	eng.ReportConfigError("feat-x", "feat/x", "syntax error: unexpected }")

	st, _ := eng.Status(ctx)
	var ws *api.WorktreeStatus
	for i := range st {
		if st[i].Worktree == "feat-x" {
			ws = &st[i]
		}
	}
	if ws == nil {
		t.Fatal("faulted worktree missing from status")
	}
	if ws.Branch != "feat/x" {
		t.Errorf("branch = %q, want feat/x", ws.Branch)
	}
	if ws.Error == "" {
		t.Error("config error not surfaced on worktree status")
	}
	if !waitForLine(logs, "feat-x", configResource, "syntax error") {
		t.Error("config error not streamed to the log")
	}
}

func TestConfigErrorClearedOnRecovery(t *testing.T) {
	eng, _, _ := testEngine(t)
	ctx := context.Background()
	eng.ReportConfigError("feat-x", "feat/x", "boom")
	eng.ClearConfigError("feat-x")

	st, _ := eng.Status(ctx)
	for _, ws := range st {
		if ws.Worktree == "feat-x" {
			t.Error("cleared config fault should not appear in status")
		}
	}
}

func TestConfigErrorOnRegisteredWorktreeKeepsResources(t *testing.T) {
	eng, _, _ := testEngine(t)
	_ = eng.Register(wt(), stackWith(localR("api", "sleep 30")))
	ctx := context.Background()
	_ = eng.Up(ctx, "main", false)
	defer eng.Down(ctx, "main")

	eng.ReportConfigError("main", "main", "reload failed: bad port")

	st, _ := eng.Status(ctx)
	main := st[0]
	if main.Error == "" {
		t.Error("registered worktree should carry the config error")
	}
	if len(main.Resources) == 0 {
		t.Error("registered worktree keeps its running resources despite a config fault")
	}
}

func TestReloadAddingSharedDepValidates(t *testing.T) {
	eng, _, _ := testEngine(t)
	_ = eng.Register(wt(), stackWith(localR("api", "sleep 30")))
	ctx := context.Background()
	_ = eng.Up(ctx, "main", false)
	defer eng.Down(ctx, "main")
	defer eng.Down(ctx, sharedSlug)

	// Editing the config to add a shared resource and a dep on it must not fail
	// validation: the shared name is external to the worktree graph.
	next := &resource.Stack{Name: "pando", Resources: []*resource.Resource{
		{Name: "auth", Kind: resource.KindTask, Task: &resource.TaskSpec{Cmd: "echo authed"}, Shared: true, RunWhen: resource.RunOnce},
		{Name: "api", Kind: resource.KindLocal, Local: &resource.LocalSpec{Cmd: "sleep 30"}, Deps: []string{"auth"}},
	}}
	if err := eng.Reload(ctx, "main", next); err != nil {
		t.Fatalf("reload adding a shared dep should succeed, got: %v", err)
	}
	st, _ := eng.Status(ctx)
	if findResource(st, sharedSlug, "auth") == nil {
		t.Error("newly-added shared 'auth' not merged into the shared stack on reload")
	}
}
