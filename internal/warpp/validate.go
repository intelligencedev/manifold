package warpp

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

type Diagnostic struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Message  string   `json:"message"`
	Path     string   `json:"path,omitempty"`
}

func HasErrors(diags []Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}

// dynamicAsEnum lists the literal values a "dynamic:as" config input accepts.
var dynamicAsEnum = map[string]bool{
	"text": true, "number": true, "boolean": true, "json": true, "list<json>": true,
}

type collector struct{ diags []Diagnostic }

func (c *collector) errf(code, path, format string, args ...any) {
	c.diags = append(c.diags, Diagnostic{SeverityError, code, fmt.Sprintf(format, args...), path})
}

func (c *collector) warnf(code, path, format string, args ...any) {
	c.diags = append(c.diags, Diagnostic{SeverityWarning, code, fmt.Sprintf(format, args...), path})
}

// typeScope resolves "node.port" -> concrete output type through the scope chain.
type typeScope struct {
	parent *typeScope
	types  map[string]map[string]Type // nodeID -> port -> type
}

func (s *typeScope) lookup(ref PortRef) (Type, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		if ports, ok := cur.types[ref.Node]; ok {
			t, ok := ports[ref.Port]
			return t, ok
		}
	}
	return Type{}, false
}

func siblingIDs(nodes []Node) map[string]bool {
	ids := make(map[string]bool, len(nodes))
	for i := range nodes {
		ids[nodes[i].ID] = true
	}
	return ids
}

// Validate validates a workflow document and returns diagnostics.
func Validate(doc Document, resolve Resolver) []Diagnostic {
	_, diags := ResolveOutputTypes(doc, resolve)
	return diags
}

// ResolveOutputTypes validates the document and returns the resolved output
// port types for each root-scope node (including the virtual "in" node).
func ResolveOutputTypes(doc Document, resolve Resolver) (map[string]map[string]Type, []Diagnostic) {
	c := &collector{}
	if strings.TrimSpace(doc.ID) == "" {
		c.errf("workflow.id.required", "id", "workflow id is required")
	} else if !nodeIDPattern.MatchString(doc.ID) {
		c.errf("workflow.id.invalid", "id", "workflow id must match [a-zA-Z0-9_-]+")
	}
	if strings.TrimSpace(doc.Name) == "" {
		c.errf("workflow.name.required", "name", "workflow name is required")
	}

	root := &typeScope{types: map[string]map[string]Type{}}
	inPorts := map[string]Type{}
	seenInput := map[string]bool{}
	for i, p := range doc.Inputs {
		path := fmt.Sprintf("inputs[%d]", i)
		if seenInput[p.Name] {
			c.errf("workflow.input.duplicate", path, "duplicate workflow input %q", p.Name)
			continue
		}
		seenInput[p.Name] = true
		t, err := ParseType(p.Type)
		if err != nil || t.HasVar() {
			c.errf("workflow.input.type", path, "workflow input %q must have a concrete type", p.Name)
			continue
		}
		if p.Default != nil {
			if err := Conforms(p.Default, t); err != nil {
				c.errf("workflow.input.default", path, "workflow input %q default: %v", p.Name, err)
			}
		}
		inPorts[p.Name] = t
	}
	root.types[ReservedInputNode] = inPorts

	validateScope(c, "workflow", doc.Nodes, doc.Outputs, root, resolve)

	if doc.Publish.Tool && len(doc.Outputs) == 0 {
		c.warnf("workflow.outputs.empty", "outputs", "workflow is published as a tool but declares no outputs")
	}
	return root.types, c.diags
}

