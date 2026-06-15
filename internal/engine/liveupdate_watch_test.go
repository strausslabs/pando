package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/guyStrauss/pando/internal/api"
	"github.com/guyStrauss/pando/internal/resource"
	"github.com/guyStrauss/pando/internal/scheduler"
)

func TestLiveUpdatePaths(t *testing.T) {
	root := "/repo"
	r := &resource.Resource{
		LiveUpdate: []resource.LiveUpdateStep{
			{Sync: &resource.SyncRule{Local: "src", Container: "/app"}},
			{Sync: &resource.SyncRule{Local: "/abs/dir", Container: "/app2"}},
			{Run: "make"},
		},
		Local: &resource.LocalSpec{Watch: []string{"watched", ""}},
	}
	got := liveUpdatePaths(r, root)
	want := []string{"/repo/src", "/abs/dir", "/repo/watched"}
	if len(got) != len(want) {
		t.Fatalf("liveUpdatePaths = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("path[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWatchDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := watchDir(dir); got != dir {
		t.Errorf("watchDir(dir) = %q, want %q", got, dir)
	}
	if got := watchDir(file); got != dir {
		t.Errorf("watchDir(file) = %q, want its parent %q", got, dir)
	}
	missing := filepath.Join(dir, "nope", "x")
	if got := watchDir(missing); got != filepath.Dir(missing) {
		t.Errorf("watchDir(missing) = %q, want parent %q", got, filepath.Dir(missing))
	}
}

func TestStartLiveUpdateNoStepsIsNoop(t *testing.T) {
	eng, _, _ := testEngine(t)
	as := &activeStack{
		info:  api.WorktreeInfo{Path: t.TempDir(), Slug: "main"},
		env:   scheduler.Env{Worktree: "main"},
		stack: &resource.Stack{Resources: []*resource.Resource{{Name: "api", Kind: resource.KindLocal}}},
	}
	eng.startWatchers(as)
	as.mu.Lock()
	live := as.live
	as.mu.Unlock()
	if live != nil {
		t.Error("startLiveUpdate should not create a watcher when no resource has live-update steps")
	}
}

func TestStartLiveUpdateWatchesAndStops(t *testing.T) {
	eng, _, _ := testEngine(t)
	dir := t.TempDir()
	as := &activeStack{
		info: api.WorktreeInfo{Path: dir, Slug: "main"},
		env:  scheduler.Env{Worktree: "main"},
		stack: &resource.Stack{Resources: []*resource.Resource{{
			Name:       "api",
			Kind:       resource.KindLocal,
			Local:      &resource.LocalSpec{Cmd: "true", Watch: []string{"."}},
			LiveUpdate: []resource.LiveUpdateStep{{Run: "true"}},
		}}},
	}
	eng.startWatchers(as)
	as.mu.Lock()
	live := as.live
	as.mu.Unlock()
	if live == nil {
		t.Fatal("startLiveUpdate should create a watcher when a resource has live-update steps")
	}
	as.stopWatchers()
	as.mu.Lock()
	stopped := as.live
	as.mu.Unlock()
	if stopped != nil {
		t.Error("stopLiveUpdate should clear the live watcher")
	}
}
