package worktree

import (
	"context"
	"testing"
)

const samplePorcelain = `worktree /Users/guy/Projects/pando
HEAD abc123def456
branch refs/heads/main

worktree /Users/guy/Projects/pando-feat-x
HEAD 999888777
branch refs/heads/feature/new-ui

worktree /Users/guy/Projects/pando-detached
HEAD 111222333
detached
`

func TestParsePorcelain(t *testing.T) {
	wts := parsePorcelain(samplePorcelain)
	if len(wts) != 3 {
		t.Fatalf("want 3 worktrees, got %d", len(wts))
	}
	if wts[0].Branch != "main" || wts[0].Head != "abc123def456" {
		t.Errorf("worktree 0 wrong: %+v", wts[0])
	}
	if wts[1].Branch != "feature/new-ui" {
		t.Errorf("branch ref not stripped: %q", wts[1].Branch)
	}
	if wts[2].Branch != "detached" {
		t.Errorf("detached not handled: %+v", wts[2])
	}
}

func TestParsePorcelainUnbornBranchHasNoHead(t *testing.T) {
	// A fresh repo with no commits reports the all-zero SHA for its unborn
	// branch; Head should be empty so the UI shows nothing rather than 0000…
	const unborn = `worktree /tmp/fresh
HEAD 0000000000000000000000000000000000000000
branch refs/heads/main
`
	wts := parsePorcelain(unborn)
	if len(wts) != 1 {
		t.Fatalf("want 1 worktree, got %d", len(wts))
	}
	if wts[0].Head != "" {
		t.Errorf("unborn branch should have empty head, got %q", wts[0].Head)
	}
	if wts[0].Branch != "main" {
		t.Errorf("branch should still parse, got %q", wts[0].Branch)
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"main":           "main",
		"feature/new-ui": "feature-new-ui",
		"FIX/Bug_123":    "fix-bug-123",
		"--weird--":      "weird",
	}
	for branch, want := range cases {
		if got := Slug(branch, "/x"); got != want {
			t.Errorf("Slug(%q) = %q, want %q", branch, got, want)
		}
	}
}

func TestSlugDetachedUsesPath(t *testing.T) {
	got := Slug("detached", "/Users/guy/Projects/pando-hotfix")
	if got != "pando-hotfix" {
		t.Errorf("detached slug should use leaf dir, got %q", got)
	}
}

func TestSlugTruncates(t *testing.T) {
	long := "this-is-a-really-really-really-long-branch-name-exceeding-limit"
	got := Slug(long, "/x")
	if len(got) > 40 {
		t.Errorf("slug not truncated: len %d", len(got))
	}
}

func TestListUsesGitRunner(t *testing.T) {
	m := &Manager{git: func(ctx context.Context, args ...string) (string, error) {
		return samplePorcelain, nil
	}}
	wts, err := m.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(wts) != 3 || wts[0].Slug != "main" {
		t.Errorf("list wrong: %+v", wts)
	}
}

func TestListPropagatesGitError(t *testing.T) {
	m := &Manager{git: func(ctx context.Context, args ...string) (string, error) {
		return "", context.DeadlineExceeded
	}}
	if _, err := m.List(context.Background()); err == nil {
		t.Error("List should propagate the git runner error")
	}
}

func TestNewManagerUsesRealGit(t *testing.T) {
	if NewManager().git == nil {
		t.Error("NewManager should wire a git runner")
	}
}

func TestPortAllocationDeterministic(t *testing.T) {
	a := DefaultAllocator()
	path := "/Users/guy/Projects/pando-feat-x"
	svcs := []string{"api", "frontend", "db"}
	first := a.Allocate(path, svcs)
	for i := 0; i < 10; i++ {
		next := a.Allocate(path, svcs)
		for k, v := range first {
			if next[k] != v {
				t.Fatalf("port for %q not stable: %d vs %d", k, v, next[k])
			}
		}
	}
}

func TestPortAllocationDistinctWorktrees(t *testing.T) {
	a := DefaultAllocator()
	svcs := []string{"api"}
	p1 := a.Allocate("/path/one", svcs)["api"]
	p2 := a.Allocate("/path/two", svcs)["api"]
	if p1 == p2 {
		t.Errorf("different worktrees should usually differ; both got %d", p1)
	}
}

func TestPortsWithinRange(t *testing.T) {
	a := DefaultAllocator()
	ports := a.Allocate("/some/path", []string{"a", "b", "c"})
	for svc, p := range ports {
		if p < a.Base || p >= a.Base+a.Range {
			t.Errorf("port for %q out of range: %d", svc, p)
		}
	}
}

func TestPortAssignmentStableAcrossServiceOrder(t *testing.T) {
	a := DefaultAllocator()
	p1 := a.Allocate("/p", []string{"api", "db", "web"})
	p2 := a.Allocate("/p", []string{"web", "api", "db"})
	if p1["api"] != p2["api"] || p1["db"] != p2["db"] {
		t.Errorf("port assignment should not depend on input order: %v vs %v", p1, p2)
	}
}

func TestDeterministicAcrossRestartSameSet(t *testing.T) {
	a := DefaultAllocator()
	svcs := []string{"web", "db", "api"}
	first := a.Allocate("/p", svcs)
	for i := 0; i < 5; i++ {
		next := a.Allocate("/p", svcs)
		for k, v := range first {
			if next[k] != v {
				t.Fatalf("same (path, set) must yield same ports across calls: %s %d != %d", k, v, next[k])
			}
		}
	}
}

func TestNoDuplicatePorts(t *testing.T) {
	a := DefaultAllocator()
	ports := a.Allocate("/p", []string{"a", "b", "c", "d", "e", "f"})
	seen := map[int]string{}
	for svc, p := range ports {
		if other, dup := seen[p]; dup {
			t.Errorf("port %d assigned to both %s and %s", p, other, svc)
		}
		seen[p] = svc
	}
}

func TestProjectName(t *testing.T) {
	if got := ProjectName("pando", "feat-x"); got != "pando-feat-x" {
		t.Errorf("got %q", got)
	}
}
