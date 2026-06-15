package dag

import (
	"fmt"
	"sort"

	"github.com/strausslabs/pando/internal/resource"
)

type Node struct {
	Resource   *resource.Resource
	deps       []string
	extDeps    []string
	dependents []string
}

type Graph struct {
	nodes map[string]*Node
	order []string
}

func Build(stack *resource.Stack) (*Graph, error) {
	return BuildExternal(stack, nil)
}

func BuildExternal(stack *resource.Stack, external map[string]bool) (*Graph, error) {
	if err := stack.ValidateExternal(external); err != nil {
		return nil, err
	}
	g := &Graph{nodes: make(map[string]*Node, len(stack.Resources))}
	for _, r := range stack.Resources {
		var deps, ext []string
		for _, d := range r.AllDeps() {
			if external[d] {
				ext = append(ext, d)
				continue
			}
			deps = append(deps, d)
		}
		g.nodes[r.Name] = &Node{Resource: r, deps: deps, extDeps: ext}
	}
	for name, n := range g.nodes {
		for _, d := range n.deps {
			dep := g.nodes[d]
			dep.dependents = append(dep.dependents, name)
		}
	}
	order, err := g.topoSort()
	if err != nil {
		return nil, err
	}
	g.order = order
	return g, nil
}

func (g *Graph) Node(name string) (*Node, bool) {
	n, ok := g.nodes[name]
	return n, ok
}

func (g *Graph) Nodes() map[string]*Node { return g.nodes }

func (g *Graph) Deps(name string) []string {
	if n, ok := g.nodes[name]; ok {
		return n.deps
	}
	return nil
}

func (g *Graph) ExternalDeps(name string) []string {
	if n, ok := g.nodes[name]; ok {
		return n.extDeps
	}
	return nil
}

func (g *Graph) Dependents(name string) []string {
	if n, ok := g.nodes[name]; ok {
		return n.dependents
	}
	return nil
}

func (g *Graph) TopoOrder() []string {
	out := make([]string, len(g.order))
	copy(out, g.order)
	return out
}

func (g *Graph) topoSort() ([]string, error) {
	indeg := make(map[string]int, len(g.nodes))
	for name := range g.nodes {
		indeg[name] = 0
	}
	for _, n := range g.nodes {
		for range n.deps {
			indeg[n.Resource.Name]++
		}
	}
	var ready []string
	for name, d := range indeg {
		if d == 0 {
			ready = append(ready, name)
		}
	}
	sort.Strings(ready)

	var order []string
	for len(ready) > 0 {
		name := ready[0]
		ready = ready[1:]
		order = append(order, name)
		var newly []string
		for _, dep := range g.nodes[name].dependents {
			indeg[dep]--
			if indeg[dep] == 0 {
				newly = append(newly, dep)
			}
		}
		sort.Strings(newly)
		ready = append(ready, newly...)
	}

	if len(order) != len(g.nodes) {
		return nil, fmt.Errorf("dependency cycle detected involving: %s", g.cycleMembers(order))
	}
	return order, nil
}

func (g *Graph) cycleMembers(sorted []string) string {
	in := make(map[string]bool, len(sorted))
	for _, n := range sorted {
		in[n] = true
	}
	var stuck []string
	for name := range g.nodes {
		if !in[name] {
			stuck = append(stuck, name)
		}
	}
	sort.Strings(stuck)
	return fmt.Sprintf("%v", stuck)
}

func (g *Graph) Dirty(changed ...string) []string {
	mark := make(map[string]bool)
	var walk func(string)
	walk = func(name string) {
		if mark[name] {
			return
		}
		mark[name] = true
		for _, dep := range g.nodes[name].dependents {
			walk(dep)
		}
	}
	for _, c := range changed {
		if _, ok := g.nodes[c]; ok {
			walk(c)
		}
	}
	var out []string
	for _, name := range g.order {
		if mark[name] {
			out = append(out, name)
		}
	}
	return out
}
