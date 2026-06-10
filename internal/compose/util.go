package compose

import (
	"bytes"
	"os"
	"os/exec"
	"sort"
	"strings"
)

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func lookDocker() string {
	path, err := exec.LookPath("docker")
	if err != nil {
		return ""
	}
	return path
}

// buildEnv forces BuildKit on for the build subprocess so --secret and inline
// cache work regardless of the user's daemon default.
func buildEnv() []string {
	return append(os.Environ(), "DOCKER_BUILDKIT=1")
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return home + path[1:]
		}
	}
	return path
}

// logWriter turns a byte stream into newline-delimited log lines. stdcopy hands
// it already-demuxed stdout/stderr; it buffers partial lines across writes.
type logWriter struct {
	emit func(string)
	buf  bytes.Buffer
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	for {
		line, err := w.buf.ReadString('\n')
		if err != nil {
			// No full line yet; put the remainder back for the next write.
			w.buf.Reset()
			w.buf.WriteString(line)
			break
		}
		w.emit(strings.TrimRight(line, "\r\n"))
	}
	return len(p), nil
}

// lineBuffer collects exec output into a string.
type lineBuffer struct{ b strings.Builder }

func (l *lineBuffer) Write(p []byte) (int, error) { return l.b.Write(p) }
func (l *lineBuffer) String() string              { return l.b.String() }