func validateScope(c *collector, scopePath string, nodes []Node, outputs map[string]Binding, scope *typeScope, resolve Resolver) {
	// Pass 1: structural checks and manifest resolution.
	manifests := map[string]Manifest{}
	seen := map[string]bool{}
	for i := range nodes {
		n := &nodes[i]
		path := fmt.Sprintf("%s.nodes[%d]", scopePath, i)
		validateNodeIdentity(c, n, path, seen, scope)
		m, ok := resolve(n.Type)
		if !ok {
			c.errf("node.type.unknown", path+".type", "unknown node type %q", n.Type)
		} else {
			manifests[n.ID] = m
			validateNodeBodyPresence(c, n, m, path)
		}
		validateNodePolicy(c, n, path)
	}

	// Cycle check (body outer-refs lifted). Stop type-checking on cycle.
	siblings := siblingIDs(nodes)
	isLocal := func(id string) bool { return siblings[id] }
	order, acyclic := topoOrder(nodes, func(n *Node) []string { return nodeDeps(n, isLocal) })
	if !acyclic {
		c.errf("workflow.graph.cycle", scopePath+".nodes", "workflow graph contains at least one cycle")
		return
	}

	nodeByID := map[string]*Node{}
	for i := range nodes {
		nodeByID[nodes[i].ID] = &nodes[i]
	}

	// Pass 2: type-check in topological order so sources are resolved first.
	for _, id := range order {
		n := nodeByID[id]
		m, ok := manifests[id]
		if !ok {
			scope.types[id] = map[string]Type{}
			continue
		}
		checkNode(c, n, m, scope, resolve, fmt.Sprintf("%s.nodes.%s", scopePath, id))
	}

	// Scope outputs must resolve.
	outNames := make([]string, 0, len(outputs))
	for name := range outputs {
		outNames = append(outNames, name)
	}
	sort.Strings(outNames)
	for _, name := range outNames {
		b := outputs[name]
		if b.HasValue {
			continue
		}
		ref, err := ParseRef(b.From)
		if err != nil {
			c.errf("workflow.output.ref", scopePath+".outputs."+name, "output %q has invalid ref %q", name, b.From)
			continue
		}
		if _, ok := scope.lookup(ref); !ok {
			c.errf("workflow.output.ref", scopePath+".outputs."+name, "output %q references unknown port %q", name, b.From)
		}
	}
}

func validateNodeIdentity(c *collector, n *Node, path string, seen map[string]bool, scope *typeScope) {
	if strings.TrimSpace(n.ID) == "" {
		c.errf("node.id.required", path+".id", "node id is required")
		return
	}
	if !nodeIDPattern.MatchString(n.ID) {
		c.errf("node.id.invalid", path+".id", "node id %q must match [a-zA-Z0-9_-]+", n.ID)
	}
	if n.ID == ReservedInputNode || n.ID == ReservedItemNode {
		c.errf("node.id.reserved", path+".id", "node id %q is reserved", n.ID)
	}
	if seen[n.ID] {
		c.errf("node.id.duplicate", path+".id", "duplicate node id %q", n.ID)
	}
	seen[n.ID] = true
}

func validateNodeBodyPresence(c *collector, n *Node, m Manifest, path string) {
	if n.Type == "control.map" {
		if n.Body == nil {
			c.errf("node.body.required", path+".body", "control.map requires a body")
		}
		return
	}
	if n.Body != nil {
		c.errf("node.body.forbidden", path+".body", "node type %q does not take a body", n.Type)
	}
}

func validateNodePolicy(c *collector, n *Node, path string) {
	if n.Policy == nil {
		return
	}
	p := n.Policy
	if strings.TrimSpace(p.Timeout) != "" {
		if _, err := time.ParseDuration(p.Timeout); err != nil {
			c.errf("node.policy.timeout", path+".policy.timeout", "invalid timeout %q", p.Timeout)
		}
	}
	switch p.OnError {
	case "", "fail", "skip":
	default:
		c.errf("node.policy.on_error", path+".policy.on_error", "on_error must be fail or skip")
	}
	if p.Retries.Max < 0 {
		c.errf("node.policy.retries", path+".policy.retries.max", "retries.max must be >= 0")
	}
	switch p.Retries.Backoff {
	case "", "fixed", "exponential":
	default:
		c.errf("node.policy.backoff", path+".policy.retries.backoff", "backoff must be fixed or exponential")
	}
}

