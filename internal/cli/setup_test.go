package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
