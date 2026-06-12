package resource

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func (r *Resource) Fingerprint() string {
	b, err := json.Marshal(r)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

type Diff struct {
	Added   []string
	Removed []string
	Changed []string
}

func DiffStacks(old, next *Stack) Diff {
	oldFP := fingerprints(old)
	newFP := fingerprints(next)
	var d Diff
	for name, fp := range newFP {
		prev, ok := oldFP[name]
		if !ok {
			d.Added = append(d.Added, name)
		} else if prev != fp {
			d.Changed = append(d.Changed, name)
		}
	}
	for name := range oldFP {
		if _, ok := newFP[name]; !ok {
			d.Removed = append(d.Removed, name)
		}
	}
	return d
}

func fingerprints(s *Stack) map[string]string {
	if s == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(s.Resources))
	for _, r := range s.Resources {
		out[r.Name] = r.Fingerprint()
	}
	return out
}