// checkNode type-checks one node's inputs, unifies its type variable, and
// records its resolved output types into the scope.
func checkNode(c *collector, n *Node, m Manifest, scope *typeScope, resolve Resolver, path string) {
	inputSpecs := map[string]PortSpec{}
	for _, p := range m.Inputs {
		inputSpecs[p.Name] = p
	}
	for name := range n.Inputs {
		if _, ok := inputSpecs[name]; !ok {
			c.errf("node.input.unknown", path+".inputs."+name, "unknown input %q for node type %q", name, n.Type)
		}
	}

	var varType *Type // unified T
	varSet := false
	bindVar := func(t Type, portPath string) {
		if !varSet {
			varType = &t
			varSet = true
			return
		}
		if *varType != t {
			c.errf("node.type_var.conflict", portPath, "type variable resolves to both %s and %s", varType, t)
		}
	}

	for _, p := range m.Inputs {
		portPath := path + ".inputs." + p.Name
		in, provided := n.Inputs[p.Name]

		// Form checks.
		if provided && !formMatches(in, p.Variadic) {
			c.errf("node.input.form", portPath, "input %q has wrong binding form for variadic %q", p.Name, p.Variadic)
			continue
		}

		portType, isDynamic := parsePortType(p.Type)

		if !provided {
			if p.Required && p.Default == nil {
				c.errf("node.input.required", portPath, "required input %q is not set", p.Name)
			}
			continue
		}

		// Dynamic config inputs (the "as" literal) are checked when we resolve
		// the dependent output; here they are plain text inputs.
		bindings := collectBindings(in, p.Variadic)
		if p.Required && (p.Variadic == "list") && len(bindings) == 0 {
			c.errf("node.input.required", portPath, "required list input %q is empty", p.Name)
			continue
		}

		// A named-variadic var port (data.template vars) is a wildcard: each
		// labeled binding may be any type and does not unify.
		isWildcard := portType.HasVar() && p.Variadic == "named"
		isVarPort := portType.HasVar() && p.Variadic != "named"
		for _, b := range bindings {
			bt, ok, isRef := resolveBindingType(b, scope)
			if isRef && !ok {
				c.errf("node.input.ref", portPath, "input %q references unknown port %q", p.Name, b.From)
				continue
			}
			switch {
			case isDynamic, isWildcard:
				// dynamic: resolved at output; wildcard: accept any type
			case isVarPort:
				if portType.Kind == KindList {
					// list<T>: source must be a list (T=elem) or json (T=json).
					switch bt.Kind {
					case KindList:
						bindVar(Type{Kind: bt.Elem}, portPath)
					case KindJSON:
						bindVar(Type{Kind: KindJSON}, portPath)
					default:
						code := "node.input.type_mismatch"
						if n.Type == "control.map" && p.Name == "items" {
							code = "map.items.type"
						}
						c.errf(code, portPath, "input %q expects a list, got %s", p.Name, bt)
					}
				} else {
					bindVar(bt, portPath)
				}
			default:
				checkConcreteBinding(c, b, bt, portType, isRef, portPath, p.Name)
			}
		}
	}

	// control.map: body validation + result typing.
	if n.Type == "control.map" && n.Body != nil {
		checkMapBody(c, n, scope, resolve, path, varType)
	}

	scope.types[n.ID] = resolveOutputTypes(c, n, m, scope, varType, varSet, path)
}

func checkConcreteBinding(c *collector, b Binding, bt Type, portType Type, isRef bool, portPath, name string) {
	if isRef {
		if !Assignable(bt, portType) {
			c.errf("node.input.type_mismatch", portPath, "input %q: %s not assignable to %s", name, bt, portType)
		}
		return
	}
	// Literals must match the port type directly; the coercion table applies
	// only to wired connections, not to hand-entered literal values.
	if Conforms(b.Value, portType) != nil {
		c.errf("node.input.literal", portPath, "input %q literal does not match %s", name, portType)
	}
}

