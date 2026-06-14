package selfupdate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestCheckFetchesAndCaches(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"tag_name":"v0.2.0"}`))
	}))
	defer srv.Close()

	saved := releasesURL
	releasesURL = srv.URL
	defer func() { releasesURL = saved }()

	cachePath := filepath.Join(t.TempDir(), "update.json")
	now := time.Unix(1_000_000, 0)

	st := Check(context.Background(), "v0.1.7", cachePath, now)
	if !st.Available || st.Latest != "v0.2.0" {
		t.Fatalf("first check: %+v", st)
	}
	if hits != 1 {
		t.Fatalf("expected 1 fetch, got %d", hits)
	}

	// Within TTL: served from cache, no new fetch.
	st = Check(context.Background(), "v0.1.7", cachePath, now.Add(time.Hour))
	if !st.Available || hits != 1 {
		t.Fatalf("cached check refetched or wrong: %+v hits=%d", st, hits)
	}

	// Past TTL: refetch.
	st = Check(context.Background(), "v0.1.7", cachePath, now.Add(25*time.Hour))
	if hits != 2 {
		t.Fatalf("expected refetch past TTL, hits=%d", hits)
	}
}

func TestCheckBestEffortOnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	saved := releasesURL
	releasesURL = srv.URL
	defer func() { releasesURL = saved }()

	st := Check(context.Background(), "v0.1.7", filepath.Join(t.TempDir(), "u.json"), time.Unix(1, 0))
	if st.Available || st.Latest != "" {
		t.Fatalf("error should yield no update: %+v", st)
	}
	if st.Current != "v0.1.7" {
		t.Fatalf("current should be preserved: %+v", st)
	}
}
