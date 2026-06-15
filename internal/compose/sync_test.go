package compose

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestTarPathFileAndDir(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "top.txt"), []byte("top"), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "nested.txt"), []byte("nested"), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := tarPath(tw, root, "app", info); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	got := map[string]string{}
	tr := tar.NewReader(&buf)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(tr)
		got[hdr.Name] = string(body)
	}

	want := map[string]string{
		filepath.Join("app", "top.txt"):           "top",
		filepath.Join("app", "sub", "nested.txt"): "nested",
	}
	if len(got) != len(want) {
		t.Fatalf("archived %d entries, want %d: %v", len(got), len(want), got)
	}
	for name, body := range want {
		if got[name] != body {
			t.Errorf("entry %q = %q, want %q", name, got[name], body)
		}
	}
}

func TestTarPathSkipsNonRegular(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink("/nowhere", link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tarPath(tw, link, "link", info); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := tar.NewReader(&buf).Next(); err != io.EOF {
		t.Error("non-regular file should be skipped, got an entry")
	}
}
