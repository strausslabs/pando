package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Store is a file-backed implementation of scheduler.RunStore. It records which
// run-once tasks have executed and the last input hash of onChange tasks, keyed
// per worktree, so the daemon does not repeat setup work across restarts.
type Store struct {
	path string

	mu   sync.Mutex
	data persisted
}

type persisted struct {
	Ran    map[string]bool   `json:"ran"`
	Inputs map[string]string `json:"inputs"`
}

func key(worktree, resource string) string { return worktree + "\x00" + resource }

func Open(path string) (*Store, error) {
	s := &Store{path: path, data: persisted{Ran: map[string]bool{}, Inputs: map[string]string{}}}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.data); err != nil {
			return nil, err
		}
		if s.data.Ran == nil {
			s.data.Ran = map[string]bool{}
		}
		if s.data.Inputs == nil {
			s.data.Inputs = map[string]string{}
		}
	}
	return s, nil
}

func (s *Store) HasRun(worktree, resource string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Ran[key(worktree, resource)]
}

func (s *Store) MarkRun(worktree, resource string) {
	s.mu.Lock()
	s.data.Ran[key(worktree, resource)] = true
	s.mu.Unlock()
	s.flush()
}

func (s *Store) LastInputs(worktree, resource string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Inputs[key(worktree, resource)]
}

func (s *Store) SetInputs(worktree, resource, hash string) {
	s.mu.Lock()
	s.data.Inputs[key(worktree, resource)] = hash
	s.mu.Unlock()
	s.flush()
}

// Reset clears bookkeeping for one worktree, forcing run-once tasks to run
// again. Used by `pando up --force`.
func (s *Store) Reset(worktree string) {
	s.mu.Lock()
	prefix := worktree + "\x00"
	for k := range s.data.Ran {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(s.data.Ran, k)
		}
	}
	for k := range s.data.Inputs {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(s.data.Inputs, k)
		}
	}
	s.mu.Unlock()
	s.flush()
}

// flush writes atomically via a temp file + rename so a crash mid-write cannot
// corrupt the state file.
func (s *Store) flush() {
	s.mu.Lock()
	b, err := json.MarshalIndent(s.data, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, s.path)
}
