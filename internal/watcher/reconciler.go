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

type Engine interface {
	Register(wt worktree.Worktree, stack *resource.Stack) error
	Reload(ctx context.Context, slug string, next *resource.Stack) error
	Deregister(ctx context.Context, slug string) error
	Registered(slug string) bool
	ReportConfigError(slug, branch, msg string)
	ClearConfigError(slug string)
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

type Reconciler struct {
	eng    Engine
	loader ConfigLoader
	lister WorktreeLister
	gitDir string
	opts   Options

	w *Watcher

	mu       sync.Mutex
	tracked  map[string]worktree.Worktree
	configOf map[string]string
}

func NewReconciler(eng Engine, loader ConfigLoader, lister WorktreeLister, gitCommonDir string, opts Options) (*Reconciler, error) {
	if opts.ConfigName == "" {
		opts.ConfigName = "pando.star"
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
		_ = r.w.Add(filepath.Join(r.gitDir, "worktrees"), gitWorktreesKey)
	}

	go func() { _ = r.w.Run(ctx) }()

	r.reconcileWorktrees(ctx)

	// Slow poll backs up fsnotify so a dropped event never leaves a worktree unmanaged.
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

func (r *Reconciler) onFire(key string, _ []string) {
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
		r.eng.ReportConfigError(wt.Slug, wt.Branch, err.Error())
		r.opts.OnError(fmt.Errorf("worktree %q config: %w", wt.Slug, err))
		// Watch the dir anyway so a later fix is picked up.
		_ = r.w.Add(filepath.Dir(cfg), "cfg:"+wt.Slug)
		r.mu.Lock()
		r.tracked[wt.Slug] = wt
		r.configOf["cfg:"+wt.Slug] = wt.Slug
		r.mu.Unlock()
		return
	}
	if err := r.eng.Register(wt, stack); err != nil {
		r.eng.ReportConfigError(wt.Slug, wt.Branch, err.Error())
		r.opts.OnError(fmt.Errorf("register %q: %w", wt.Slug, err))
		return
	}
	r.eng.ClearConfigError(wt.Slug)

	key := "cfg:" + wt.Slug
	r.mu.Lock()
	r.tracked[wt.Slug] = wt
	r.configOf[key] = wt.Slug
	r.mu.Unlock()
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
		r.eng.ReportConfigError(slug, wt.Branch, err.Error())
		r.opts.OnError(fmt.Errorf("reload %q config: %w", slug, err))
		return
	}
	if !r.eng.Registered(slug) {
		if err := r.eng.Register(wt, stack); err != nil {
			r.eng.ReportConfigError(slug, wt.Branch, err.Error())
			r.opts.OnError(err)
			return
		}
		r.eng.ClearConfigError(slug)
		return
	}
	if err := r.eng.Reload(ctx, slug, stack); err != nil {
		r.eng.ReportConfigError(slug, wt.Branch, err.Error())
		r.opts.OnError(fmt.Errorf("reload %q: %w", slug, err))
		return
	}
	r.eng.ClearConfigError(slug)
}
