package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const cacheTTL = 24 * time.Hour

var releasesURL = "https://api.github.com/repos/strausslabs/pando/releases/latest"

type Status struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
}

type cache struct {
	Latest      string `json:"latest"`
	CheckedUnix int64  `json:"checkedUnix"`
}

func Check(ctx context.Context, current, cachePath string, now time.Time) Status {
	latest := cachedLatest(cachePath, now)
	if latest == "" {
		if fetched, err := fetchLatest(ctx); err == nil {
			latest = fetched
			writeCache(cachePath, cache{Latest: latest, CheckedUnix: now.Unix()})
		}
	}
	return Status{Current: current, Latest: latest, Available: Newer(current, latest)}
}

func cachedLatest(path string, now time.Time) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var c cache
	if err := json.Unmarshal(b, &c); err != nil {
		return ""
	}
	if now.Sub(time.Unix(c.CheckedUnix, 0)) > cacheTTL {
		return ""
	}
	return c.Latest
}

func writeCache(path string, c cache) {
	b, err := json.Marshal(c)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, b, 0o644)
}

func fetchLatest(ctx context.Context) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, releasesURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("releases API: %s", resp.Status)
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.TagName, nil
}
