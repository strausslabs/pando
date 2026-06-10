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

func TestForgetClearsHasRun(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "state.json"))
	s.MarkRun("main", "migrate")
	s.Forget("main", "migrate")
	if s.HasRun("main", "migrate") {
		t.Error("Forget should clear run state")
	}
}

func TestForgetClearsLastInputs(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "state.json"))
	s.SetInputs("main", "seed", "hash123")
	s.Forget("main", "seed")
	if got := s.LastInputs("main", "seed"); got != "" {
		t.Errorf("Forget should clear last inputs, got %q", got)
	}
}

func TestForgetIsScopedToOneResource(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "state.json"))
	s.MarkRun("main", "migrate")
	s.MarkRun("main", "seed")
	s.Forget("main", "migrate")
	if s.HasRun("main", "migrate") {
		t.Error("Forget should clear the named resource")
	}
	if !s.HasRun("main", "seed") {
		t.Error("Forget must not touch other resources in the same worktree")
	}
}

func TestForgetIsScopedToOneWorktree(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "state.json"))
	s.MarkRun("main", "migrate")
	s.MarkRun("feat-x", "migrate")
	s.Forget("main", "migrate")
	if s.HasRun("main", "migrate") {
		t.Error("Forget should clear the named worktree")
	}
	if !s.HasRun("feat-x", "migrate") {
		t.Error("Forget must not touch the same resource in other worktrees")
	}
}

func TestForgetPersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, _ := Open(path)
	s.MarkRun("main", "migrate")
	s.Forget("main", "migrate")

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.HasRun("main", "migrate") {
		t.Error("Forget should have flushed the cleared state to disk")
	}
}

func TestForgetAbsentKeyIsNoOp(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "state.json"))
	s.Forget("main", "never-marked") // must not panic
	if s.HasRun("main", "never-marked") {
		t.Error("Forget on absent key should leave it unset")
	}
}
