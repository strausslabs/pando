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

	"github.com/strausslabs/pando/internal/resource"
)

// .pando holds state.json, which the daemon rewrites constantly — watching it would rebuild every onChange task in a loop.
var alwaysIgnoreDirs = map[string]bool{".git": true, "node_modules": true, ".pando": true}

func (e *Engine) inputHash(root string, r *resource.Resource) string {
	if len(r.OnChange) == 0 {
		return ""
	}
	seen := map[string]struct{}{}
	for _, pattern := range r.OnChange {
		for _, p := range matchInputs(root, pattern, r.Ignore) {
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

func ignored(rel string, ignore []string) bool {
	for _, pattern := range ignore {
		if matchGlob(pattern, rel) || matchGlob(pattern, filepath.Base(rel)) {
			return true
		}
	}
	return false
}

func matchInputs(root, pattern string, ignore []string) []string {
	pattern = strings.TrimPrefix(pattern, "./")
	base, glob := splitGlobBase(pattern)

	var out []string
	_ = filepath.WalkDir(filepath.Join(root, base), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if alwaysIgnoreDirs[d.Name()] || ignored(rel, ignore) {
				return filepath.SkipDir
			}
			return nil
		}
		if ignored(rel, ignore) {
			return nil
		}
		if glob == "" || matchGlob(glob, strings.TrimPrefix(rel, base+string(filepath.Separator))) || matchGlob(pattern, rel) {
			out = append(out, rel)
		}
		return nil
	})
	return out
}

func splitGlobBase(pattern string) (base, glob string) {
	parts := strings.Split(pattern, "/")
	for i, p := range parts {
		if strings.ContainsAny(p, "*?[") {
			return strings.Join(parts[:i], "/"), strings.Join(parts[i:], "/")
		}
	}
	return pattern, ""
}
