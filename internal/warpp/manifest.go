package warpp

const (
	// DynamicPrefix marks an output port whose concrete type is named by a
	// literal config input, e.g. "dynamic:as".
	DynamicPrefix = "dynamic:"
	// DynamicBody marks control.map's results port; its element type is the
	// body's declared output type.
	DynamicBody = "dynamic:body"
)

// Manifest declares a node type's interface (spec §6).
type Manifest struct {
	Type        string     `json:"type"`
	Title       string     `json:"title"`
	Category    string     `json:"category"`
	Description string     `json:"description,omitempty"`
	Inputs      []PortSpec `json:"inputs"`
	Outputs     []PortSpec `json:"outputs"`
}

func (m Manifest) Input(name string) (PortSpec, bool) {
	for _, p := range m.Inputs {
		if p.Name == name {
			return p, true
		}
	}
	return PortSpec{}, false
}

func (m Manifest) Output(name string) (PortSpec, bool) {
	for _, p := range m.Outputs {
		if p.Name == name {
			return p, true
		}
	}
	return PortSpec{}, false
}

// Resolver maps a node type to its manifest.
type Resolver func(nodeType string) (Manifest, bool)

// ChainResolvers combines resolvers; the first match wins.
func ChainResolvers(rs ...Resolver) Resolver {
	return func(nodeType string) (Manifest, bool) {
		for _, r := range rs {
			if r == nil {
				continue
			}
			if m, ok := r(nodeType); ok {
				return m, true
			}
		}
		return Manifest{}, false
	}
}

func req(name, typ, desc string) PortSpec {
	return PortSpec{Name: name, Type: typ, Required: true, Description: desc}
}

func opt(name, typ string, def any, desc string) PortSpec {
	return PortSpec{Name: name, Type: typ, Default: def, Description: desc}
}

// BuiltinManifests returns manifests for all built-in node types.
func BuiltinManifests() []Manifest {
	return []Manifest{
		{
			Type: "data.extract", Title: "Extract", Category: "data",
			Description: "Pluck a value out of a JSON structure by dot/index path.",
			Inputs: []PortSpec{
				req("source", "json", "Structure to extract from."),
				req("path", "text", "Dot/index path, e.g. results.0.title."),
				opt("as", "text", "json", "Output type: text, number, boolean, json, list<json>."),
			},
			Outputs: []PortSpec{{Name: "value", Type: "dynamic:as"}},
		},
		{
			Type: "data.template", Title: "Template", Category: "data",
			Description: "Build a string from named inputs using {name} slots.",
			Inputs: []PortSpec{
				req("template", "text", "Template with {name} placeholders."),
				{Name: "vars", Type: "T", Variadic: "named", Description: "Values for placeholders."},
			},
			Outputs: []PortSpec{{Name: "text", Type: "text"}},
		},
		{
			Type: "data.merge", Title: "Merge", Category: "data",
			Description: "Shallow-merge JSON objects; later inputs win.",
			Inputs: []PortSpec{
				{Name: "objects", Type: "json", Required: true, Variadic: "list", Description: "Objects to merge in order."},
			},
			Outputs: []PortSpec{{Name: "json", Type: "json"}},
		},
		{
			Type: "data.stringify", Title: "Stringify", Category: "data",
			Description: "Render any value as text (JSON values pretty-printed).",
			Inputs:      []PortSpec{req("value", "T", "Value to render.")},
			Outputs:     []PortSpec{{Name: "text", Type: "text"}},
		},
		{
			Type: "data.parse", Title: "Parse JSON", Category: "data",
			Description: "Parse text as JSON; fails on invalid input.",
			Inputs:      []PortSpec{req("text", "text", "JSON text.")},
			Outputs:     []PortSpec{{Name: "json", Type: "json"}},
		},
		{
			Type: "data.constant", Title: "Constant", Category: "data",
			Description: "A fixed literal value shared by multiple consumers.",
			Inputs: []PortSpec{
				req("value", "json", "The literal value."),
				opt("as", "text", "json", "Output type: text, number, boolean, json, list<json>."),
			},
			Outputs: []PortSpec{{Name: "value", Type: "dynamic:as"}},
		},
		{
			Type: "logic.if", Title: "If", Category: "logic",
			Description: "Route a value to exactly one branch based on a condition.",
			Inputs: []PortSpec{
				req("condition", "boolean", "Branch selector."),
				req("value", "T", "Value to route."),
			},
			Outputs: []PortSpec{{Name: "then", Type: "T"}, {Name: "else", Type: "T"}},
		},
		{
			Type: "logic.coalesce", Title: "Coalesce", Category: "logic",
			Description: "Emit the first input that fired; rejoins branches.",
			Inputs: []PortSpec{
				{Name: "values", Type: "T", Required: true, Variadic: "list", Description: "Candidates in priority order."},
			},
			Outputs: []PortSpec{{Name: "value", Type: "T"}},
		},
		{
			Type: "logic.equals", Title: "Equals", Category: "logic",
			Description: "Deep equality comparison.",
			Inputs:      []PortSpec{req("a", "T", "Left."), req("b", "T", "Right.")},
			Outputs:     []PortSpec{{Name: "result", Type: "boolean"}},
		},
		{
			Type: "logic.contains", Title: "Contains", Category: "logic",
			Description: "Substring test.",
			Inputs:      []PortSpec{req("haystack", "text", "Text to search."), req("needle", "text", "Text to find.")},
			Outputs:     []PortSpec{{Name: "result", Type: "boolean"}},
		},
		{
			Type: "logic.not", Title: "Not", Category: "logic",
			Description: "Boolean negation.",
			Inputs:      []PortSpec{req("value", "boolean", "Value to negate.")},
			Outputs:     []PortSpec{{Name: "result", Type: "boolean"}},
		},
		{
			Type: "logic.greater_than", Title: "Greater Than", Category: "logic",
			Description: "Numeric comparison a > b.",
			Inputs:      []PortSpec{req("a", "number", "Left."), req("b", "number", "Right.")},
			Outputs:     []PortSpec{{Name: "result", Type: "boolean"}},
		},
		{
			Type: "control.map", Title: "Map", Category: "control",
			Description: "Run the body subgraph once per item, gathering results.",
			Inputs: []PortSpec{
				req("items", "list<T>", "Items to iterate."),
				opt("concurrency", "number", float64(4), "Max parallel iterations."),
				opt("on_item_error", "text", "fail", "fail | skip."),
			},
			Outputs: []PortSpec{{Name: "results", Type: DynamicBody}},
		},
		{
			Type: "llm.generate", Title: "LLM", Category: "llm",
			Description: "Single LLM completion over the configured provider.",
			Inputs: []PortSpec{
				opt("instruction", "text", "", "System instruction."),
				req("input", "text", "User content."),
				opt("model", "text", "", "Model override; empty uses the default."),
			},
			Outputs: []PortSpec{{Name: "text", Type: "text"}},
		},
	}
}

// BuiltinResolver resolves the builtin node types.
func BuiltinResolver() Resolver {
	byType := map[string]Manifest{}
	for _, m := range BuiltinManifests() {
		byType[m.Type] = m
	}
	return func(nodeType string) (Manifest, bool) {
		m, ok := byType[nodeType]
		return m, ok
	}
}
