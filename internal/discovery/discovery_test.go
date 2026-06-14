package discovery

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestSocketPathStableAndPerRepo(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	a1 := SocketPath("/repos/a/.git")
	a2 := SocketPath("/repos/a/.git")
	b := SocketPath("/repos/b/.git")
	if a1 != a2 {
		t.Errorf("socket path not stable for the same repo: %q vs %q", a1, a2)
	}
	if a1 == b {
		t.Error("different repos must get different sockets")
	}
}

func TestWriteLoadRemoveRoundTrip(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	gcd := "/repos/demo/.git"
	in := Info{Socket: SocketPath(gcd), PID: 4321, GitCommonDir: gcd, UIAddr: "127.0.0.1:7420", StartedUnix: 100}
	if err := Write(in); err != nil {
		t.Fatal(err)
	}
	got, ok := Load(gcd)
	if !ok {
		t.Fatal("info not loaded after write")
	}
	if got != in {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", got, in)
	}
	Remove(gcd)
	if _, ok := Load(gcd); ok {
		t.Error("info should be gone after Remove")
	}
}

func TestUIPortStablePerRepoAndInRange(t *testing.T) {
	a1 := UIPort("/repos/a/.git")
	a2 := UIPort("/repos/a/.git")
	if a1 != a2 {
		t.Errorf("UI port not stable for the same repo: %d vs %d", a1, a2)
	}
	if a1 < uiBasePort || a1 >= uiBasePort+uiPortSpan {
		t.Errorf("UI port %d outside [%d,%d)", a1, uiBasePort, uiBasePort+uiPortSpan)
	}
	seen := map[int]bool{}
	for _, r := range []string{"/r/a/.git", "/r/b/.git", "/r/c/.git", "/r/d/.git", "/r/e/.git"} {
		seen[UIPort(r)] = true
	}
	if len(seen) < 2 {
		t.Errorf("expected distinct UI ports across repos, got %v", seen)
	}
}

func TestFreeUIPortWalksPastBoundPort(t *testing.T) {
	gcd := "/repos/walk/.git"
	want := UIPort(gcd)
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", want))
	if err != nil {
		t.Skipf("preferred port %d not bindable in this env: %v", want, err)
	}
	defer func() { _ = ln.Close() }()
	got := FreeUIPort(gcd)
	if got == want {
		t.Errorf("FreeUIPort returned the bound port %d; should have walked past it", want)
	}
	if got < want || got >= want+uiPortSpan {
		t.Errorf("walked port %d outside [%d,%d)", got, want, want+uiPortSpan)
	}
}

func TestLoadMissingIsNotFound(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	if _, ok := Load("/repos/never/.git"); ok {
		t.Error("Load of an unknown repo should report not-found")
	}
}

func TestResolveDetectsLiveAndDeadSocket(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	// macOS caps unix socket paths at 104 bytes, far shorter than a temp dir, so
	// bind the listener under os.TempDir() with a short name.
	sock := filepath.Join(os.TempDir(), "pando-disc-test.sock")
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close(); _ = os.Remove(sock) }()
	if !alive(sock) {
		t.Fatal("listening socket should be alive")
	}

	dead := filepath.Join(os.TempDir(), "pando-disc-dead.sock")
	_ = os.Remove(dead)
	if alive(dead) {
		t.Error("a nonexistent socket must not report alive")
	}
	_ = os.WriteFile(dead, nil, 0o600) // a plain file, nothing listening
	defer func() { _ = os.Remove(dead) }()
	if alive(dead) {
		t.Error("a plain file is not a live daemon")
	}
}
