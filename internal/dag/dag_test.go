package dag

import (
	"reflect"
	"strings"
	"testing"

	"github.com/guyStrauss/pando/internal/resource"
)

func r(name string, deps ...string) *resource.Resource {
	return &resource.Resource{
		Name: name,
		Kind: resource.KindTask,
		Task: &resource.TaskSpec{Cmd: "true"},
		Deps: deps,
	}
}

func stack(rs ...*resource.Resource) *resource.Stack {
	return &resource.Stack{Name: "test", Resources: rs}
}

func TestBuildTopoOrder(t *testing.T) {
	g, err := Build(stack(
		r("api", "migrate"),
		r("migrate", "db"),
		r("db"),
		r("seed", "migrate"),
	))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	order := g.TopoOrder()
	pos := make(map[string]int)
	for i, n := range order {
		pos[n] = i
	}
	if pos["db"] > pos["migrate"] {
		t.Errorf("db must precede migrate: %v", order)
	}
	if pos["migrate"] > pos["api"] || pos["migrate"] > pos["seed"] {
		t.Errorf("migrate must precede api and seed: %v", order)
	}
}

func TestBuildDeterministicTies(t *testing.T) {
	build := func() []string {
		g, err := Build(stack(r("c"), r("a"), r("b")))
		if err != nil {
			t.Fatal(err)
		}
		return g.TopoOrder()
	}
	first := build()
	for i := 0; i < 5; i++ {
		if !reflect.DeepEqual(first, build()) {
			t.Fatalf("topo order not deterministic: %v", first)
		}
	}
	if !reflect.DeepEqual(first, []string{"a", "b", "c"}) {
		t.Errorf("want alphabetical [a b c], got %v", first)
	}
}

func TestComposeDependsOnCountsAsDep(t *testing.T) {
	api := &resource.Resource{
		Name:    "api",
		Kind:    resource.KindCompose,
		Compose: &resource.ComposeSpec{Image: "x", DependsOn: []string{"db"}},
	}
	db := &resource.Resource{
		Name:    "db",
		Kind:    resource.KindCompose,
		Compose: &resource.ComposeSpec{Image: "postgres"},
	}
	g, err := Build(stack(api, db))
	if err != nil {
		t.Fatal(err)
	}
	order := g.TopoOrder()
	if order[0] != "db" {
		t.Errorf("db must come first: %v", order)
	}
}

func TestCycleDetected(t *testing.T) {
	_, err := Build(stack(r("a", "b"), r("b", "a")))
	if err == nil {
		t.Fatal("expected cycle error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("want cycle error, got: %v", err)
	}
}

func TestDirtyTransitiveDependents(t *testing.T) {
	g, err := Build(stack(
		r("db"),
		r("migrate", "db"),
		r("api", "migrate"),
		r("seed", "migrate"),
		r("frontend"),
	))
	if err != nil {
		t.Fatal(err)
	}
	dirty := g.Dirty("db")
	want := map[string]bool{"db": true, "migrate": true, "api": true, "seed": true}
	if len(dirty) != len(want) {
		t.Fatalf("want %d dirty, got %v", len(want), dirty)
	}
	for _, d := range dirty {
		if !want[d] {
			t.Errorf("unexpected dirty node %q", d)
		}
	}
	for _, d := range dirty {
		if d == "frontend" {
			t.Error("frontend has no dep on db, must not be dirty")
		}
	}
}

func TestDirtyInTopoOrder(t *testing.T) {
	g, _ := Build(stack(
		r("db"),
		r("migrate", "db"),
		r("api", "migrate"),
	))
	dirty := g.Dirty("db")
	if !reflect.DeepEqual(dirty, []string{"db", "migrate", "api"}) {
		t.Errorf("dirty not in topo order: %v", dirty)
	}
}

func TestDirtyUnknownNodeIgnored(t *testing.T) {
	g, _ := Build(stack(r("a")))
	if d := g.Dirty("nonexistent"); len(d) != 0 {
		t.Errorf("unknown node should yield empty dirty set, got %v", d)
	}
}

func TestDependentsTracked(t *testing.T) {
	g, _ := Build(stack(r("db"), r("api", "db"), r("worker", "db")))
	deps := g.Dependents("db")
	got := map[string]bool{}
	for _, d := range deps {
		got[d] = true
	}
	if !got["api"] || !got["worker"] {
		t.Errorf("db should have api+worker as dependents, got %v", deps)
	}
}

func TestBuildExternalDepNotWiredAsEdge(t *testing.T) {
	// api depends on "auth", which is external (shared); the graph must accept
	// it without auth being a node, record it as an external dep, and keep it
	// out of the regular dep edges.
	g, err := BuildExternal(stack(r("api", "auth")), map[string]bool{"auth": true})
	if err != nil {
		t.Fatalf("build with external dep: %v", err)
	}
	if deps := g.Deps("api"); len(deps) != 0 {
		t.Errorf("external dep should not be a regular dep edge, got %v", deps)
	}
	ext := g.ExternalDeps("api")
	if len(ext) != 1 || ext[0] != "auth" {
		t.Errorf("auth should be recorded as external dep, got %v", ext)
	}
	if _, ok := g.Node("auth"); ok {
		t.Error("external dep must not be a node in this graph")
	}
}

func TestBuildUnknownDepStillErrorsWhenNotExternal(t *testing.T) {
	if _, err := BuildExternal(stack(r("api", "ghost")), nil); err == nil {
		t.Error("dep on unknown, non-external resource should error")
	}
}
