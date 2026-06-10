package compose

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"

	"github.com/guyStrauss/pando/internal/api"
	"github.com/guyStrauss/pando/internal/interp"
	"github.com/guyStrauss/pando/internal/logbuf"
	"github.com/guyStrauss/pando/internal/resource"
	"github.com/guyStrauss/pando/internal/scheduler"
)

// Backend runs Compose-kind resources through the Docker SDK for typed
// container lifecycle, log demuxing, and interactive exec. The image build step
// uses the `docker build` CLI (see build.go) because BuildKit secret mounts
// require a build session the classic SDK ImageBuild endpoint does not expose;
// the CLI build hits the same daemon-side BuildKit cache, so nothing is lost.
//
// Each worktree maps to one container per resource, named <project>-<resource>,
// labelled with pando.project so concurrent worktrees never collide.
type Backend struct {
	cli    client.APIClient
	docker string
	sink   Sink
	clock  func() time.Time
}

type Sink interface {
	Append(worktree, resource string, stream logbuf.Stream, text string, mk func() logbuf.Line)
}

func New(sink Sink, clock func() time.Time) (*Backend, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	if clock == nil {
		clock = time.Now
	}
	b := &Backend{cli: cli, sink: sink, clock: clock}
	b.docker = lookDocker()
	return b, nil
}

func containerName(project, res string) string { return project + "-" + res }
func imageTag(project, res string) string {
	return fmt.Sprintf("pando/%s/%s:dev", project, res)
}

func scopeOf(env scheduler.Env) interp.Scope {
	return interp.Scope{Ports: env.Ports, Vars: env.Vars}
}

func (b *Backend) Start(ctx context.Context, r *resource.Resource, env scheduler.Env, rep scheduler.Reporter) error {
	if r.Build != nil {
		rep.Phase(scheduler.PhaseStarting)
		if err := b.build(ctx, r, env); err != nil {
			return err
		}
	}
	return b.run(ctx, r, env, rep)
}

func (b *Backend) run(ctx context.Context, r *resource.Resource, env scheduler.Env, rep scheduler.Reporter) error {
	b.removeContainer(ctx, env.Project, r.Name)

	cfg, hostCfg, err := containerConfig(r, env)
	if err != nil {
		return err
	}
	name := containerName(env.Project, r.Name)
	created, err := b.cli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, name)
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	if err := b.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}
	rep.Phase(scheduler.PhaseRunning)
	go b.followLogs(context.WithoutCancel(ctx), env.Worktree, r.Name, created.ID)
	return nil
}

// containerConfig translates a resource into Docker container + host configs.
// Pure (no IO) so it is unit tested without a daemon. Ports and env are
// interpolated against the worktree scope.
func containerConfig(r *resource.Resource, env scheduler.Env) (*container.Config, *container.HostConfig, error) {
	sc := scopeOf(env)

	image := imageTag(env.Project, r.Name)
	if r.Build == nil && r.Compose != nil && r.Compose.Image != "" {
		image = r.Compose.Image
	}

	cfg := &container.Config{
		Image:  image,
		Labels: map[string]string{"pando.project": env.Project, "pando.resource": r.Name},
	}
	hostCfg := &container.HostConfig{}

	if r.Compose != nil {
		for _, k := range sortedKeys(r.Compose.Env) {
			val, err := sc.String(r.Compose.Env[k])
			if err != nil {
				return nil, nil, err
			}
			cfg.Env = append(cfg.Env, k+"="+val)
		}
		if len(r.Compose.Command) > 0 {
			cfg.Cmd = r.Compose.Command
		}
		hostCfg.Binds = append(hostCfg.Binds, r.Compose.Volumes...)

		specs := make([]string, 0, len(r.Compose.Ports))
		for _, p := range r.Compose.Ports {
			mapped, err := sc.String(p)
			if err != nil {
				return nil, nil, err
			}
			specs = append(specs, mapped)
		}
		exposed, bindings, err := nat.ParsePortSpecs(specs)
		if err != nil {
			return nil, nil, fmt.Errorf("ports: %w", err)
		}
		cfg.ExposedPorts = exposed
		hostCfg.PortBindings = bindings
	}
	return cfg, hostCfg, nil
}

func (b *Backend) Stop(ctx context.Context, r *resource.Resource, env scheduler.Env) error {
	b.removeContainer(ctx, env.Project, r.Name)
	return nil
}

func (b *Backend) removeContainer(ctx context.Context, project, res string) {
	name := containerName(project, res)
	rmCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	timeout := 10
	_ = b.cli.ContainerStop(rmCtx, name, container.StopOptions{Timeout: &timeout})
	_ = b.cli.ContainerRemove(rmCtx, name, container.RemoveOptions{Force: true})
}

// followLogs streams a container's stdout/stderr into the log store. Docker
// multiplexes both streams over one connection; stdcopy demuxes them so stderr
// lines are tagged correctly.
func (b *Backend) followLogs(ctx context.Context, worktree, res, containerID string) {
	rc, err := b.cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		// "all": the container is freshly created by Pando, so its entire log
		// history is ours to show; starting at 0 would race past the first
		// lines emitted before this follow attaches.
		Tail: "all",
	})
	if err != nil {
		return
	}
	defer rc.Close()
	_, _ = stdcopy.StdCopy(
		b.lineWriter(worktree, res, logbuf.Stdout),
		b.lineWriter(worktree, res, logbuf.Stderr),
		rc,
	)
}

func (b *Backend) Exec(ctx context.Context, worktree, name string, argv []string, env scheduler.Env) (api.ExecResult, error) {
	if len(argv) == 0 {
		return api.ExecResult{}, fmt.Errorf("exec: empty command")
	}
	cname := containerName(env.Project, name)
	created, err := b.cli.ContainerExecCreate(ctx, cname, container.ExecOptions{
		Cmd:          argv,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return api.ExecResult{}, err
	}
	att, err := b.cli.ContainerExecAttach(ctx, created.ID, container.ExecAttachOptions{})
	if err != nil {
		return api.ExecResult{}, err
	}
	defer att.Close()

	var stdout, stderr lineBuffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, att.Reader); err != nil {
		return api.ExecResult{}, err
	}
	inspect, err := b.cli.ContainerExecInspect(ctx, created.ID)
	if err != nil {
		return api.ExecResult{}, err
	}
	return api.ExecResult{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: inspect.ExitCode}, nil
}

func (b *Backend) lineWriter(worktree, res string, stream logbuf.Stream) io.Writer {
	return &logWriter{
		emit: func(text string) {
			b.sink.Append(worktree, res, stream, text, func() logbuf.Line {
				return logbuf.Line{Time: b.clock()}
			})
		},
	}
}
