package selfupdate

import (
	"strconv"
	"strings"
)

// A non-release current ("dev", "", unparseable) is older than any real
// release, so dev builds still see the update notice.
func Newer(current, latest string) bool {
	lc, lp := parse(current)
	rc, rp := parse(latest)
	if !rp {
		return false
	}
	if !lp {
		return true
	}
	for i := 0; i < 3; i++ {
		if lc[i] != rc[i] {
			return rc[i] > lc[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" {
		return [3]int{}, false
	}
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}