func checkMapBody(c *collector, n *Node, scope *typeScope, resolve Resolver, path string, elemVar *Type) {
	// on_item_error enum.
	if in, ok := n.Inputs["on_item_error"]; ok && in.One != nil && in.One.HasValue {
		if s, ok := in.One.Value.(string); ok && s != "fail" && s != "skip" {
			c.errf("map.on_item_error.enum", path+".inputs.on_item_error", "on_item_error must be fail or skip")
		}
	}
	// body outputs must be exactly {"result": ...}.
	if len(n.Body.Outputs) != 1 {
		c.errf("map.body.outputs.single", path+".body.outputs", "map body must declare exactly one output named result")
	} else if _, ok := n.Body.Outputs["result"]; !ok {
		c.errf("map.body.outputs.single", path+".body.outputs", "map body output must be named result")
	}
	itemType := Type{Kind: KindJSON}
	if elemVar != nil {
		itemType = *elemVar
	}
	child := &typeScope{
		parent: scope,
		types: map[string]map[string]Type{
			ReservedItemNode: {
				"value": itemType,
				"index": {Kind: KindNumber},
			},
		},
	}
	validateScope(c, path+".body", n.Body.Nodes, n.Body.Outputs, child, resolve)
}

func resolveOutputTypes(c *collector, n *Node, m Manifest, scope *typeScope, varType *Type, varSet bool, path string) map[string]Type {
	out := map[string]Type{}
	for _, p := range m.Outputs {
		switch {
		case p.Type == DynamicBody:
			out[p.Name] = resolveMapResultType(n, scope)
		case strings.HasPrefix(p.Type, DynamicPrefix):
			out[p.Name] = resolveDynamicType(c, n, m, p.Type, path)
		default:
			pt, _ := parsePortType(p.Type)
			if pt.HasVar() {
				if !varSet || varType == nil {
					c.errf("node.type_var.unresolved", path, "type variable for output %q could not be resolved", p.Name)
					out[p.Name] = Type{Kind: KindJSON}
					continue
				}
				if pt.Kind == KindList {
					out[p.Name] = Type{Kind: KindList, Elem: varType.Kind}
				} else {
					out[p.Name] = *varType
				}
				continue
			}
			out[p.Name] = pt
		}
	}
	return out
}

func resolveDynamicType(c *collector, n *Node, m Manifest, portType, path string) Type {
	configName := strings.TrimPrefix(portType, DynamicPrefix)
	literal := ""
	if in, ok := n.Inputs[configName]; ok {
		if in.One == nil || !in.One.HasValue {
			c.errf("node.dynamic.literal_required", path+".inputs."+configName, "input %q must be a literal", configName)
			return Type{Kind: KindJSON}
		}
		s, ok := in.One.Value.(string)
		if !ok || !dynamicAsEnum[s] {
			c.errf("node.dynamic.enum", path+".inputs."+configName, "input %q must be one of text/number/boolean/json/list<json>", configName)
			return Type{Kind: KindJSON}
		}
		literal = s
	} else if spec, ok := m.Input(configName); ok {
		if def, ok := spec.Default.(string); ok {
			literal = def
		}
	}
	if literal == "" {
		literal = "json"
	}
	t, err := ParseType(literal)
	if err != nil {
		return Type{Kind: KindJSON}
	}
	return t
}

// resolveMapResultType types the map's `results` output as list<K> where K is
// the body's single `result` output kind. The validation child scope built in
// checkMapBody is discarded, so deriveBodyResultKind replays body typing.
func resolveMapResultType(n *Node, scope *typeScope) Type {
	if n.Body == nil {
		return Type{Kind: KindList, Elem: KindJSON}
	}
	if _, ok := n.Body.Outputs["result"]; !ok {
		return Type{Kind: KindList, Elem: KindJSON}
	}
	return Type{Kind: KindList, Elem: deriveBodyResultKind(n, scope)}
}

