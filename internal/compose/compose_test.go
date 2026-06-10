package compose

import (
	"strings"
	"testing"

	"github.com/guyStrauss/pando/internal/resource"
	"github.com/guyStrauss/pando/internal/scheduler"
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
	if hostCfg.Resources.Memory != 256<<20 {
		t.Errorf("memory limit = %d, want %d", hostCfg.Resources.Memory, 256<<20)
	}
	if hostCfg.Resources.MemoryReservation != 256<<20 {
		t.Errorf("memory reservation = %d, want %d", hostCfg.Resources.MemoryReservation, 256<<20)
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
	if hostCfg.Resources.Memory != 0 {
		t.Errorf("unbounded by default, got memory = %d", hostCfg.Resources.Memory)
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
	w.Write([]byte("line one\nline two\npar"))
	w.Write([]byte("tial done\n"))
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
