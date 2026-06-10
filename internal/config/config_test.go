package config

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/guyStrauss/pando/internal/resource"
)

func loaderOrSkip(t *testing.T) *Loader {
	t.Helper()
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun not installed; skipping config eval test")
	}
	l, err := NewLoader()
	if err != nil {
		t.Fatalf("NewLoader: %v", err)
	}
	return l
}

func TestLoadFixtureStack(t *testing.T) {
	l := loaderOrSkip(t)
	stack, err := l.LoadFile(context.Background(), "testdata/pando.config.ts")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if stack.Name != "demo" {
		t.Errorf("name = %q, want demo", stack.Name)
	}
	if len(stack.Resources) != 4 {
		t.Fatalf("want 4 resources, got %d", len(stack.Resources))
	}
}

func TestLoadKindsInferred(t *testing.T) {
	l := loaderOrSkip(t)
	stack, err := l.LoadFile(context.Background(), "testdata/pando.config.ts")
	if err != nil {
		t.Fatal(err)
	}
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

func TestLoadResolvesImportsAndBuildSecrets(t *testing.T) {
	l := loaderOrSkip(t)
	stack, err := l.LoadFile(context.Background(), "testdata/pando.config.ts")
	if err != nil {
		t.Fatal(err)
	}
	// db comes from an imported module; presence proves import resolution.
	db, ok := stack.Get("db")
	if !ok || db.Compose == nil || db.Compose.Image != "postgres:16" {
		t.Fatalf("imported db resource not resolved: %+v", db)
	}
	api, _ := stack.Get("api")
	if api.Build == nil || api.Build.Target != "dev" {
		t.Errorf("api build target not set from env logic: %+v", api.Build)
	}
	if len(api.Build.Secrets) != 1 || api.Build.Secrets[0].ID != "zscaler_cert" {
		t.Errorf("build secret not parsed: %+v", api.Build.Secrets)
	}
}

func TestLoadParsesRunWhenAndProbe(t *testing.T) {
	l := loaderOrSkip(t)
	stack, err := l.LoadFile(context.Background(), "testdata/pando.config.ts")
	if err != nil {
		t.Fatal(err)
	}
	migrate, _ := stack.Get("migrate")
	if migrate.RunWhen != resource.RunOnce {
		t.Errorf("migrate runWhen = %q, want once", migrate.RunWhen)
	}
	api, _ := stack.Get("api")
	if api.Ready.Kind != resource.ProbeHTTPGet {
		t.Errorf("api probe kind = %q, want httpGet", api.Ready.Kind)
	}
	if api.Ready.Timeout.Seconds() != 30 {
		t.Errorf("api probe timeout = %v, want 30s", api.Ready.Timeout)
	}
}

func TestLoadParsesLiveUpdate(t *testing.T) {
	l := loaderOrSkip(t)
	stack, err := l.LoadFile(context.Background(), "testdata/pando.config.ts")
	if err != nil {
		t.Fatal(err)
	}
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

func TestLoadParsesEveryAndPreview(t *testing.T) {
	l := loaderOrSkip(t)
	stack, err := l.LoadFile(context.Background(), "testdata/periodic.config.ts")
	if err != nil {
		t.Fatal(err)
	}

	sync, ok := stack.Get("sync")
	if !ok {
		t.Fatal("missing sync resource")
	}
	if sync.Every != 30*time.Minute {
		t.Errorf("sync every = %v, want 30m", sync.Every)
	}
	if !sync.IsPeriodic() {
		t.Error("sync should be periodic")
	}
	if sync.DefaultRunPolicy() != resource.RunAlways {
		t.Errorf("periodic task policy = %q, want always", sync.DefaultRunPolicy())
	}

	web, ok := stack.Get("web")
	if !ok {
		t.Fatal("missing web resource")
	}
	if !web.Preview {
		t.Error("web should be flagged preview")
	}
	if sync.Preview {
		t.Error("sync should not be flagged preview")
	}
}

func TestLoadMissingFileErrors(t *testing.T) {
	l := loaderOrSkip(t)
	if _, err := l.LoadFile(context.Background(), "testdata/nope.config.ts"); err == nil {
		t.Fatal("expected error for missing config")
	}
}
