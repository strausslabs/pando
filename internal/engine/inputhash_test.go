package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/guyStrauss/pando/internal/logbuf"
	"github.com/guyStrauss/pando/internal/resource"
	"github.com/guyStrauss/pando/internal/worktree"
)

func worktreeAt(path string) worktree.Worktree {
	return worktree.Worktree{Path: path, Branch: "main", Slug: "main"}
}

func waitForCount(logs *logbuf.Store, wt, res, substr string, want int) bool {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if countLines(logs, wt, res, substr) >= want {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func TestInputHashSkipsVendorDirs(t *testing.T) {
	eng, _, _ := testEngine(t)
	root := t.TempDir()
	for _, d := range []string{"node_modules/pkg", ".git/objects"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, d, "x.go"), []byte("package x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	r := &resource.Resource{
		Name: "b", Kind: resource.KindTask,
		RunWhen: resource.RunOnChange, OnChange: []string{"**/*.go"},
	}
	if got := eng.inputHash(root, r); got != "" {
		t.Errorf("inputHash should skip node_modules/.git and find nothing, got %q", got)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o600); err != nil {
		t.Fatal(err)
	}
	if eng.inputHash(root, r) == "" {
		t.Error("a real top-level .go should produce a hash")
	}
}

func TestInputHashHardIgnoresPandoDir(t *testing.T) {
	eng, _, _ := testEngine(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".pando"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pando", "x.go"), []byte("package x"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &resource.Resource{Name: "b", Kind: resource.KindTask, RunWhen: resource.RunOnChange, OnChange: []string{"**/*.go"}}

	before := eng.inputHash(root, r)
	if err := os.WriteFile(filepath.Join(root, ".pando", "state.json"), []byte(`{"v":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pando", "x.go"), []byte("package x // changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if eng.inputHash(root, r) != before {
		t.Error("changes under .pando must not affect the input hash (would cause a rebuild loop)")
	}
}

func TestInputHashHonorsResourceIgnore(t *testing.T) {
	eng, _, _ := testEngine(t)
	root := t.TempDir()
	for _, f := range []string{"main.go", "main_test.go"} {
		if err := os.WriteFile(filepath.Join(root, f), []byte("package main"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	r := &resource.Resource{
		Name: "b", Kind: resource.KindTask, RunWhen: resource.RunOnChange,
		OnChange: []string{"*.go"},
		Ignore:   []string{"*_test.go"},
	}
	before := eng.inputHash(root, r)
	if err := os.WriteFile(filepath.Join(root, "main_test.go"), []byte("package main // edited"), 0o600); err != nil {
		t.Fatal(err)
	}
	if eng.inputHash(root, r) != before {
		t.Error("editing an ignored file must not change the hash")
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main // edited"), 0o600); err != nil {
		t.Fatal(err)
	}
	if eng.inputHash(root, r) == before {
		t.Error("editing a non-ignored file should change the hash")
	}
}

func TestOnChangeDirsHonorsIgnore(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"src", "src/vendor", ".pando"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	r := &resource.Resource{
		Name: "x", Kind: resource.KindTask, RunWhen: resource.RunOnChange,
		OnChange: []string{"src/**/*.go"},
		Ignore:   []string{"src/vendor"},
	}
	for _, d := range onChangeDirs(r, root) {
		base := filepath.Base(d)
		if base == "vendor" || base == ".pando" {
			t.Errorf("onChangeDirs should not watch ignored/hard-ignored dir %s", d)
		}
	}
}

func TestSplitGlobBase(t *testing.T) {
	cases := []struct{ pattern, base, glob string }{
		{"migrations", "migrations", ""},
		{"src/**/*.go", "src", "**/*.go"},
		{"*.go", "", "*.go"},
		{"a/b/c.txt", "a/b/c.txt", ""},
	}
	for _, c := range cases {
		base, glob := splitGlobBase(c.pattern)
		if base != c.base || glob != c.glob {
			t.Errorf("splitGlobBase(%q) = (%q, %q), want (%q, %q)", c.pattern, base, glob, c.base, c.glob)
		}
	}
}

func TestInputHashChangesWithFileContent(t *testing.T) {
	eng, _, _ := testEngine(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "migrations"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(root, "migrations", name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("001.sql", "create table a;")

	r := &resource.Resource{
		Name: "migrate", Kind: resource.KindTask,
		Task:     &resource.TaskSpec{Cmd: "true"},
		RunWhen:  resource.RunOnChange,
		OnChange: []string{"migrations/*.sql"},
	}

	h1 := eng.inputHash(root, r)
	if h1 == "" {
		t.Fatal("inputHash should be non-empty when matching files exist")
	}
	if eng.inputHash(root, r) != h1 {
		t.Error("inputHash should be stable for unchanged inputs")
	}

	write("002.sql", "create table b;")
	if eng.inputHash(root, r) == h1 {
		t.Error("inputHash should change when a new matching file is added")
	}
}

func TestInputHashEmptyWhenNoMatch(t *testing.T) {
	eng, _, _ := testEngine(t)
	r := &resource.Resource{
		Name: "migrate", Kind: resource.KindTask,
		RunWhen:  resource.RunOnChange,
		OnChange: []string{"does-not-exist/*.sql"},
	}
	if got := eng.inputHash(t.TempDir(), r); got != "" {
		t.Errorf("inputHash with no matches should be empty, got %q", got)
	}
}

func TestInputHashNoOnChange(t *testing.T) {
	eng, _, _ := testEngine(t)
	r := &resource.Resource{Name: "x", Kind: resource.KindTask}
	if got := eng.inputHash(t.TempDir(), r); got != "" {
		t.Errorf("inputHash without OnChange should be empty, got %q", got)
	}
}

func TestOnChangeDirs(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"a", "a/b", "node_modules", ".git"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	r := &resource.Resource{
		Name: "x", Kind: resource.KindTask,
		RunWhen: resource.RunOnChange, OnChange: []string{"a/**/*.go"},
	}
	dirs := onChangeDirs(r, root)
	has := func(rel string) bool {
		for _, d := range dirs {
			if d == filepath.Join(root, rel) {
				return true
			}
		}
		return false
	}
	if !has("a") || !has("a/b") {
		t.Errorf("onChangeDirs should include a and a/b: %v", dirs)
	}
	for _, d := range dirs {
		if filepath.Base(d) == "node_modules" || filepath.Base(d) == ".git" {
			t.Errorf("onChangeDirs should skip %s", d)
		}
	}
}

func TestOnChangeDirsIgnoresNonOnChange(t *testing.T) {
	r := &resource.Resource{Name: "x", Kind: resource.KindTask, Task: &resource.TaskSpec{Cmd: "true"}}
	if got := onChangeDirs(r, t.TempDir()); got != nil {
		t.Errorf("onChangeDirs should be nil for a non-onChange resource, got %v", got)
	}
}

func TestEngineSkipsOnChangeUntilInputChanges(t *testing.T) {
	eng, logs, _ := testEngine(t)
	root := t.TempDir()
	seed := filepath.Join(root, "seed.txt")
	if err := os.WriteFile(seed, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	stack := &resource.Stack{Name: "pando", Resources: []*resource.Resource{{
		Name: "seeder", Kind: resource.KindTask,
		Task:     &resource.TaskSpec{Cmd: "echo seeded"},
		RunWhen:  resource.RunOnChange,
		OnChange: []string{"seed.txt"},
	}}}
	wtree := worktreeAt(root)
	if err := eng.Register(wtree, stack); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx := t.Context()

	if err := eng.Up(ctx, wtree.Slug, false); err != nil {
		t.Fatalf("up 1: %v", err)
	}
	waitForLine(logs, wtree.Slug, "seeder", "seeded")
	first := countLines(logs, wtree.Slug, "seeder", "seeded")

	if err := eng.Up(ctx, wtree.Slug, false); err != nil {
		t.Fatalf("up 2: %v", err)
	}
	if got := countLines(logs, wtree.Slug, "seeder", "seeded"); got != first {
		t.Errorf("unchanged input should skip the rerun: %d -> %d", first, got)
	}

	if err := os.WriteFile(seed, []byte("v2-changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := eng.Up(ctx, wtree.Slug, false); err != nil {
		t.Fatalf("up 3: %v", err)
	}
	if !waitForCount(logs, wtree.Slug, "seeder", "seeded", first+1) {
		t.Errorf("changed input should re-run the task: still %d", countLines(logs, wtree.Slug, "seeder", "seeded"))
	}
}

func TestEngineWatchRerunsOnChangeWithoutManualUp(t *testing.T) {
	eng, logs, _ := testEngine(t)
	root := t.TempDir()
	src := filepath.Join(root, "main.go")
	if err := os.WriteFile(src, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stack := &resource.Stack{Name: "pando", Resources: []*resource.Resource{{
		Name: "build", Kind: resource.KindTask,
		Task:     &resource.TaskSpec{Cmd: "echo built"},
		RunWhen:  resource.RunOnChange,
		OnChange: []string{"*.go"},
	}}}
	wtree := worktreeAt(root)
	if err := eng.Register(wtree, stack); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx := t.Context()
	if err := eng.Up(ctx, wtree.Slug, false); err != nil {
		t.Fatalf("up: %v", err)
	}
	if !waitForLine(logs, wtree.Slug, "build", "built") {
		t.Fatal("build task never ran on initial up")
	}
	before := countLines(logs, wtree.Slug, "build", "built")

	if err := os.WriteFile(src, []byte("package main\nvar x = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !waitForCount(logs, wtree.Slug, "build", "built", before+1) {
		t.Errorf("editing a watched .go file should re-run the build via the watcher, no manual up: still %d", before)
	}
}
