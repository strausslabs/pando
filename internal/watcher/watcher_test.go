package watcher

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDebounceCoalesces(t *testing.T) {
	var fires int32
	w, err := New(80*time.Millisecond, func(string, []string) { atomic.AddInt32(&fires, 1) })
	if err != nil {
		t.Fatal(err)
	}
	// Hammer the same key; should collapse to a single fire.
	for i := 0; i < 10; i++ {
		w.schedule("k", "")
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(150 * time.Millisecond)
	if got := atomic.LoadInt32(&fires); got != 1 {
		t.Errorf("debounce should coalesce to 1 fire, got %d", got)
	}
}

func TestSeparateKeysFireSeparately(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	w, _ := New(40*time.Millisecond, func(k string, _ []string) {
		mu.Lock()
		seen[k]++
		mu.Unlock()
	})
	w.schedule("a", "")
	w.schedule("b", "")
	time.Sleep(120 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if seen["a"] != 1 || seen["b"] != 1 {
		t.Errorf("each key should fire once: %v", seen)
	}
}

func TestWatchFileChangeFires(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "pando.config.ts")
	if err := os.WriteFile(file, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	fired := make(chan string, 4)
	w, err := New(50*time.Millisecond, func(k string, _ []string) { fired <- k })
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Add(dir, "cfg:main"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = w.Run(ctx) }()

	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(file, []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}

	select {
	case key := <-fired:
		if key != "cfg:main" {
			t.Errorf("wrong key fired: %q", key)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("file change did not fire watcher")
	}
}

func TestFireCarriesChangedPaths(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "version.txt")
	if err := os.WriteFile(file, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := make(chan []string, 4)
	w, err := New(50*time.Millisecond, func(_ string, paths []string) { got <- paths })
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Add(dir, "app"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = w.Run(ctx) }()

	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(file, []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case paths := <-got:
		found := false
		for _, p := range paths {
			if filepath.Base(p) == "version.txt" {
				found = true
			}
		}
		if !found {
			t.Errorf("fire should carry the changed file path, got %v", paths)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watcher did not fire")
	}
}

func TestMatchPrefersExactThenDir(t *testing.T) {
	w, _ := New(10*time.Millisecond, func(string, []string) {})
	dir := t.TempDir()
	exact := filepath.Join(dir, "exact.ts")
	_ = w.Add(exact, "exact-key")
	_ = w.Add(dir, "dir-key")

	if key, ok := w.match(exact); !ok || key != "exact-key" {
		t.Errorf("exact path should win: %q %v", key, ok)
	}
	other := filepath.Join(dir, "other.ts")
	if key, ok := w.match(other); !ok || key != "dir-key" {
		t.Errorf("non-exact path should fall back to dir key: %q %v", key, ok)
	}
}
