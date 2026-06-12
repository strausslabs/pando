package config

import (
	"fmt"
	"sort"

	"go.starlark.net/starlark"
)

// toGo converts the Starlark stack value into the JSON shape of resource.Stack:
// {name, resources:[{name, kind, ...}]}. The config writes services as a dict
// keyed by name; here we flatten it into the resource list and infer each
// resource's kind from which spec it carries.
func toGo(v starlark.Value) (any, error) {
	root, err := starToGo(v)
	if err != nil {
		return nil, err
	}
	m, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("define_stack must be called with keyword fields")
	}
	out := map[string]any{"name": m["name"]}
	services, _ := m["services"].(map[string]any)
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)

	resources := make([]any, 0, len(names))
	for _, name := range names {
		svc, ok := services[name].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("service %q must be a service(...) value", name)
		}
		res, err := normalizeResource(name, svc)
		if err != nil {
			return nil, err
		}
		resources = append(resources, res)
	}
	out["resources"] = resources
	return out, nil
}

func normalizeResource(name string, svc map[string]any) (map[string]any, error) {
	res := map[string]any{"name": name, "kind": kindOf(svc)}
	for k, v := range svc {
		switch k {
		case "deps", "every", "preview", "shared", "build", "compose", "local", "task", "liveUpdate", "hooks":
			res[k] = v
		case "runWhen":
			res["runWhen"] = v
		case "onChange":
			res["onChange"] = v
		case "ready":
			res["ready"] = v
		default:
			return nil, fmt.Errorf("service %q: unknown field %q", name, k)
		}
	}
	return res, nil
}

// kindOf infers a resource's kind from the spec it carries: a local/task spec
// wins, otherwise it is a compose resource.
func kindOf(svc map[string]any) string {
	switch {
	case svc["local"] != nil:
		return "local"
	case svc["task"] != nil:
		return "task"
	default:
		return "compose"
	}
}

func starToGo(v starlark.Value) (any, error) {
	switch t := v.(type) {
	case starlark.NoneType:
		return nil, nil
	case starlark.Bool:
		return bool(t), nil
	case starlark.String:
		return string(t), nil
	case starlark.Int:
		n, _ := t.Int64()
		return n, nil
	case starlark.Float:
		return float64(t), nil
	case *starlark.List:
		out := make([]any, 0, t.Len())
		it := t.Iterate()
		defer it.Done()
		var e starlark.Value
		for it.Next(&e) {
			g, err := starToGo(e)
			if err != nil {
				return nil, err
			}
			out = append(out, g)
		}
		return out, nil
	case starlark.Tuple:
		out := make([]any, 0, t.Len())
		for _, e := range t {
			g, err := starToGo(e)
			if err != nil {
				return nil, err
			}
			out = append(out, g)
		}
		return out, nil
	case *starlark.Dict:
		out := make(map[string]any, t.Len())
		for _, item := range t.Items() {
			key, ok := starlark.AsString(item[0])
			if !ok {
				return nil, fmt.Errorf("config dict keys must be strings, got %s", item[0].Type())
			}
			g, err := starToGo(item[1])
			if err != nil {
				return nil, err
			}
			out[key] = g
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported config value of type %s", v.Type())
	}
}
