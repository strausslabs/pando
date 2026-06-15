package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/strausslabs/pando/internal/interp"
	"github.com/strausslabs/pando/internal/logbuf"
	"github.com/strausslabs/pando/internal/resource"
	"github.com/strausslabs/pando/internal/scheduler"
)

func (e *Engine) runLiveUpdate(ctx context.Context, as *activeStack, r *resource.Resource, changed []string) error {
	lock := as.liveLock(r.Name)
	lock.Lock()
	defer lock.Unlock()

	if e.cfg.Logs != nil {
		e.cfg.Logs.PublishPhase(as.env.Worktree, r.Name, string(scheduler.PhaseLiveUpdating))
	}
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
		case step.LocalRun != "":
			if !triggered(step.Trigger, changed, as.info.Path) {
				continue
			}
			if err := e.liveLocalRun(ctx, as, r, step.LocalRun); err != nil {
				e.liveLog(as.env.Worktree, r.Name, "live-update local_run %q failed: %v", step.LocalRun, err)
				return err
			}
		case step.RestartContainer:
			if err := e.liveRestart(ctx, as, r); err != nil {
				e.liveLog(as.env.Worktree, r.Name, "live-update restart failed: %v", err)
				return err
			}
		}
	}
	return nil
}

func (as *activeStack) liveLock(name string) *sync.Mutex {
	as.mu.Lock()
	defer as.mu.Unlock()
	if as.liveRunning == nil {
		as.liveRunning = map[string]*sync.Mutex{}
	}
	if as.liveRunning[name] == nil {
		as.liveRunning[name] = &sync.Mutex{}
	}
	return as.liveRunning[name]
}

func (e *Engine) liveSync(ctx context.Context, as *activeStack, r *resource.Resource, s *resource.SyncRule) error {
	exec, ok := e.cfg.Executors[r.Kind]
	if !ok {
		return fmt.Errorf("no executor for kind %q", r.Kind)
	}
	syncer, ok := exec.(scheduler.Syncer)
	if !ok {
		return nil
	}
	local := s.Local
	if !filepath.IsAbs(local) {
		local = filepath.Join(as.info.Path, local)
	}
	return syncer.Sync(ctx, r, as.env, local, s.Container)
}

func (e *Engine) liveRestart(ctx context.Context, as *activeStack, r *resource.Resource) error {
	if exec, ok := e.cfg.Executors[r.Kind]; ok {
		if restarter, ok := exec.(scheduler.Restarter); ok {
			return restarter.RestartContainer(ctx, r, as.env)
		}
	}
	return as.sched.UpSubset(ctx, r.Name)
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
	if res.Stderr != "" {
		e.liveLogStream(as.env.Worktree, r.Name, logbuf.Stderr, strings.TrimRight(res.Stderr, "\n"))
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("exit %d", res.ExitCode)
	}
	return nil
}

func (e *Engine) liveLocalRun(ctx context.Context, as *activeStack, r *resource.Resource, raw string) error {
	sc := interp.Scope{Ports: as.env.Ports, Vars: as.env.Vars}
	line, err := sc.Shell(raw)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", line)
	cmd.Dir = as.info.Path
	cmd.Env = os.Environ()
	for k, v := range as.env.Vars {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	if text := strings.TrimRight(string(out), "\n"); text != "" {
		e.liveLog(as.env.Worktree, r.Name, "%s", text)
	}
	return err
}

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
			t = strings.TrimPrefix(t, "./")
			if matchGlob(t, rel) || matchGlob(t, filepath.Base(c)) || underDir(t, rel) {
				return true
			}
		}
	}
	return false
}

func underDir(dir, path string) bool {
	if strings.ContainsAny(dir, "*?[") {
		return false
	}
	dir = strings.TrimSuffix(dir, "/")
	return path == dir || strings.HasPrefix(path, dir+"/")
}

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
	e.liveLogStream(worktree, name, logbuf.System, fmt.Sprintf(format, args...))
}

func (e *Engine) liveLogStream(worktree, name string, stream logbuf.Stream, text string) {
	e.logAppend(worktree, name, stream, text)
}
