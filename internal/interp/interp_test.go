package interp

import "testing"

func scope() Scope {
	return Scope{
		Ports: map[string]int{"api": 8001, "frontend": 3001},
		Vars:  map[string]string{"ENV": "dev", "HOST": "localhost"},
	}
}

func TestPortSubstitution(t *testing.T) {
	got, err := scope().String("http://localhost:$PORT_api/health")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://localhost:8001/health" {
		t.Errorf("got %q", got)
	}
}

func TestBracedVar(t *testing.T) {
	got, err := scope().String("${HOST}:${PORT_frontend}")
	if err != nil {
		t.Fatal(err)
	}
	if got != "localhost:3001" {
		t.Errorf("got %q", got)
	}
}

func TestDefaultValue(t *testing.T) {
	got, err := scope().String("${MISSING:-fallback}")
	if err != nil {
		t.Fatal(err)
	}
	if got != "fallback" {
		t.Errorf("got %q", got)
	}
}

func TestDefaultNotUsedWhenPresent(t *testing.T) {
	got, _ := scope().String("${ENV:-prod}")
	if got != "dev" {
		t.Errorf("present var should win, got %q", got)
	}
}

func TestEscapedDollar(t *testing.T) {
	got, err := scope().String("price is $$5 and port $PORT_api")
	if err != nil {
		t.Fatal(err)
	}
	if got != "price is $5 and port 8001" {
		t.Errorf("got %q", got)
	}
}

func TestUndefinedVarErrors(t *testing.T) {
	if _, err := scope().String("$NOPE"); err == nil {
		t.Fatal("undefined var must error")
	}
	if _, err := scope().String("${ALSO_NOPE}"); err == nil {
		t.Fatal("undefined braced var must error")
	}
}

func TestUndefinedPortErrors(t *testing.T) {
	if _, err := scope().String("$PORT_unknown"); err == nil {
		t.Fatal("undefined port must error")
	}
}

func TestUnterminatedBrace(t *testing.T) {
	if _, err := scope().String("${HOST"); err == nil {
		t.Fatal("unterminated brace must error")
	}
}

func TestTrailingDollarLiteral(t *testing.T) {
	got, err := scope().String("cost$")
	if err != nil {
		t.Fatal(err)
	}
	if got != "cost$" {
		t.Errorf("lone trailing $ should pass through, got %q", got)
	}
}

func TestDollarFollowedByNonName(t *testing.T) {
	got, err := scope().String("a $ b")
	if err != nil {
		t.Fatal(err)
	}
	if got != "a $ b" {
		t.Errorf("got %q", got)
	}
}

func TestSliceAndMap(t *testing.T) {
	s := scope()
	sl, err := s.Slice([]string{"$PORT_api:8000", "$PORT_frontend:3000"})
	if err != nil {
		t.Fatal(err)
	}
	if sl[0] != "8001:8000" || sl[1] != "3001:3000" {
		t.Errorf("slice got %v", sl)
	}
	m, err := s.Map(map[string]string{"URL": "http://$HOST:$PORT_api"})
	if err != nil {
		t.Fatal(err)
	}
	if m["URL"] != "http://localhost:8001" {
		t.Errorf("map got %v", m)
	}
}

func TestNilSliceMapPassThrough(t *testing.T) {
	s := scope()
	if sl, _ := s.Slice(nil); sl != nil {
		t.Error("nil slice should stay nil")
	}
	if m, _ := s.Map(nil); m != nil {
		t.Error("nil map should stay nil")
	}
}
