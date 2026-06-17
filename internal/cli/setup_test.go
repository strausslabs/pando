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

func serveSkill(t *testing.T, body string) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestInstallSkillGlobalWritesUnderHome(t *testing.T) {
	const body = "---\nname: pando-star\n---\nbody\n"
	home := t.TempDir()
	t.Setenv("HOME", home)
	withSkillURL(serveSkill(t, body), func() {
		if err := installSkill(context.Background(), true); err != nil {
			t.Fatal(err)
		}
	})

	got, err := os.ReadFile(filepath.Join(home, ".claude", "skills", "pando-star", "SKILL.md"))
	if err != nil {
		t.Fatalf("skill not written: %v", err)
	}
	if string(got) != body {
		t.Errorf("skill content mismatch:\n got %q\nwant %q", got, body)
	}
}

func TestInstallSkillLocalWritesUnderCwd(t *testing.T) {
	const body = "skill body\n"
	cwd := t.TempDir()
	t.Chdir(cwd)
	t.Setenv("HOME", t.TempDir())
	withSkillURL(serveSkill(t, body), func() {
		if err := installSkill(context.Background(), false); err != nil {
			t.Fatal(err)
		}
	})

	if _, err := os.ReadFile(filepath.Join(cwd, ".claude", "skills", "pando-star", "SKILL.md")); err != nil {
		t.Fatalf("local skill not written under cwd: %v", err)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".claude")); !os.IsNotExist(err) {
		t.Errorf("local install must not write under ~/.claude")
	}
}

func TestInstallSkillFailsOnBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	t.Chdir(t.TempDir())
	withSkillURL(srv.URL, func() {
		if err := installSkill(context.Background(), false); err == nil {
			t.Fatal("expected error on 404, got nil")
		}
	})
}

func TestRegisterMCPWithoutClaude(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	out := captureStdout(t, func() { registerMCP("/usr/local/bin/pando", false) })
	if !strings.Contains(out, "claude mcp add pando -- /usr/local/bin/pando mcp") {
		t.Errorf("should print manual local instructions when claude is absent:\n%s", out)
	}
}

func TestRegisterMCPGlobalWithoutClaudeShowsUserScope(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	out := captureStdout(t, func() { registerMCP("/usr/local/bin/pando", true) })
	if !strings.Contains(out, "claude mcp add pando --scope user -- /usr/local/bin/pando mcp") {
		t.Errorf("global manual instructions should carry --scope user:\n%s", out)
	}
}

func TestRegisterMCPLocalInvokesClaudeWithoutUserScope(t *testing.T) {
	args := fakeClaudeArgs(t, "/usr/local/bin/pando", false)
	if strings.Contains(args, "--scope user") {
		t.Errorf("local registration must not pass --scope user, got: %s", args)
	}
	for _, want := range []string{"mcp", "add", "pando", "/usr/local/bin/pando"} {
		if !strings.Contains(args, want) {
			t.Errorf("missing %q in claude args: %s", want, args)
		}
	}
}

func TestRegisterMCPGlobalInvokesClaudeWithUserScope(t *testing.T) {
	args := fakeClaudeArgs(t, "/usr/local/bin/pando", true)
	if !strings.Contains(args, "--scope user") {
		t.Errorf("global registration must pass --scope user, got: %s", args)
	}
}

func TestSetupCmdRunsBothSteps(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	script := "#!/bin/sh\necho \"$*\" > " + argsFile + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Chdir(t.TempDir())

	cmd := setupCmd(&globalFlags{})
	withSkillURL(serveSkill(t, "skill\n"), func() {
		cmd.SetArgs(nil)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})

	if _, err := os.Stat(filepath.Join(".claude", "skills", "pando-star", "SKILL.md")); err != nil {
		t.Errorf("setup should install the skill locally: %v", err)
	}
	if _, err := os.Stat(argsFile); err != nil {
		t.Errorf("setup should register the MCP server: %v", err)
	}
}

func TestSetupCmdHonorsSkipFlags(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Chdir(t.TempDir())

	cmd := setupCmd(&globalFlags{})
	cmd.SetArgs([]string{"--no-skill", "--no-mcp"})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})

	if _, err := os.Stat(".claude"); !os.IsNotExist(err) {
		t.Error("--no-skill must not write the skill")
	}
	if strings.Contains(out, "claude mcp add") {
		t.Errorf("--no-mcp must not attempt registration:\n%s", out)
	}
}

func fakeClaudeArgs(t *testing.T, self string, global bool) string {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	script := "#!/bin/sh\necho \"$*\" > " + argsFile + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	out := captureStdout(t, func() { registerMCP(self, global) })
	if !strings.Contains(out, "registered MCP server") {
		t.Fatalf("should report success when claude succeeds:\n%s", out)
	}
	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("claude was not invoked: %v", err)
	}
	return string(got)
}
