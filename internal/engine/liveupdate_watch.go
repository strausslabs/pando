package engine

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/guyStrauss/pando/internal/resource"
	"github.com/guyStrauss/pando/internal/watcher"
)

type liveWatcher struct {
	w      *watcher.Watcher
	cancel context.CancelFunc
}

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

func (e *Engine) startLiveUpdate(as *activeStack) {
	as.stopLiveUpdate()

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
	go func() { _ = w.Run(ctx) }()

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

func watchDir(p string) string {
	if fi, err := os.Stat(p); err == nil && fi.IsDir() {
		return p
	}
	return filepath.Dir(p)
}
