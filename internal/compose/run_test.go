package compose

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/strausslabs/pando/internal/resource"
	"github.com/strausslabs/pando/internal/scheduler"
)

type fakeDockerCli struct {
	client.APIClient
	startErr   error
	removed    []string
	restarted  []string
	restartErr error
}

func (f *fakeDockerCli) ImageInspectWithRaw(context.Context, string) (types.ImageInspect, []byte, error) {
	return types.ImageInspect{}, nil, nil
}
func (f *fakeDockerCli) ContainerCreate(context.Context, *container.Config, *container.HostConfig, *network.NetworkingConfig, *ocispec.Platform, string) (container.CreateResponse, error) {
	return container.CreateResponse{ID: "cid"}, nil
}
func (f *fakeDockerCli) ContainerStart(context.Context, string, container.StartOptions) error {
	return f.startErr
}
func (f *fakeDockerCli) ContainerStop(context.Context, string, container.StopOptions) error {
	return nil
}
func (f *fakeDockerCli) ContainerRemove(_ context.Context, name string, _ container.RemoveOptions) error {
	f.removed = append(f.removed, name)
	return nil
}
func (f *fakeDockerCli) ContainerRestart(_ context.Context, name string, _ container.StopOptions) error {
	f.restarted = append(f.restarted, name)
	return f.restartErr
}

func TestRunRemovesContainerWhenStartFails(t *testing.T) {
	fake := &fakeDockerCli{startErr: fmt.Errorf("oom")}
	b := &Backend{cli: fake, sink: &fakeSink{}, clock: func() time.Time { return time.Unix(1, 0) }}
	r := &resource.Resource{Name: "api", Kind: resource.KindCompose, Compose: &resource.ComposeSpec{Image: "alpine"}}
	env := scheduler.Env{Worktree: "main", Project: "pando-main", Ports: map[string]int{}, Vars: map[string]string{}}

	err := b.run(context.Background(), r, env, nopReporter{})
	if err == nil {
		t.Fatal("run should fail when ContainerStart errors")
	}
	want := containerName(env.Project, r.Name)
	got := 0
	for _, n := range fake.removed {
		if n == want {
			got++
		}
	}
	if got < 2 {
		t.Errorf("expected cleanup removal after failed start; removed %v", fake.removed)
	}
}

func TestRestartContainerBouncesInPlace(t *testing.T) {
	fake := &fakeDockerCli{}
	b := &Backend{cli: fake, sink: &fakeSink{}, clock: func() time.Time { return time.Unix(1, 0) }}
	r := &resource.Resource{Name: "api", Kind: resource.KindCompose, Compose: &resource.ComposeSpec{Image: "alpine"}}
	env := scheduler.Env{Worktree: "main", Project: "pando-main", Ports: map[string]int{}, Vars: map[string]string{}}

	if err := b.RestartContainer(context.Background(), r, env); err != nil {
		t.Fatalf("RestartContainer: %v", err)
	}

	want := containerName(env.Project, r.Name)
	if len(fake.restarted) != 1 || fake.restarted[0] != want {
		t.Errorf("RestartContainer should restart the same container in place, got %v", fake.restarted)
	}
	if len(fake.removed) != 0 {
		t.Errorf("RestartContainer must NOT remove/recreate the container (would drop synced files), removed %v", fake.removed)
	}
}

func TestRestartContainerPropagatesError(t *testing.T) {
	fake := &fakeDockerCli{restartErr: fmt.Errorf("no such container")}
	b := &Backend{cli: fake, sink: &fakeSink{}, clock: func() time.Time { return time.Unix(1, 0) }}
	r := &resource.Resource{Name: "api", Kind: resource.KindCompose, Compose: &resource.ComposeSpec{Image: "alpine"}}
	env := scheduler.Env{Worktree: "main", Project: "pando-main"}

	if err := b.RestartContainer(context.Background(), r, env); err == nil {
		t.Fatal("RestartContainer should surface a docker restart error")
	}
}
