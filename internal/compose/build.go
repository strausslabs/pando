package compose

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/guyStrauss/pando/internal/logbuf"
	"github.com/guyStrauss/pando/internal/resource"
	"github.com/guyStrauss/pando/internal/scheduler"
)

func (b *Backend) build(ctx context.Context, r *resource.Resource, env scheduler.Env) error {
	if b.docker == "" {
		return fmt.Errorf("docker CLI not found on PATH (required to build images)")
	}
	args, err := buildArgs(r, env)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, b.docker, args...)
	cmd.Env = buildEnv()
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{}, 2)
	go func() { b.pipe(env.Worktree, r.Name, logbuf.Stdout, stdout); done <- struct{}{} }()
	go func() { b.pipe(env.Worktree, r.Name, logbuf.Stderr, stderr); done <- struct{}{} }()
	<-done
	<-done
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("build %s: %w", r.Name, err)
	}
	return nil
}

func buildArgs(r *resource.Resource, env scheduler.Env) ([]string, error) {
	sc := scopeOf(env)
	args := []string{"build", "-t", imageTag(env.Project, r.Name)}

	if r.Build.Dockerfile != "" {
		args = append(args, "-f", r.Build.Dockerfile)
	}
	if r.Build.Target != "" {
		args = append(args, "--target", r.Build.Target)
	}
	for _, k := range sortedKeys(r.Build.Args) {
		val, err := sc.String(r.Build.Args[k])
		if err != nil {
			return nil, err
		}
		args = append(args, "--build-arg", k+"="+val)
	}
	for _, s := range r.Build.Secrets {
		args = append(args, "--secret", fmt.Sprintf("id=%s,src=%s", s.ID, expandHome(s.Src)))
	}
	ctxDir, err := sc.String(r.Build.Context)
	if err != nil {
		return nil, err
	}
	args = append(args, ctxDir)
	return args, nil
}

func (b *Backend) pipe(worktree, res string, stream logbuf.Stream, r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		b.sink.Append(worktree, res, stream, sc.Text(), func() logbuf.Line {
			return logbuf.Line{Time: b.clock()}
		})
	}
}
