package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withSkillURL(url string, fn func()) {
	saved := skillURL
	skillURL = url
	defer func() { skillURL = saved }()
	fn()
}

func TestInstallSkillWritesFile(t *testing.T) {
	const body = "---\nname: pando-star\n---\nbody\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	t.Setenv("HOME", t.TempDir())
	withSkillURL(srv.URL, func() {
		if err := installSkill(context.Background()); err != nil {
			t.Fatal(err)
		}
	})

	path := filepath.Join(os.Getenv("HOME"), ".claude", "skills", "pando-star", "SKILL.md")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("skill not written: %v", err)
	}
	if string(got) != body {
		t.Errorf("skill content mismatch:\n got %q\nwant %q", got, body)
	}
}

func TestInstallSkillFailsOnBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	t.Setenv("HOME", t.TempDir())
	withSkillURL(srv.URL, func() {
		if err := installSkill(context.Background()); err == nil {
			t.Fatal("expected error on 404, got nil")
		}
	})
}

func TestRegisterMCPWithoutClaude(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no `claude` on PATH
	out := captureStdout(t, func() { registerMCP("/usr/local/bin/pando") })
	if !strings.Contains(out, "claude mcp add pando") {
		t.Errorf("should print manual instructions when claude is absent:\n%s", out)
	}
}

func TestRegisterMCPWithFakeClaude(t *testing.T) {
	dir := t.TempDir()
	claude := filepath.Join(dir, "claude")
	if err := os.WriteFile(claude, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	out := captureStdout(t, func() { registerMCP("/usr/local/bin/pando") })
	if !strings.Contains(out, "registered MCP server") {
		t.Errorf("should report success when claude succeeds:\n%s", out)
	}
}
