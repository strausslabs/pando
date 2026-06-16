package executor

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/strausslabs/pando/internal/api"
	"github.com/strausslabs/pando/internal/interp"
	"github.com/strausslabs/pando/internal/logbuf"
	"github.com/strausslabs/pando/internal/resource"
	"github.com/strausslabs/pando/internal/scheduler"
)

type Clock func() time.Time

type Sink interface {
	Append(worktree, resource string, stream logbuf.Stream, text string, mk func() logbuf.Line)
}

func scopeFromEnv(env scheduler.Env) interp.Scope {
	return interp.Scope{Ports: env.Ports, Vars: env.Vars}
}

func command(ctx context.Context, raw, cwd, baseDir string, extraEnv map[string]string, sc interp.Scope) (*exec.Cmd, error) {
	line, err := sc.Shell(raw)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", line)
	dir := baseDir
	if cwd != "" {
		resolved, err := sc.Shell(cwd)
		if err != nil {
			return nil, err
		}
		if filepath.IsAbs(resolved) || baseDir == "" {
			dir = resolved
		} else {
			dir = filepath.Join(baseDir, resolved)
		}
	}
	cmd.Dir = dir
	envv := os.Environ()
	for k, v := range extraEnv {
		val, err := sc.Shell(v)
		if err != nil {
			return nil, err
		}
		envv = append(envv, k+"="+val)
	}
	cmd.Env = envv
	// Setpgid: stopOne signals the whole group via Kill(-pid, ...).
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd, nil
}

func (e *Engine) pipeOutput(wt, name string, stream logbuf.Stream, r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		text := sc.Text()
		e.sink.Append(wt, name, stream, text, e.mkLine)
	}
}

func (e *Engine) mkLine() logbuf.Line {
	return logbuf.Line{Time: e.clock()}
}

func (e *Engine) system(wt, name, format string, args ...any) {
	e.sink.Append(wt, name, logbuf.System, fmt.Sprintf(format, args...), e.mkLine)
}

type Engine struct {
	sink  Sink
	clock Clock

	mu      sync.Mutex
	running map[string]*managed
}

type managed struct {
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	done     chan struct{}
	stopping atomic.Bool
}

func NewEngine(sink Sink, clock Clock) *Engine {
	if clock == nil {
		clock = time.Now
	}
	return &Engine{sink: sink, clock: clock, running: map[string]*managed{}}
}

func key(wt, name string) string { return wt + "\x00" + name }

func (e *Engine) Start(ctx context.Context, r *resource.Resource, env scheduler.Env, rep scheduler.Reporter) error {
	switch r.Kind {
	case resource.KindTask:
		return e.startTask(ctx, r, env, rep)
	case resource.KindLocal:
		return e.startLocal(ctx, r, env, rep)
	default:
		return fmt.Errorf("process engine cannot run kind %q", r.Kind)
	}
}

func (e *Engine) startTask(ctx context.Context, r *resource.Resource, env scheduler.Env, rep scheduler.Reporter) error {
	cmd, err := command(ctx, r.Task.Cmd, r.Task.Cwd, env.Dir, r.Task.Env, scopeFromEnv(env))
	if err != nil {
		return err
	}
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	rep.Phase(scheduler.PhaseRunning)
	if err := cmd.Start(); err != nil {
		return err
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); e.pipeOutput(env.Worktree, r.Name, logbuf.Stdout, stdout) }()
	go func() { defer wg.Done(); e.pipeOutput(env.Worktree, r.Name, logbuf.Stderr, stderr) }()
	wg.Wait()
	if err := cmd.Wait(); err != nil {
		e.system(env.Worktree, r.Name, "task exited with error: %v", err)
		return fmt.Errorf("task %q failed: %w", r.Name, err)
	}
	return nil
}

func (e *Engine) startLocal(ctx context.Context, r *resource.Resource, env scheduler.Env, rep scheduler.Reporter) error {
	e.stopOne(env.Worktree, r.Name)

	procCtx, cancel := context.WithCancel(context.Background())
	cmd, err := command(procCtx, r.Local.Cmd, r.Local.Cwd, env.Dir, r.Local.Env, scopeFromEnv(env))
	if err != nil {
		cancel()
		return err
	}
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		cancel()
		return err
	}

	m := &managed{cmd: cmd, cancel: cancel, done: make(chan struct{})}
	e.mu.Lock()
	e.running[key(env.Worktree, r.Name)] = m
	e.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); e.pipeOutput(env.Worktree, r.Name, logbuf.Stdout, stdout) }()
	go func() { defer wg.Done(); e.pipeOutput(env.Worktree, r.Name, logbuf.Stderr, stderr) }()

	go func() {
		wg.Wait()
		err := cmd.Wait()
		close(m.done)
		e.mu.Lock()
		delete(e.running, key(env.Worktree, r.Name))
		e.mu.Unlock()
		if err != nil && !m.stopping.Load() {
			e.system(env.Worktree, r.Name, "process exited unexpectedly: %v", err)
			rep.Phase(scheduler.PhaseFailed)
		}
	}()

	rep.Phase(scheduler.PhaseRunning)
	return nil
}

func (e *Engine) Stop(ctx context.Context, r *resource.Resource, env scheduler.Env) error {
	e.stopOne(env.Worktree, r.Name)
	return nil
}

func (e *Engine) Exec(ctx context.Context, worktree, name string, argv []string, env scheduler.Env) (api.ExecResult, error) {
	if len(argv) == 0 {
		return api.ExecResult{}, fmt.Errorf("exec: empty command")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	envv := os.Environ()
	for k, v := range env.Vars {
		envv = append(envv, k+"="+v)
	}
	cmd.Env = envv
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := api.ExecResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
			return res, nil
		}
		return res, err
	}
	return res, nil
}

func (e *Engine) stopOne(wt, name string) {
	e.mu.Lock()
	m, ok := e.running[key(wt, name)]
	if ok {
		delete(e.running, key(wt, name))
	}
	e.mu.Unlock()
	if !ok {
		return
	}
	m.stopping.Store(true)
	if m.cmd.Process != nil {
		_ = syscall.Kill(-m.cmd.Process.Pid, syscall.SIGTERM)
	}
	select {
	case <-m.done:
	case <-time.After(5 * time.Second):
		if m.cmd.Process != nil {
			_ = syscall.Kill(-m.cmd.Process.Pid, syscall.SIGKILL)
		}
		<-m.done
	}
	m.cancel()
}
