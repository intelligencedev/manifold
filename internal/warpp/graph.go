package warpp

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	// ReservedInputNode exposes workflow inputs as ports ("in.<name>").
	ReservedInputNode = "in"
	// ReservedItemNode exposes the current Map iteration ("item.value",
	// "item.index") inside a body.
	ReservedItemNode = "item"
)

var nodeIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// PortRef addresses one output port: "<node>.<port>".
type PortRef struct {
	Node string
	Port string
}

// ParseRef splits a ref on the first dot.
func ParseRef(s string) (PortRef, error) {
	node, port, found := strings.Cut(s, ".")
	if !found || node == "" || port == "" {
		return PortRef{}, fmt.Errorf("invalid port ref %q (want node.port)", s)
	}
	if !nodeIDPattern.MatchString(node) {
		return PortRef{}, fmt.Errorf("invalid node id in ref %q", s)
	}
	return PortRef{Node: node, Port: port}, nil
}

// nodeBindings flattens all bindings of a node in deterministic order.
func nodeBindings(n *Node) []Binding {
	names := make([]string, 0, len(n.Inputs))
	for name := range n.Inputs {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Binding, 0, len(names))
	for _, name := range names {
		in := n.Inputs[name]
		if in.One != nil {
			out = append(out, *in.One)
		}
		out = append(out, in.List...)
		keys := make([]string, 0, len(in.Named))
		for k := range in.Named {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out = append(out, in.Named[k])
		}
	}
	return out
}

// nodeDeps returns the unique local node ids this node depends on. For
// control.map, refs inside the body that do not resolve to body-local nodes
// (or "item") lift to become dependencies of the map node itself (spec §6).
func nodeDeps(n *Node, isLocal func(id string) bool) []string {
	seen := map[string]bool{}
	var deps []string
	add := func(id string) {
		if id != "" && !seen[id] && isLocal(id) && id != n.ID {
			seen[id] = true
			deps = append(deps, id)
		}
	}
	collect := func(bindings []Binding, bodyLocal map[string]bool) {
		for _, b := range bindings {
			if b.HasValue || b.From == "" {
				continue
			}
			ref, err := ParseRef(b.From)
			if err != nil {
				continue // validator reports malformed refs
			}
			if bodyLocal != nil && (bodyLocal[ref.Node] || ref.Node == ReservedItemNode) {
				continue
			}
			add(ref.Node)
		}
	}
	collect(nodeBindings(n), nil)
	if n.Body != nil {
		bodyLocal := map[string]bool{}
		for _, bn := range n.Body.Nodes {
			bodyLocal[bn.ID] = true
		}
		for i := range n.Body.Nodes {
			collect(nodeBindings(&n.Body.Nodes[i]), bodyLocal)
			// Nested maps lift transitively through their own bodies.
			if inner := n.Body.Nodes[i].Body; inner != nil {
				innerNode := n.Body.Nodes[i]
				for _, dep := range nodeDeps(&innerNode, func(id string) bool {
					return !bodyLocal[id] && id != ReservedItemNode
				}) {
					if !bodyLocal[dep] {
						add(dep)
					}
				}
			}
		}
		keys := make([]string, 0, len(n.Body.Outputs))
		for k := range n.Body.Outputs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		bodyOut := make([]Binding, 0, len(keys))
		for _, k := range keys {
			bodyOut = append(bodyOut, n.Body.Outputs[k])
		}
		collect(bodyOut, bodyLocal)
	}
	sort.Strings(deps)
	return deps
}

// topoOrder returns node ids in dependency order (Kahn), declaration order
// as tie-break. ok=false when the graph has a cycle.
func topoOrder(nodes []Node, deps func(n *Node) []string) ([]string, bool) {
	index := make(map[string]int, len(nodes))
	for i, n := range nodes {
		index[n.ID] = i
	}
	indegree := make(map[string]int, len(nodes))
	dependents := make(map[string][]string, len(nodes))
	for i := range nodes {
		n := &nodes[i]
		ds := deps(n)
		indegree[n.ID] = len(ds)
		for _, d := range ds {
			dependents[d] = append(dependents[d], n.ID)
		}
	}
	var queue []string
	for _, n := range nodes {
		if indegree[n.ID] == 0 {
			queue = append(queue, n.ID)
		}
	}
	order := make([]string, 0, len(nodes))
	for len(queue) > 0 {
		sort.Slice(queue, func(a, b int) bool { return index[queue[a]] < index[queue[b]] })
		id := queue[0]
		queue = queue[1:]
		order = append(order, id)
		for _, dep := range dependents[id] {
			indegree[dep]--
			if indegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}
	return order, len(order) == len(nodes)
}
