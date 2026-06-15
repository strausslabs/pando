package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler(t *testing.T) {
	h, ok := Handler()
	if !ok {
		if h != nil {
			t.Error("handler should be nil when index.html is absent")
		}
		t.Skip("no embedded UI assets; run `make ui` to exercise the serving paths")
	}

	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec
	}

	if rec := get("/"); rec.Code != 200 {
		t.Errorf("GET / = %d, want 200", rec.Code)
	}
	rec := get("/some/client/route")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<") {
		t.Errorf("SPA fallback failed: code %d", rec.Code)
	}

	sub, _ := fs.Sub(dist, "dist")
	entries, _ := fs.ReadDir(sub, "assets")
	if len(entries) > 0 {
		if rec := get("/assets/" + entries[0].Name()); rec.Code != http.StatusOK {
			t.Errorf("GET asset %q = %d, want 200", entries[0].Name(), rec.Code)
		}
	}
}
