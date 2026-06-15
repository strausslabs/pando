package engine

import (
	"context"
	"io/fs"
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

func onChangeDirs(r *resource.Resource, root string) []string {
	if r.DefaultRunPolicy() != resource.RunOnChange {
		return nil
	}
	seen := map[string]struct{}{}
	for _, pattern := range r.OnChange {
		base, glob := splitGlobBase(pattern)
		start := filepath.Join(root, base)
		if glob == "" {
			seen[watchDir(start)] = struct{}{}
			continue
		}
		_ = filepath.WalkDir(start, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if name := d.Name(); name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			seen[path] = struct{}{}
			return nil
		})
	}
	dirs := make([]string, 0, len(seen))
	for d := range seen {
		dirs = append(dirs, d)
	}
	return dirs
}

func (e *Engine) startWatchers(as *activeStack) {
	as.stopWatchers()

	actions := map[string][]func(context.Context, []string){}
	watch := func(dir string, fn func(context.Context, []string)) {
		actions[dir] = append(actions[dir], fn)
	}
	for _, r := range as.stack.Resources {
		r := r
		for _, p := range liveUpdatePaths(r, as.info.Path) {
			watch(watchDir(p), func(ctx context.Context, paths []string) {
				_ = e.runLiveUpdate(ctx, as, r, paths)
			})
		}
		for _, dir := range onChangeDirs(r, as.info.Path) {
			watch(dir, func(ctx context.Context, _ []string) {
				_ = as.sched.UpSubset(ctx, r.Name)
			})
		}
	}
	if len(actions) == 0 {
		return
	}

	lw := &liveWatcher{}
	w, err := watcher.New(300*time.Millisecond, func(key string, paths []string) {
		fns := actions[key]
		if len(fns) == 0 {
			return
		}
		if len(paths) == 0 {
			paths = []string{key}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		for _, fn := range fns {
			fn(ctx, paths)
		}
	})
	if err != nil {
		e.liveLog(as.env.Worktree, "", "watcher failed: %v", err)
		return
	}
	for dir := range actions {
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

func (as *activeStack) stopWatchers() {
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
