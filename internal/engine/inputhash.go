package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/guyStrauss/pando/internal/resource"
)

// inputHash digests the files a runWhen=onChange resource declares in OnChange,
// so the scheduler re-runs it only when those inputs actually change. Empty
// when nothing matches — the scheduler treats an empty hash as "don't skip".
func (e *Engine) inputHash(root string, r *resource.Resource) string {
	if len(r.OnChange) == 0 {
		return ""
	}
	seen := map[string]struct{}{}
	for _, pattern := range r.OnChange {
		for _, p := range matchInputs(root, pattern) {
			seen[p] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return ""
	}
	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	h := sha256.New()
	for _, rel := range paths {
		fi, err := os.Stat(filepath.Join(root, rel))
		if err != nil {
			continue
		}
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00%d\n", rel, fi.Size(), fi.ModTime().UnixNano())
	}
	return hex.EncodeToString(h.Sum(nil))
}

func matchInputs(root, pattern string) []string {
	pattern = strings.TrimPrefix(pattern, "./")
	base, glob := splitGlobBase(pattern)

	var out []string
	_ = filepath.WalkDir(filepath.Join(root, base), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if name := d.Name(); name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		if glob == "" || matchGlob(glob, strings.TrimPrefix(rel, base+string(filepath.Separator))) || matchGlob(pattern, rel) {
			out = append(out, rel)
		}
		return nil
	})
	return out
}

// splitGlobBase splits a pattern into the longest leading path with no glob
// metacharacters — so the walk starts there, not at the whole worktree — and
// the remaining glob.
func splitGlobBase(pattern string) (base, glob string) {
	parts := strings.Split(pattern, "/")
	for i, p := range parts {
		if strings.ContainsAny(p, "*?[") {
			return strings.Join(parts[:i], "/"), strings.Join(parts[i:], "/")
		}
	}
	return pattern, ""
}
