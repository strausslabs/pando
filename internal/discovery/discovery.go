package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	uiBasePort = 7420
	uiPortSpan = 100
)

type Info struct {
	Socket       string `json:"socket"`
	PID          int    `json:"pid"`
	UIAddr       string `json:"uiAddr,omitempty"`
	GitCommonDir string `json:"gitCommonDir"`
	Version      string `json:"version,omitempty"`
	StartedUnix  int64  `json:"startedUnix"`
}

// GitCommonDir resolves the repo's shared .git directory. It is identical for
// every worktree of a repo, so hashing it yields one daemon key per repo
// regardless of which worktree the caller sits in. Empty when not in a repo.
func GitCommonDir(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return ""
	}
	abs, err := filepath.Abs(strings.TrimSpace(string(out)))
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

func runtimeDir() string {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, fmt.Sprintf("pando-%d", os.Getuid()))
}

func repoHash(gitCommonDir string) [32]byte {
	return sha256.Sum256([]byte(gitCommonDir))
}

func repoKey(gitCommonDir string) string {
	sum := repoHash(gitCommonDir)
	return hex.EncodeToString(sum[:8])
}

func SocketPath(gitCommonDir string) string {
	return filepath.Join(runtimeDir(), repoKey(gitCommonDir)+".sock")
}

func UIPort(gitCommonDir string) int {
	sum := repoHash(gitCommonDir)
	off := binary.BigEndian.Uint32(sum[:4]) % uiPortSpan
	return uiBasePort + int(off)
}

func FreeUIPort(gitCommonDir string) int {
	want := UIPort(gitCommonDir)
	for p := want; p < want+uiPortSpan; p++ {
		if portFree(p) {
			return p
		}
	}
	return want
}

func portFree(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func infoPath(gitCommonDir string) string {
	return filepath.Join(runtimeDir(), repoKey(gitCommonDir)+".json")
}

func LogPath(gitCommonDir string) string {
	return filepath.Join(runtimeDir(), repoKey(gitCommonDir)+".log")
}

// EnsureDir creates the per-user runtime dir (0700) that holds the socket and
// info file; the unix listener needs it to exist before Listen.
func EnsureDir() error {
	return os.MkdirAll(runtimeDir(), 0o700)
}

func Write(info Info) error {
	if err := EnsureDir(); err != nil {
		return err
	}
	b, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return os.WriteFile(infoPath(info.GitCommonDir), b, 0o600)
}

func Remove(gitCommonDir string) {
	_ = os.Remove(infoPath(gitCommonDir))
}

func Load(gitCommonDir string) (Info, bool) {
	b, err := os.ReadFile(infoPath(gitCommonDir))
	if err != nil {
		return Info{}, false
	}
	var info Info
	if err := json.Unmarshal(b, &info); err != nil {
		return Info{}, false
	}
	return info, true
}

func alive(socket string) bool {
	conn, err := net.DialTimeout("unix", socket, 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// Resolve returns the daemon info for the current repo and whether its socket
// is reachable. found is false when the cwd is not a repo or no daemon ever
// recorded itself; running is false when a recorded daemon is no longer up.
func Resolve(ctx context.Context) (info Info, found, running bool) {
	gcd := GitCommonDir(ctx)
	if gcd == "" {
		return Info{}, false, false
	}
	info, ok := Load(gcd)
	if !ok {
		return Info{}, false, false
	}
	return info, true, alive(info.Socket)
}
