package watcher

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Event struct {
	Path string
}

type Watcher struct {
	fsw      *fsnotify.Watcher
	debounce time.Duration

	mu      sync.Mutex
	keyOf   map[string]string
	timers  map[string]*time.Timer
	changed map[string]map[string]bool
	onFire  func(key string, paths []string)
}

func New(debounce time.Duration, onFire func(key string, paths []string)) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{
		fsw:      fsw,
		debounce: debounce,
		keyOf:    map[string]string{},
		timers:   map[string]*time.Timer{},
		changed:  map[string]map[string]bool{},
		onFire:   onFire,
	}, nil
}

func (w *Watcher) Add(path, key string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	w.mu.Lock()
	w.keyOf[abs] = key
	w.mu.Unlock()
	return w.fsw.Add(abs)
}

func (w *Watcher) Remove(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	w.mu.Lock()
	delete(w.keyOf, abs)
	w.mu.Unlock()
	_ = w.fsw.Remove(abs)
}

func (w *Watcher) Run(ctx context.Context) error {
	defer func() { _ = w.fsw.Close() }()
	for {
		select {
		case <-ctx.Done():
			w.cancelTimers()
			return nil
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return nil
			}
			if key, found := w.match(ev.Name); found {
				w.schedule(key, ev.Name)
			}
		case _, ok := <-w.fsw.Errors:
			if !ok {
				return nil
			}
		}
	}
}

func (w *Watcher) match(path string) (string, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if key, ok := w.keyOf[abs]; ok {
		return key, true
	}
	if key, ok := w.keyOf[filepath.Dir(abs)]; ok {
		return key, true
	}
	return "", false
}

func (w *Watcher) schedule(key, path string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if t, ok := w.timers[key]; ok {
		t.Stop()
	}
	if w.changed[key] == nil {
		w.changed[key] = map[string]bool{}
	}
	if path != "" {
		w.changed[key][path] = true
	}
	w.timers[key] = time.AfterFunc(w.debounce, func() {
		w.mu.Lock()
		delete(w.timers, key)
		paths := make([]string, 0, len(w.changed[key]))
		for p := range w.changed[key] {
			paths = append(paths, p)
		}
		delete(w.changed, key)
		w.mu.Unlock()
		w.onFire(key, paths)
	})
}

func (w *Watcher) cancelTimers() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, t := range w.timers {
		t.Stop()
	}
	w.timers = map[string]*time.Timer{}
}
