package compose

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/strausslabs/pando/internal/resource"
	"github.com/strausslabs/pando/internal/scheduler"
)

func env() scheduler.Env {
	return scheduler.Env{
		Worktree: "feat-x",
		Project:  "pando-feat-x",
		Ports:    map[string]int{"api": 8042},
		Vars:     map[string]string{"ENV": "stg"},
	}
}

func TestContainerNaming(t *testing.T) {
	if got := containerName("pando-feat-x", "api"); got != "pando-feat-x-api" {
		t.Errorf("container name: %q", got)
	}
	if got := imageTag("pando-feat-x", "api"); got != "pando/pando-feat-x/api:dev" {
		t.Errorf("image tag: %q", got)
	}
}

func TestBuildArgsMultiStageAndSecrets(t *testing.T) {
	r := &resource.Resource{
		Name: "api", Kind: resource.KindCompose,
		Build: &resource.Build{
			Context:    "./api",
			Dockerfile: "Dockerfile",
			Target:     "prod",
			Args:       map[string]string{"AWS_PROFILE": "$ENV", "VERSION": "1.2"},
			Secrets:    []resource.BuildSecret{{ID: "zscaler_cert", Src: "~/zscaler.crt"}},
		},
	}
	args, err := buildArgs(r, env())
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--target prod") {
		t.Errorf("missing target: %v", args)
	}
	// Build args interpolate against scope and are emitted in sorted key order.
	if !strings.Contains(joined, "--build-arg AWS_PROFILE=stg") {
		t.Errorf("AWS_PROFILE not interpolated: %v", args)
	}
	if !strings.Contains(joined, "--build-arg VERSION=1.2") {
		t.Errorf("VERSION missing: %v", args)
	}
	if strings.Index(joined, "AWS_PROFILE") > strings.Index(joined, "VERSION") {
		t.Errorf("build args not in sorted order: %v", args)
	}
	if !strings.Contains(joined, "--secret id=zscaler_cert,src=") {
		t.Errorf("secret mount missing: %v", args)
	}
	if strings.Contains(joined, "~/") {
		t.Errorf("secret src not home-expanded: %v", args)
	}
	if args[len(args)-1] != "./api" {
		t.Errorf("context should be last arg: %v", args)
	}
}