// deriveBodyResultKind re-resolves the map body's single result output type by
// replaying node output resolution in a child scope. Nested lists collapse to
// json because list<list<...>> is not representable.
func deriveBodyResultKind(n *Node, scope *typeScope) Kind {
	itemType := Type{Kind: KindJSON}
	if in, ok := n.Inputs["items"]; ok && in.One != nil && !in.One.HasValue {
		if ref, err := ParseRef(in.One.From); err == nil {
			if t, ok := scope.lookup(ref); ok {
				if t.Kind == KindList {
					itemType = Type{Kind: t.Elem}
				} else if t.Kind == KindJSON {
					itemType = Type{Kind: KindJSON}
				}
			}
		}
	}
	child := &typeScope{
		parent: scope,
		types: map[string]map[string]Type{
			ReservedItemNode: {"value": itemType, "index": {Kind: KindNumber}},
		},
	}
	replayBodyTypes(n.Body.Nodes, child)
	b := n.Body.Outputs["result"]
	if b.HasValue {
		return InferLiteral(b.Value).Kind
	}
	ref, err := ParseRef(b.From)
	if err != nil {
		return KindJSON
	}
	t, ok := child.lookup(ref)
	if !ok {
		return KindJSON
	}
	if t.Kind == KindList {
		return KindJSON // nested lists collapse
	}
	return t.Kind
}

// replayBodyTypes recomputes body node output types (no diagnostics) so the
// map result type can be derived after the discarded validation child scope.
func replayBodyTypes(nodes []Node, scope *typeScope) {
	resolve := BuiltinResolver()
	silent := &collector{}
	siblings := siblingIDs(nodes)
	order, ok := topoOrder(nodes, func(n *Node) []string {
		return nodeDeps(n, func(id string) bool { return siblings[id] })
	})
	if !ok {
		return
	}
	byID := map[string]*Node{}
	for i := range nodes {
		byID[nodes[i].ID] = &nodes[i]
	}
	for _, id := range order {
		n := byID[id]
		m, ok := resolve(n.Type)
		if !ok {
			scope.types[id] = map[string]Type{}
			continue
		}
		checkNode(silent, n, m, scope, resolve, "replay."+id)
	}
}

// --- binding helpers ---

func parsePortType(s string) (Type, bool) {
	if s == DynamicBody || strings.HasPrefix(s, DynamicPrefix) {
		return Type{Kind: KindJSON}, true
	}
	t, err := ParseType(s)
	if err != nil {
		return Type{Kind: KindJSON}, false
	}
	return t, false
}

func formMatches(in Input, variadic string) bool {
	switch variadic {
	case "list":
		return in.List != nil && in.One == nil && in.Named == nil
	case "named":
		return in.Named != nil && in.One == nil && in.List == nil
	default:
		return in.One != nil && in.List == nil && in.Named == nil
	}
}

func collectBindings(in Input, variadic string) []Binding {
	switch variadic {
	case "list":
		return in.List
	case "named":
		keys := make([]string, 0, len(in.Named))
		for k := range in.Named {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]Binding, 0, len(keys))
		for _, k := range keys {
			out = append(out, in.Named[k])
		}
		return out
	default:
		if in.One != nil {
			return []Binding{*in.One}
		}
		return nil
	}
}

func resolveBindingType(b Binding, scope *typeScope) (t Type, ok bool, isRef bool) {
	if b.HasValue {
		return InferLiteral(b.Value), true, false
	}
	ref, err := ParseRef(b.From)
	if err != nil {
		return Type{}, false, true
	}
	t, ok = scope.lookup(ref)
	return t, ok, true
}
