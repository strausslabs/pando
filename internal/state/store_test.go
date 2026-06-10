package state

import (
	"path/filepath"
	"testing"
)

func TestRunRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.HasRun("main", "migrate") {
		t.Error("fresh store should report not run")
	}
	s.MarkRun("main", "migrate")
	if !s.HasRun("main", "migrate") {
		t.Error("should report run after MarkRun")
	}
}

func TestPersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, _ := Open(path)
	s.MarkRun("main", "migrate")
	s.SetInputs("main", "seed", "hash123")

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.HasRun("main", "migrate") {
		t.Error("run state not persisted across reopen")
	}
	if reopened.LastInputs("main", "seed") != "hash123" {
		t.Error("input hash not persisted")
	}
}

func TestIsolatedPerWorktree(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "state.json"))
	s.MarkRun("main", "migrate")
	if s.HasRun("feat-x", "migrate") {
		t.Error("worktrees must not share run state")
	}
}

func TestResetClearsOnlyOneWorktree(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "state.json"))
	s.MarkRun("main", "migrate")
	s.MarkRun("feat-x", "migrate")
	s.Reset("main")
	if s.HasRun("main", "migrate") {
		t.Error("reset should clear main")
	}
	if !s.HasRun("feat-x", "migrate") {
		t.Error("reset must not touch other worktrees")
	}
}

func TestOpenMissingFileIsEmpty(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("opening missing file should succeed: %v", err)
	}
	if s.HasRun("x", "y") {
		t.Error("missing file should yield empty store")
	}
}
