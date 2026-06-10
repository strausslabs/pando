package watcher

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/guyStrauss/pando/internal/resource"
	"github.com/guyStrauss/pando/internal/worktree"
)

const gitWorktreesKey = "__git_worktrees__"

// Engine is the subset of engine.Engine the reconciler drives. Defined here as
// an interface to keep the watcher decoupled and unit-testable with a fake.
type Engine interface {
	Register(wt worktree.Worktree, stack *resource.Stack) error
	Reload(ctx context.Context, slug string, next *resource.Stack) error
	Deregister(ctx context.Context, slug string) error
	Registered(slug string) bool
}

type ConfigLoader interface {
	LoadFile(ctx context.Context, path string) (*resource.Stack, error)
}

type WorktreeLister interface {
	List(ctx context.Context) ([]worktree.Worktree, error)
}

type Options struct {
	ConfigName string
	AutoUp     bool
	OnUp       func(ctx context.Context, slug string)
	OnError    func(err error)
	PollEvery  time.Duration
}

// Reconciler keeps the engine's registered worktrees and their configs in sync
// with the filesystem. It watches .git/worktrees for branch worktrees coming
// and going, and each worktree's config file for hot-reload. A slow poll backs
// up fsnotify so changes are never missed even if an event is dropped.
type Reconciler struct {
	eng    Engine
	loader ConfigLoader
	lister WorktreeLister
	gitDir string
	opts   Options

	w *Watcher

	mu       sync.Mutex
	tracked  map[string]worktree.Worktree // slug -> worktree
	configOf map[string]string            // config-key -> slug
}

func NewReconciler(eng Engine, loader ConfigLoader, lister WorktreeLister, gitCommonDir string, opts Options) (*Reconciler, error) {
	if opts.ConfigName == "" {
		opts.ConfigName = "pando.config.ts"
	}
	if opts.PollEvery == 0 {
		opts.PollEvery = 2 * time.Second
	}
	if opts.OnError == nil {
		opts.OnError = func(error) {}
	}
	r := &Reconciler{
		eng:      eng,
		loader:   loader,
		lister:   lister,
		gitDir:   gitCommonDir,
		opts:     opts,
		tracked:  map[string]worktree.Worktree{},
		configOf: map[string]string{},
	}
	w, err := New(100*time.Millisecond, r.onFire)
	if err != nil {
		return nil, err
	}
	r.w = w
	return r, nil
}

func (r *Reconciler) Run(ctx context.Context) error {
	if r.gitDir != "" {
		// .git/worktrees holds one dir per linked worktree; writes here signal
		// add/remove. Best-effort: absence is fine for single-worktree repos.
		_ = r.w.Add(filepath.Join(r.gitDir, "worktrees"), gitWorktreesKey)
	}

	go r.w.Run(ctx)

	r.reconcileWorktrees(ctx)

	ticker := time.NewTicker(r.opts.PollEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			r.reconcileWorktrees(ctx)
		}
	}
}

func (r *Reconciler) onFire(key string) {
	ctx := context.Background()
	if key == gitWorktreesKey {
		r.reconcileWorktrees(ctx)
		return
	}
	r.mu.Lock()
	slug, ok := r.configOf[key]
	r.mu.Unlock()
	if ok {
		r.reloadConfig(ctx, slug)
	}
}

// reconcileWorktrees diffs the live `git worktree list` against what is
// tracked: new worktrees are registered (and watched), vanished ones are
// deregistered and unwatched.
func (r *Reconciler) reconcileWorktrees(ctx context.Context) {
	wts, err := r.lister.List(ctx)
	if err != nil {
		r.opts.OnError(fmt.Errorf("list worktrees: %w", err))
		return
	}
	live := make(map[string]worktree.Worktree, len(wts))
	for _, wt := range wts {
		live[wt.Slug] = wt
	}

	r.mu.Lock()
	tracked := make(map[string]worktree.Worktree, len(r.tracked))
	for k, v := range r.tracked {
		tracked[k] = v
	}
	r.mu.Unlock()

	for slug, wt := range live {
		if _, ok := tracked[slug]; !ok {
			r.addWorktree(ctx, wt)
		}
	}
	for slug, wt := range tracked {
		if _, ok := live[slug]; !ok {
			r.removeWorktree(ctx, slug, wt)
		}
	}
}

func (r *Reconciler) configPath(wt worktree.Worktree) string {
	return filepath.Join(wt.Path, r.opts.ConfigName)
}

func (r *Reconciler) addWorktree(ctx context.Context, wt worktree.Worktree) {
	cfg := r.configPath(wt)
	stack, err := r.loader.LoadFile(ctx, cfg)
	if err != nil {
		// A worktree without a (valid) config is simply not managed; surface it
		// but keep tracking so a later fix is picked up.
		r.opts.OnError(fmt.Errorf("worktree %q config: %w", wt.Slug, err))
		return
	}
	if err := r.eng.Register(wt, stack); err != nil {
		r.opts.OnError(fmt.Errorf("register %q: %w", wt.Slug, err))
		return
	}

	key := "cfg:" + wt.Slug
	r.mu.Lock()
	r.tracked[wt.Slug] = wt
	r.configOf[key] = wt.Slug
	r.mu.Unlock()
	// Watch the config's directory; editors often replace the file (rename),
	// which only surfaces as a directory-level event.
	_ = r.w.Add(filepath.Dir(cfg), key)

	if r.opts.AutoUp && r.opts.OnUp != nil {
		r.opts.OnUp(ctx, wt.Slug)
	}
}

func (r *Reconciler) removeWorktree(ctx context.Context, slug string, wt worktree.Worktree) {
	if err := r.eng.Deregister(ctx, slug); err != nil {
		r.opts.OnError(fmt.Errorf("deregister %q: %w", slug, err))
	}
	key := "cfg:" + slug
	r.w.Remove(filepath.Dir(r.configPath(wt)))
	r.mu.Lock()
	delete(r.tracked, slug)
	delete(r.configOf, key)
	r.mu.Unlock()
}

func (r *Reconciler) reloadConfig(ctx context.Context, slug string) {
	r.mu.Lock()
	wt, ok := r.tracked[slug]
	r.mu.Unlock()
	if !ok {
		return
	}
	stack, err := r.loader.LoadFile(ctx, r.configPath(wt))
	if err != nil {
		// Keep the running stack as-is on a broken edit rather than tearing it
		// down; report so the user can fix the config.
		r.opts.OnError(fmt.Errorf("reload %q config: %w", slug, err))
		return
	}
	if !r.eng.Registered(slug) {
		if err := r.eng.Register(wt, stack); err != nil {
			r.opts.OnError(err)
		}
		return
	}
	if err := r.eng.Reload(ctx, slug, stack); err != nil {
		r.opts.OnError(fmt.Errorf("reload %q: %w", slug, err))
	}
}
