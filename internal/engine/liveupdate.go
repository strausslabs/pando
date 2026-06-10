package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/guyStrauss/pando/internal/logbuf"
	"github.com/guyStrauss/pando/internal/resource"
	"github.com/guyStrauss/pando/internal/scheduler"
)

// runLiveUpdate applies a resource's liveUpdate steps in order for a batch of
// changed files. Steps:
//   - sync: copy local → container (compose) or no-op (host process).
//   - run:  execute a command inside the resource; skipped unless one of the
//     changed files matches a trigger glob (no trigger ⇒ always run).
//   - restart: re-run the resource (and its dependents) via the scheduler.
//
// A failing step aborts the pipeline and is logged to the resource; a full
// restart still happens only if a restart step is reached. Returns the first
// error so callers can surface it.
func (e *Engine) runLiveUpdate(ctx context.Context, as *activeStack, r *resource.Resource, changed []string) error {
	for _, step := range r.LiveUpdate {
		switch {
		case step.Sync != nil:
			if err := e.liveSync(ctx, as, r, step.Sync); err != nil {
				e.liveLog(as.env.Worktree, r.Name, "live-update sync failed: %v", err)
				return err
			}
		case step.Run != "":
			if !triggered(step.Trigger, changed, as.info.Path) {
				continue
			}
			if err := e.liveRun(ctx, as, r, step.Run); err != nil {
				e.liveLog(as.env.Worktree, r.Name, "live-update run %q failed: %v", step.Run, err)
				return err
			}
		case step.Restart:
			if err := as.sched.UpSubset(ctx, r.Name); err != nil {
				e.liveLog(as.env.Worktree, r.Name, "live-update restart failed: %v", err)
				return err
			}
		}
	}
	return nil
}

func (e *Engine) liveSync(ctx context.Context, as *activeStack, r *resource.Resource, s *resource.SyncRule) error {
	exec, ok := e.cfg.Executors[r.Kind]
	if !ok {
		return fmt.Errorf("no executor for kind %q", r.Kind)
	}
	syncer, ok := exec.(scheduler.Syncer)
	if !ok {
		return nil // executor cannot sync (host process); files already on host.
	}
	local := s.Local
	if !filepath.IsAbs(local) {
		local = filepath.Join(as.info.Path, local)
	}
	return syncer.Sync(ctx, r, as.env, local, s.Container)
}

func (e *Engine) liveRun(ctx context.Context, as *activeStack, r *resource.Resource, cmd string) error {
	execer, ok := e.cfg.Execers[r.Kind]
	if !ok {
		return fmt.Errorf("no execer for kind %q", r.Kind)
	}
	res, err := execer.Exec(ctx, as.env.Worktree, r.Name, []string{"sh", "-c", cmd}, as.env)
	if err != nil {
		return err
	}
	if res.Stdout != "" {
		e.liveLog(as.env.Worktree, r.Name, "%s", strings.TrimRight(res.Stdout, "\n"))
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("exit %d: %s", res.ExitCode, strings.TrimRight(res.Stderr, "\n"))
	}
	return nil
}

// triggered reports whether any changed file matches one of the trigger globs.
// An empty trigger list means the run step always fires. Globs are matched
// against the path relative to the worktree root and against the base name, so
// "requirements.txt" or "src/**/*.go"-style patterns both work.
func triggered(triggers, changed []string, root string) bool {
	if len(triggers) == 0 {
		return true
	}
	for _, c := range changed {
		rel := c
		if r, err := filepath.Rel(root, c); err == nil {
			rel = r
		}
		for _, t := range triggers {
			if matchGlob(t, rel) || matchGlob(t, filepath.Base(c)) {
				return true
			}
		}
	}
	return false
}

// matchGlob supports a leading "**/" to match at any depth, plus standard
// filepath.Match semantics for the remainder.
func matchGlob(pattern, path string) bool {
	if rest, ok := strings.CutPrefix(pattern, "**/"); ok {
		if ok, _ := filepath.Match(rest, path); ok {
			return true
		}
		if ok, _ := filepath.Match(rest, filepath.Base(path)); ok {
			return true
		}
		return strings.HasSuffix(path, strings.TrimPrefix(rest, "*"))
	}
	ok, _ := filepath.Match(pattern, path)
	return ok
}

func (e *Engine) liveLog(worktree, name, format string, args ...any) {
	if e.cfg.Logs == nil {
		return
	}
	e.cfg.Logs.Append(worktree, name, logbuf.System, fmt.Sprintf(format, args...),
		func() logbuf.Line { return logbuf.Line{Time: e.cfg.Clock()} })
}
