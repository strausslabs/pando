package engine

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/guyStrauss/pando/internal/resource"
	"github.com/guyStrauss/pando/internal/watcher"
)

// liveWatcher watches a worktree's source paths and runs each affected
// resource's liveUpdate pipeline on change. One per active stack; started on Up,
// stopped on Down/Reload.
type liveWatcher struct {
	w      *watcher.Watcher
	cancel context.CancelFunc
}

// liveUpdatePaths returns, for a resource, the directories to watch: each
// liveUpdate sync source plus any local.watch entries, resolved against the
// worktree root.
func liveUpdatePaths(r *resource.Resource, root string) []string {
	var paths []string
	add := func(p string) {
		if p == "" {
			return
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, p)
		}
		paths = append(paths, p)
	}
	for _, step := range r.LiveUpdate {
		if step.Sync != nil {
			add(step.Sync.Local)
		}
	}
	if r.Local != nil {
		for _, w := range r.Local.Watch {
			add(w)
		}
	}
	return paths
}

// startLiveUpdate begins watching for a stack's live-update resources. A no-op
// when none declare liveUpdate. Replaces any existing watcher for the stack.
func (e *Engine) startLiveUpdate(as *activeStack) {
	as.stopLiveUpdate()

	// Map each watched directory to the resource that owns it.
	owner := map[string]*resource.Resource{}
	for _, r := range as.stack.Resources {
		if len(r.LiveUpdate) == 0 {
			continue
		}
		for _, p := range liveUpdatePaths(r, as.info.Path) {
			owner[watchDir(p)] = r
		}
	}
	if len(owner) == 0 {
		return
	}

	lw := &liveWatcher{}
	w, err := watcher.New(300*time.Millisecond, func(key string, paths []string) {
		r, ok := owner[key]
		if !ok {
			return
		}
		// Fall back to the watched dir if fsnotify gave no concrete path (rare).
		if len(paths) == 0 {
			paths = []string{key}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_ = e.runLiveUpdate(ctx, as, r, paths)
	})
	if err != nil {
		e.liveLog(as.env.Worktree, "", "live-update watcher failed: %v", err)
		return
	}
	for dir := range owner {
		_ = w.Add(dir, dir)
	}
	lw.w = w

	ctx, cancel := context.WithCancel(context.Background())
	lw.cancel = cancel
	go w.Run(ctx)

	as.mu.Lock()
	as.live = lw
	as.mu.Unlock()
}

func (as *activeStack) stopLiveUpdate() {
	as.mu.Lock()
	lw := as.live
	as.live = nil
	as.mu.Unlock()
	if lw != nil && lw.cancel != nil {
		lw.cancel()
	}
}

// watchDir returns the directory to register with the fsnotify watcher for a
// path: the path itself if it is a directory, else its parent (fsnotify watches
// directories, and the matcher resolves events to the parent key).
func watchDir(p string) string {
	if fi, err := os.Stat(p); err == nil && fi.IsDir() {
		return p
	}
	return filepath.Dir(p)
}