func TestContainerConfigPortsAndEnv(t *testing.T) {
	r := &resource.Resource{
		Name: "api", Kind: resource.KindCompose,
		Compose: &resource.ComposeSpec{
			Ports: []string{"$PORT_api:8000"},
			Env:   map[string]string{"DB": "postgres", "MODE": "$ENV"},
		},
	}
	cfg, hostCfg, err := containerConfig(r, env())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Image != "pando/pando-feat-x/api:dev" {
		t.Errorf("image: %q", cfg.Image)
	}
	if cfg.Labels["pando.project"] != "pando-feat-x" {
		t.Errorf("missing project label: %v", cfg.Labels)
	}
	// Port spec interpolated: host 8042 -> container 8000.
	found := false
	for port, binds := range hostCfg.PortBindings {
		if port.Port() == "8000" {
			for _, bind := range binds {
				if bind.HostPort == "8042" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("port binding 8042->8000 not set: %v", hostCfg.PortBindings)
	}
	envHas := func(want string) bool {
		for _, e := range cfg.Env {
			if e == want {
				return true
			}
		}
		return false
	}
	if !envHas("DB=postgres") || !envHas("MODE=stg") {
		t.Errorf("env not set/interpolated: %v", cfg.Env)
	}
}

func TestContainerConfigUsesComposeImageWhenNoBuild(t *testing.T) {
	r := &resource.Resource{
		Name: "db", Kind: resource.KindCompose,
		Compose: &resource.ComposeSpec{Image: "postgres:16"},
	}
	cfg, _, err := containerConfig(r, env())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Image != "postgres:16" {
		t.Errorf("should use compose image: %q", cfg.Image)
	}
}

func TestContainerConfigSetsMemoryLimit(t *testing.T) {
	r := &resource.Resource{
		Name: "api", Kind: resource.KindCompose,
		Compose: &resource.ComposeSpec{Image: "postgres:16", Memory: 256 << 20},
	}
	_, hostCfg, err := containerConfig(r, env())
	if err != nil {
		t.Fatal(err)
	}
	if hostCfg.Memory != 256<<20 {
		t.Errorf("memory limit = %d, want %d", hostCfg.Memory, 256<<20)
	}
	if hostCfg.MemoryReservation != 256<<20 {
		t.Errorf("memory reservation = %d, want %d", hostCfg.MemoryReservation, 256<<20)
	}
}

func TestContainerConfigNoMemoryLimitByDefault(t *testing.T) {
	r := &resource.Resource{
		Name: "api", Kind: resource.KindCompose,
		Compose: &resource.ComposeSpec{Image: "postgres:16"},
	}
	_, hostCfg, err := containerConfig(r, env())
	if err != nil {
		t.Fatal(err)
	}
	if hostCfg.Memory != 0 {
		t.Errorf("unbounded by default, got memory = %d", hostCfg.Memory)
	}
}

func TestContainerConfigBadPortErrors(t *testing.T) {
	r := &resource.Resource{
		Name: "api", Kind: resource.KindCompose,
		Compose: &resource.ComposeSpec{Ports: []string{"$PORT_missing:8000"}},
	}
	if _, _, err := containerConfig(r, env()); err == nil {
		t.Error("undefined port should error")
	}
}

func TestLogWriterSplitsLines(t *testing.T) {
	var got []string
	w := &logWriter{emit: func(s string) { got = append(got, s) }}
	_, _ = w.Write([]byte("line one\nline two\npar"))
	_, _ = w.Write([]byte("tial done\n"))
	if len(got) != 3 {
		t.Fatalf("want 3 lines, got %d: %v", len(got), got)
	}
	if got[0] != "line one" || got[2] != "partial done" {
		t.Errorf("lines wrong: %v", got)
	}
}

func TestExpandHome(t *testing.T) {
	if !strings.HasPrefix(expandHome("~/x"), "/") {
		t.Error("~/ should expand to absolute path")
	}
	if expandHome("/abs/path") != "/abs/path" {
		t.Error("absolute path should be unchanged")
	}
	if expandHome("rel/path") != "rel/path" {
		t.Error("relative non-home path unchanged")
	}
}

func TestSortedKeys(t *testing.T) {
	got := sortedKeys(map[string]string{"c": "", "a": "", "b": ""})
	if strings.Join(got, "") != "abc" {
		t.Errorf("keys not sorted: %v", got)
	}
}

func TestResolveDockerHostDeferToEnv(t *testing.T) {
	t.Setenv("DOCKER_HOST", "tcp://1.2.3.4:2375")
	if got := resolveDockerHost("/usr/bin/docker"); got != "" {
		t.Errorf("DOCKER_HOST set should yield empty (FromEnv honors it), got %q", got)
	}
}

func TestResolveDockerHostNoCLI(t *testing.T) {
	t.Setenv("DOCKER_HOST", "")
	if got := resolveDockerHost(""); got != "" {
		t.Errorf("no docker CLI should yield empty, got %q", got)
	}
}

func TestResolveDockerHostFromContext(t *testing.T) {
	t.Setenv("DOCKER_HOST", "")
	dir := t.TempDir()
	fake := filepath.Join(dir, "docker")
	script := "#!/bin/sh\necho 'unix:///run/fake.sock'\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := resolveDockerHost(fake); got != "unix:///run/fake.sock" {
		t.Errorf("context host: %q", got)
	}
}

func TestStartFailsFastWhenUnreachable(t *testing.T) {
	b := &Backend{pingErr: errors.New("cannot connect")}
	r := &resource.Resource{Name: "db", Kind: resource.KindCompose, Compose: &resource.ComposeSpec{}}
	err := b.Start(context.Background(), r, env(), nopReporter{})
	if err == nil {
		t.Fatal("unreachable docker should fail Start")
	}
	if !strings.Contains(err.Error(), "db") || !strings.Contains(err.Error(), "needs Docker") {
		t.Errorf("error should name resource and docker requirement: %v", err)
	}
}

func TestReadEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# comment\nFOO=bar\n\n  BAZ=qux  \nNOEQUALS\nWITH=eq= in=value\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"FOO=bar", "BAZ=qux", "WITH=eq= in=value"}
	if len(got) != len(want) {
		t.Fatalf("readEnvFile = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReadEnvFileMissing(t *testing.T) {
	if _, err := readEnvFile(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("missing env file should error")
	}
}

func TestContainerConfigEnvFileVolumesHealthcheck(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("FROMFILE=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &resource.Resource{
		Name: "api", Kind: resource.KindCompose,
		Compose: &resource.ComposeSpec{
			Image:     "alpine",
			EnvFile:   []string{envPath},
			Env:       map[string]string{"INLINE": "yes"},
			Volumes:   []string{"/host:/container"},
			Command:   []string{"sleep", "1"},
			CPUs:      1.5,
			PidsLimit: 64,
			Restart:   "on-failure",
			Healthcheck: &resource.Healthcheck{
				Test:     []string{"curl", "localhost"},
				Interval: time.Second,
				Retries:  3,
			},
		},
	}
	cfg, hostCfg, err := containerConfig(r, env())
	if err != nil {
		t.Fatal(err)
	}
	if !containsStr(cfg.Env, "FROMFILE=1") || !containsStr(cfg.Env, "INLINE=yes") {
		t.Errorf("env not merged: %v", cfg.Env)
	}
	if len(cfg.Cmd) != 2 {
		t.Errorf("command not set: %v", cfg.Cmd)
	}
	if len(hostCfg.Binds) != 1 || hostCfg.Binds[0] != "/host:/container" {
		t.Errorf("volumes not bound: %v", hostCfg.Binds)
	}
	if hostCfg.NanoCPUs != int64(1.5*1e9) {
		t.Errorf("cpus wrong: %d", hostCfg.NanoCPUs)
	}
	if hostCfg.PidsLimit == nil || *hostCfg.PidsLimit != 64 {
		t.Errorf("pids limit wrong: %v", hostCfg.PidsLimit)
	}
	if string(hostCfg.RestartPolicy.Name) != "on-failure" {
		t.Errorf("restart policy wrong: %v", hostCfg.RestartPolicy.Name)
	}
	if cfg.Healthcheck == nil || len(cfg.Healthcheck.Test) != 3 || cfg.Healthcheck.Test[0] != "CMD" {
		t.Errorf("healthcheck not built with CMD prefix: %+v", cfg.Healthcheck)
	}
}

func TestContainerConfigSingleHealthcheckUsesShell(t *testing.T) {
	r := &resource.Resource{
		Name: "api", Kind: resource.KindCompose,
		Compose: &resource.ComposeSpec{
			Image:       "alpine",
			Healthcheck: &resource.Healthcheck{Test: []string{"pgrep x"}},
		},
	}
	cfg, _, err := containerConfig(r, env())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Healthcheck.Test[0] != "CMD-SHELL" {
		t.Errorf("single-element test should use CMD-SHELL, got %v", cfg.Healthcheck.Test)
	}
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
