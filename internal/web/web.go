package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// dist holds the production UI build. The directory is populated by
// `bun run build` (or `make ui`); a committed placeholder keeps go:embed valid
// when the UI has not been built yet.
//
//go:embed all:dist
var dist embed.FS

// Handler serves the embedded single-page app. Unknown non-asset paths fall
// back to index.html so client-side rendering works on any route. Returns
// (nil, false) when no real build is embedded, so the daemon can skip mounting
// the UI rather than serve a placeholder.
func Handler() (http.Handler, bool) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil, false
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(sub, path); err != nil {
			// Not a real file (a client route) → serve the SPA shell.
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	}), true
}
