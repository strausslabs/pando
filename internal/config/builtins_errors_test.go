package config

import (
	"strings"
	"testing"
)

func TestServicePositionalArgRejected(t *testing.T) {
	_, err := loadSrc(t, `
define_stack(
    name = "s",
    services = {"a": service("positional")},
)
`)
	if err == nil || !strings.Contains(err.Error(), "keyword") {
		t.Errorf("positional arg to service() should error, got %v", err)
	}
}

func TestDefineStackPositionalArgRejected(t *testing.T) {
	_, err := loadSrc(t, `define_stack("oops")`)
	if err == nil || !strings.Contains(err.Error(), "keyword") {
		t.Errorf("positional arg to define_stack() should error, got %v", err)
	}
}

func TestBytesWrongTypeErrors(t *testing.T) {
	_, err := loadSrc(t, `
define_stack(
    name = "b",
    services = {"c": service(compose = compose(image = "alpine", memory = bytes(True)))},
)
`)
	if err == nil || !strings.Contains(err.Error(), "size") {
		t.Errorf("bytes(bool) should error, got %v", err)
	}
}

func TestDurationWrongTypeErrors(t *testing.T) {
	_, err := loadSrc(t, `
define_stack(
    name = "d",
    services = {"p": service(task = task(cmd = "true"), every = duration(True))},
)
`)
	if err == nil || !strings.Contains(err.Error(), "duration") {
		t.Errorf("duration(bool) should error, got %v", err)
	}
}

func TestProbeBadDurationErrors(t *testing.T) {
	_, err := loadSrc(t, `
define_stack(
    name = "p",
    services = {"a": service(task = task(cmd = "true"), ready = http_get("http://x", timeout = "soon"))},
)
`)
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Errorf("probe with bad timeout should error, got %v", err)
	}
}

func TestDecodeRejectsUnknownField(t *testing.T) {
	_, err := decode(map[string]any{"name": "s", "bogusField": 1})
	if err == nil || !strings.Contains(err.Error(), "decode stack") {
		t.Errorf("decode should reject unknown stack fields, got %v", err)
	}
}

func TestDecodeTruncatesLongPayload(t *testing.T) {
	long := map[string]any{"name": "s"}
	for i := 0; i < 60; i++ {
		long[strings.Repeat("k", 1)+string(rune('a'+i%26))+string(rune('0'+i%10))] = "padding-padding"
	}
	long["bogus"] = "x"
	_, err := decode(long)
	if err == nil || !strings.Contains(err.Error(), "...") {
		t.Errorf("decode error should truncate long payloads with an ellipsis, got %v", err)
	}
}

func TestBytesPlainIntBytes(t *testing.T) {
	st := mustLoadSrc(t, `
define_stack(
    name = "b",
    services = {"c": service(compose = compose(image = "alpine", memory = bytes(4096)))},
)
`)
	if got := st.Resources[0].Compose.Memory; got != 4096 {
		t.Errorf("memory = %d, want 4096", got)
	}
}
