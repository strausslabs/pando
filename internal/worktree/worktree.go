package worktree

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

type Worktree struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
	Head   string `json:"head"`
	Slug   string `json:"slug"`
}

type gitRunner func(ctx context.Context, args ...string) (string, error)

func realGit(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

type Manager struct {
	git gitRunner
}

func NewManager() *Manager { return &Manager{git: realGit} }

func (m *Manager) List(ctx context.Context) ([]Worktree, error) {
	out, err := m.git(ctx, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parsePorcelain(out), nil
}

func parsePorcelain(out string) []Worktree {
	var wts []Worktree
	var cur *Worktree
	flush := func() {
		if cur != nil {
			cur.Slug = Slug(cur.Branch, cur.Path)
			wts = append(wts, *cur)
			cur = nil
		}
	}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur = &Worktree{Path: strings.TrimPrefix(line, "worktree ")}
		case cur == nil:
			continue
		case strings.HasPrefix(line, "HEAD "):
			head := strings.TrimPrefix(line, "HEAD ")
			// An unborn branch (fresh repo, no commits) reports the all-zero SHA;
			// treat that as "no head" so the UI shows nothing rather than 0000…
			if strings.Trim(head, "0") != "" {
				cur.Head = head
			}
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			cur.Branch = strings.TrimPrefix(ref, "refs/heads/")
		case line == "detached":
			cur.Branch = "detached"
		}
	}
	flush()
	return wts
}

var slugStrip = regexp.MustCompile(`[^a-z0-9]+`)

// Slug derives a stable, filesystem- and docker-safe identifier. It prefers the
// branch name, falling back to the worktree's leaf directory when detached or
// branch-less, so two worktrees on different paths never collide.
func Slug(branch, path string) string {
	base := branch
	if base == "" || base == "detached" {
		base = leaf(path)
	}
	s := slugStrip.ReplaceAllString(strings.ToLower(base), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "wt"
	}
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-")
	}
	return s
}

func leaf(path string) string {
	path = strings.TrimRight(path, "/")
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		return path[i+1:]
	}
	return path
}

func ProjectName(stack, slug string) string {
	return fmt.Sprintf("%s-%s", stack, slug)
}

// PortAllocator assigns deterministic ports per worktree. The same worktree
// path always yields the same ports across daemon restarts and machines, so a
// branch's services keep stable URLs. Allocation is keyed on the worktree path
// (not branch) because the path is what is truly unique and durable.
type PortAllocator struct {
	Base   int
	Range  int
	Stride int
}

// Base 27000 reads as "tree" (2-7) and sits clear of common dev ports
// (3000/5173/8000/8080). Each worktree gets a 100-port block within the range.
func DefaultAllocator() PortAllocator {
	return PortAllocator{Base: 27000, Range: 40000, Stride: 100}
}

// Allocate returns a map of service-name -> port for one worktree. Each
// worktree gets a contiguous block of `Stride` ports; within the block a
// service's slot is derived from a hash of its name, so adding or removing a
// service never moves the ports of the others. Collisions are resolved by
// deterministic linear probing in sorted name order, keeping assignment stable
// regardless of input order.
func (a PortAllocator) Allocate(worktreePath string, services []string) map[string]int {
	block := a.blockStart(worktreePath)
	sorted := append([]string(nil), services...)
	sortStrings(sorted)
	out := make(map[string]int, len(sorted))
	used := make(map[int]bool, len(sorted))
	for _, svc := range sorted {
		slot := a.slot(svc)
		for used[slot] {
			slot = (slot + 1) % a.Stride
		}
		used[slot] = true
		out[svc] = block + slot
	}
	return out
}

func (a PortAllocator) slot(service string) int {
	sum := sha256.Sum256([]byte(service))
	n := binary.BigEndian.Uint64(sum[:8])
	return int(n % uint64(a.Stride))
}

func (a PortAllocator) blockStart(path string) int {
	sum := sha256.Sum256([]byte(path))
	n := binary.BigEndian.Uint64(sum[:8])
	blocks := uint64(a.Range / a.Stride)
	if blocks == 0 {
		blocks = 1
	}
	return a.Base + int(n%blocks)*a.Stride
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
