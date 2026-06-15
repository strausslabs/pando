package config

import (
	"context"
	"testing"
	"time"

	"github.com/guyStrauss/pando/internal/resource"
)

func load(t *testing.T, path string) *resource.Stack {
	t.Helper()
	l, err := NewLoader()
	if err != nil {
		t.Fatalf("NewLoader: %v", err)
	}
	stack, err := l.LoadFile(context.Background(), path)
	if err != nil {
		t.Fatalf("LoadFile(%s): %v", path, err)
	}
	return stack
}

func TestLoadFixtureStack(t *testing.T) {
	stack := load(t, "testdata/pando.star")
	if stack.Name != "demo" {
		t.Errorf("name = %q, want demo", stack.Name)
	}
	if len(stack.Resources) != 4 {
		t.Fatalf("want 4 resources, got %d", len(stack.Resources))
	}
}

func TestLoadKindsInferred(t *testing.T) {
	stack := load(t, "testdata/pando.star")
	want := map[string]resource.Kind{
		"db":       resource.KindCompose,
		"migrate":  resource.KindTask,
		"api":      resource.KindCompose,
		"frontend": resource.KindLocal,
	}
	for name, kind := range want {
		r, ok := stack.Get(name)
		if !ok {
			t.Errorf("missing resource %q", name)
			continue
		}
		if r.Kind != kind {
			t.Errorf("%s kind = %q, want %q", name, r.Kind, kind)
		}
	}
}

func TestLoadHelperAndConditional(t *testing.T) {
	stack := load(t, "testdata/pando.star")
	// db comes from a def helper; presence proves functions work.
	db, ok := stack.Get("db")
	if !ok || db.Compose == nil || db.Compose.Image != "postgres:16" {
		t.Fatalf("helper-produced db resource not resolved: %+v", db)
	}
	// target uses an if-expression on env.
	api, _ := stack.Get("api")
	if api.Build == nil || api.Build.Target != "dev" {
		t.Errorf("api build target not set from conditional: %+v", api.Build)
	}
	if len(api.Build.Secrets) != 1 || api.Build.Secrets[0].ID != "zscaler_cert" {
		t.Errorf("build secret not parsed: %+v", api.Build.Secrets)
	}
}

func TestLoadParsesRunWhenAndProbe(t *testing.T) {
	stack := load(t, "testdata/pando.star")
	migrate, _ := stack.Get("migrate")
	if migrate.RunWhen != resource.RunOnce {
		t.Errorf("migrate runWhen = %q, want once", migrate.RunWhen)
	}
	api, _ := stack.Get("api")
	if api.Ready.Kind != resource.ProbeHTTPGet {
		t.Errorf("api probe kind = %q, want httpGet", api.Ready.Kind)
	}
	if api.Ready.Timeout != 30*time.Second {
		t.Errorf("api probe timeout = %v, want 30s", api.Ready.Timeout)
	}
}

func TestLoadParsesLiveUpdate(t *testing.T) {
	stack := load(t, "testdata/pando.star")
	api, _ := stack.Get("api")
	if len(api.LiveUpdate) != 3 {
		t.Fatalf("want 3 liveUpdate steps, got %d", len(api.LiveUpdate))
	}
	if api.LiveUpdate[0].Sync == nil || api.LiveUpdate[0].Sync.Container != "/app/src" {
		t.Errorf("first step should be a sync: %+v", api.LiveUpdate[0])
	}
	if api.LiveUpdate[1].Run == "" || len(api.LiveUpdate[1].Trigger) != 1 {
		t.Errorf("second step should be a run with trigger: %+v", api.LiveUpdate[1])
	}
	if !api.LiveUpdate[2].Restart {
		t.Errorf("third step should be restart: %+v", api.LiveUpdate[2])
	}
}

func TestLoadParsesEveryMemory(t *testing.T) {
	stack := load(t, "testdata/periodic.star")

	sync, ok := stack.Get("sync")
	if !ok {
		t.Fatal("missing sync resource")
	}
	if sync.Every != 30*time.Minute {
		t.Errorf("sync every = %v, want 30m", sync.Every)
	}
	if sync.DefaultRunPolicy() != resource.RunAlways {
		t.Errorf("periodic task policy = %q, want always", sync.DefaultRunPolicy())
	}

	cache, _ := stack.Get("cache")
	if cache.Compose == nil || cache.Compose.Memory != 256<<20 {
		t.Errorf("cache memory = %v, want 256m (%d bytes)", cache.Compose, 256<<20)
	}
}

func TestLoadMissingFileErrors(t *testing.T) {
	l, _ := NewLoader()
	if _, err := l.LoadFile(context.Background(), "testdata/nope.star"); err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestLoadWithoutDefineStackErrors(t *testing.T) {
	if _, err := eval(context.Background(), "x.star", []byte(`x = 1`)); err == nil {
		t.Fatal("config that never calls define_stack should error")
	}
}

func TestLoadDurationAndBytesHelpers(t *testing.T) {
	src := []byte(`
define_stack(
    name = "u",
    services = {
        "t": service(task = task(cmd = "echo hi"), every = duration("2m")),
        "c": service(compose = compose(image = "redis", memory = bytes("1g"))),
    },
)
`)
	raw, err := eval(context.Background(), "u.star", src)
	if err != nil {
		t.Fatal(err)
	}
	stack, err := decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	tsk, _ := stack.Get("t")
	if tsk.Every != 2*time.Minute {
		t.Errorf("every = %v, want 2m", tsk.Every)
	}
	c, _ := stack.Get("c")
	if c.Compose.Memory != 1<<30 {
		t.Errorf("memory = %d, want 1g", c.Compose.Memory)
	}
}

func TestLoadUnknownFieldErrors(t *testing.T) {
	src := []byte(`define_stack(name = "x", services = {"a": service(task = task(cmd = "x"), bogus = 1)})`)
	if _, err := eval(context.Background(), "x.star", src); err == nil {
		t.Fatal("unknown service field should error")
	}
}
