package interp

import (
	"fmt"
	"strings"
)

const PortPrefix = "PORT_"

type Scope struct {
	Ports map[string]int
	Vars  map[string]string
}

func (s Scope) lookup(name string) (string, bool) {
	if strings.HasPrefix(name, PortPrefix) {
		svc := strings.TrimPrefix(name, PortPrefix)
		if p, ok := s.Ports[svc]; ok {
			return fmt.Sprintf("%d", p), true
		}
		return "", false
	}
	if v, ok := s.Vars[name]; ok {
		return v, true
	}
	return "", false
}

func (s Scope) String(in string) (string, error) {
	return s.expand(in, false)
}

func (s Scope) Shell(in string) (string, error) {
	return s.expand(in, true)
}

func (s Scope) expand(in string, shell bool) (string, error) {
	var b strings.Builder
	for i := 0; i < len(in); {
		c := in[i]
		if c != '$' {
			b.WriteByte(c)
			i++
			continue
		}
		if i+1 < len(in) && in[i+1] == '$' {
			b.WriteByte('$')
			i += 2
			continue
		}
		if i+1 < len(in) && in[i+1] == '{' {
			end := strings.IndexByte(in[i+2:], '}')
			if end < 0 {
				return "", fmt.Errorf("unterminated ${ in %q", in)
			}
			expr := in[i+2 : i+2+end]
			val, ok, err := s.resolveExpr(expr, in, shell)
			if err != nil {
				return "", err
			}
			if ok {
				b.WriteString(val)
			} else {
				// Unresolved ${...}: keep the literal so nothing is silently dropped.
				b.WriteString(in[i : i+2+end+1])
			}
			i += 2 + end + 1
			continue
		}
		name, n := scanName(in[i+1:])
		if n == 0 {
			b.WriteByte('$')
			i++
			continue
		}
		val, ok := s.lookup(name)
		if !ok {
			if shell && !strings.HasPrefix(name, PortPrefix) {
				b.WriteString(in[i : i+1+n])
				i += 1 + n
				continue
			}
			return "", fmt.Errorf("undefined variable $%s in %q", name, in)
		}
		b.WriteString(val)
		i += 1 + n
	}
	return b.String(), nil
}

func (s Scope) resolveExpr(expr, src string, shell bool) (string, bool, error) {
	name := expr
	var def string
	hasDef := false
	if idx := strings.Index(expr, ":-"); idx >= 0 {
		name = expr[:idx]
		def = expr[idx+2:]
		hasDef = true
	}
	if val, ok := s.lookup(name); ok {
		return val, true, nil
	}
	if hasDef {
		return def, true, nil
	}
	if shell && !strings.HasPrefix(name, PortPrefix) {
		return "", false, nil
	}
	return "", false, fmt.Errorf("undefined variable ${%s} in %q", name, src)
}

func scanName(s string) (string, int) {
	i := 0
	for i < len(s) {
		c := s[i]
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			i++
			continue
		}
		break
	}
	return s[:i], i
}

func (s Scope) Slice(in []string) ([]string, error) {
	if in == nil {
		return nil, nil
	}
	out := make([]string, len(in))
	for i, v := range in {
		r, err := s.String(v)
		if err != nil {
			return nil, err
		}
		out[i] = r
	}
	return out, nil
}

func (s Scope) Map(in map[string]string) (map[string]string, error) {
	if in == nil {
		return nil, nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		r, err := s.String(v)
		if err != nil {
			return nil, err
		}
		out[k] = r
	}
	return out, nil
}
