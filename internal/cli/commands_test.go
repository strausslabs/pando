package cli

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/strausslabs/pando/internal/api"
	"github.com/strausslabs/pando/internal/client"
)

func TestPathContains(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	sibling := t.TempDir()

	if !pathContains(root, child) {
		t.Error("parent should contain nested child")
	}
	if !pathContains(root, root) {
		t.Error("a path contains itself")
	}
	if pathContains(root, sibling) {
		t.Error("unrelated sibling must not be contained")
	}
	if pathContains(child, root) {
		t.Error("child does not contain its parent")
	}
}

func TestCanonPathNonexistentFallsBackToAbs(t *testing.T) {
	got, err := canonPath("relative/does/not/exist")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("canonPath should return an absolute path, got %q", got)
	}
}

func TestResolveWorktreeExplicitFlag(t *testing.T) {
	got, err := resolveWorktree(nil, "feat")
	if err != nil || got != "feat" {
		t.Errorf("explicit flag should win: got %q err %v", got, err)
	}
}

func TestResolveWorktreeEnv(t *testing.T) {
	t.Setenv("PANDO_WORKTREE", "envwt")
	got, err := resolveWorktree(nil, "")
	if err != nil || got != "envwt" {
		t.Errorf("env var should resolve: got %q err %v", got, err)
	}
}

func TestResolveWorktreeSingleton(t *testing.T) {
	t.Setenv("PANDO_WORKTREE", "")
	cl := worktreeDaemon(t, []api.WorktreeInfo{{Slug: "only", Path: t.TempDir()}})
	got, err := resolveWorktree(cl, "")
	if err != nil || got != "only" {
		t.Errorf("single worktree should resolve: got %q err %v", got, err)
	}
}

func TestResolveWorktreeAmbiguous(t *testing.T) {
	t.Setenv("PANDO_WORKTREE", "")
	cl := worktreeDaemon(t, []api.WorktreeInfo{
		{Slug: "a", Path: t.TempDir()},
		{Slug: "b", Path: t.TempDir()},
	})
	if _, err := resolveWorktree(cl, ""); err == nil {
		t.Error("ambiguous worktrees with no cwd match should error")
	}
}

func TestResolveWorktreeByCwd(t *testing.T) {
	t.Setenv("PANDO_WORKTREE", "")
	dir := t.TempDir()
	cl := worktreeDaemon(t, []api.WorktreeInfo{
		{Slug: "other", Path: t.TempDir()},
		{Slug: "here", Path: dir},
	})
	t.Chdir(dir)
	got, err := resolveWorktree(cl, "")
	if err != nil || got != "here" {
		t.Errorf("cwd should match its worktree: got %q err %v", got, err)
	}
}

func TestPrintStatus(t *testing.T) {
	st := []api.WorktreeStatus{{
		Worktree: "main",
		Resources: []api.ResourceStatus{
			{Name: "api", Kind: "compose", Phase: "healthy", Port: 8080},
			{Name: "task", Kind: "task", Phase: "done"},
		},
	}}
	out := captureStdout(t, func() { printStatus(st) })
	for _, want := range []string{"WORKTREE", "api", "compose", "healthy", "8080", "task", "done"} {
		if !strings.Contains(out, want) {
			t.Errorf("printStatus output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "task\ttask\tdone\t0") {
		t.Error("zero port should render blank, not 0")
	}
}

func TestPrintStatusShowsConfigError(t *testing.T) {
	st := []api.WorktreeStatus{{
		Worktree: "feat",
		Error:    "evaluate config: pando.star:3:1: undefined: srvice",
	}}
	var out, errOut string
	errOut = captureStderr(t, func() {
		out = captureStdout(t, func() { printStatus(st) })
	})
	if !strings.Contains(out, "config error") {
		t.Errorf("status table should flag the config error row:\n%s", out)
	}
	if !strings.Contains(errOut, "undefined: srvice") {
		t.Errorf("stderr should carry the config error detail:\n%s", errOut)
	}
}

func TestWorktreeIn(t *testing.T) {
	st := []api.WorktreeStatus{
		{Worktree: "a"},
		{Worktree: "b", Resources: []api.ResourceStatus{{Name: "x"}}},
	}
	got := worktreeIn(st, "b")
	if len(got) != 1 || got[0].Worktree != "b" {
		t.Fatalf("worktreeIn should isolate the named worktree, got %+v", got)
	}
	if worktreeIn(st, "missing") != nil {
		t.Error("worktreeIn should return nil for an unknown worktree")
	}
}

func TestUpPrintsConfirmationAndResources(t *testing.T) {
	ops := &stubOps{}
	g := &globalFlags{socket: liveDaemon(t, ops)}
	out := captureStdout(t, func() {
		if err := runCmd(upCmd(g), "--worktree", "main"); err != nil {
			t.Fatalf("up: %v", err)
		}
	})
	if !strings.Contains(out, "main is up") {
		t.Errorf("up should confirm the worktree is up:\n%s", out)
	}
	if !strings.Contains(out, "api") {
		t.Errorf("up should print the worktree's resources:\n%s", out)
	}
}

func TestUpJSONPrintsOnlyTargetWorktree(t *testing.T) {
	ops := &stubOps{}
	g := &globalFlags{socket: liveDaemon(t, ops), json: true}
	out := captureStdout(t, func() {
		if err := runCmd(upCmd(g), "--worktree", "main"); err != nil {
			t.Fatalf("up --json: %v", err)
		}
	})
	var got []api.WorktreeStatus
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("up --json emitted invalid JSON: %v\n%s", err, out)
	}
	if len(got) != 1 || got[0].Worktree != "main" {
		t.Errorf("up --json should emit only the target worktree, got %+v", got)
	}
}

func worktreeDaemon(t *testing.T, wts []api.WorktreeInfo) *client.Client {
	t.Helper()
	dir, err := os.MkdirTemp("", "cli")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("GET /worktrees", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(wts)
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
	})
	return client.New(sock)
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	return capture(t, &os.Stdout, fn)
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	return capture(t, &os.Stderr, fn)
}

func capture(t *testing.T, target **os.File, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := *target
	*target = w
	defer func() { *target = saved }()
	fn()
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}
