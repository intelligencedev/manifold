package toolnode

import (
	"context"

	"manifold/internal/llm"
	"manifold/internal/tools"
	"manifold/internal/warpp"
)

// CuratedToolNames returns the set of registry tool names covered by curated
// adapters, so the dynamic layer can skip them.
func CuratedToolNames() map[string]bool {
	out := map[string]bool{}
	for _, a := range Builtin() {
		out[a.Tool] = true
	}
	return out
}

// dynamicTool describes a registry tool exposed as a schema-derived node.
type dynamicTool struct {
	name        string
	manifest    warpp.Manifest
	passthrough bool // true when args are a single free-form json object
	argPorts    []string
}

func dynamicTools(reg tools.Registry, exclude map[string]bool) []dynamicTool {
	if reg == nil {
		return nil
	}
	var out []dynamicTool
	for _, schema := range reg.Schemas() {
		if exclude[schema.Name] || schema.Name == "" {
			continue
		}
		out = append(out, deriveDynamicTool(schema))
	}
	return out
}

// deriveDynamicTool builds a node descriptor from a tool's JSON schema. The
// node title is the tool's actual registry name (not the humanized/friendly
// title) so Flow nodes are unambiguous about which tool they call.
func deriveDynamicTool(schema llm.ToolSchema) dynamicTool {
	m := warpp.Manifest{
		Type:        "tool." + schema.Name,
		Title:       schema.Name,
		Category:    "tool",
		Description: schema.Description,
		Outputs:     []warpp.PortSpec{{Name: "result", Type: "json"}},
	}

	props, required := schemaProperties(schema.Parameters)
	if len(props) == 0 {
		// Free-form tool: accept the whole args object.
		m.Inputs = []warpp.PortSpec{{
			Name: "args", Type: "json", Default: map[string]any{},
			Description: "Arguments object for this tool.",
		}}
		return dynamicTool{name: schema.Name, manifest: m, passthrough: true}
	}

	requiredSet := map[string]bool{}
	for _, r := range required {
		requiredSet[r] = true
	}
	argPorts := make([]string, 0, len(props))
	for _, p := range props {
		desc, _ := p.prop["description"].(string)
		m.Inputs = append(m.Inputs, warpp.PortSpec{
			Name:        p.name,
			Type:        portTypeFromSchema(p.prop),
			Required:    requiredSet[p.name],
			Description: desc,
		})
		argPorts = append(argPorts, p.name)
	}
	return dynamicTool{name: schema.Name, manifest: m, argPorts: argPorts}
}

type schemaProp struct {
	name string
	prop map[string]any
}

// schemaProperties extracts ordered-ish properties and the required list from a
// JSON-schema parameters object. Property order is not guaranteed by JSON maps;
// the editor sorts ports for display, so this is acceptable.
func schemaProperties(parameters map[string]any) ([]schemaProp, []string) {
	if parameters == nil {
		return nil, nil
	}
	rawProps, _ := parameters["properties"].(map[string]any)
	var props []schemaProp
	for name, v := range rawProps {
		prop, _ := v.(map[string]any)
		if prop == nil {
			prop = map[string]any{}
		}
		props = append(props, schemaProp{name: name, prop: prop})
	}
	var required []string
	for _, r := range toAnySlice(parameters["required"]) {
		if s, ok := r.(string); ok {
			required = append(required, s)
		}
	}
	return props, required
}

func toAnySlice(v any) []any {
	switch t := v.(type) {
	case []any:
		return t
	case []string:
		out := make([]any, len(t))
		for i, s := range t {
			out[i] = s
		}
		return out
	}
	return nil
}

// portTypeFromSchema maps a JSON-schema property to a warpp port type.
func portTypeFromSchema(prop map[string]any) string {
	switch schemaType(prop["type"]) {
	case "string":
		return "text"
	case "number", "integer":
		return "number"
	case "boolean":
		return "boolean"
	case "array":
		items, _ := prop["items"].(map[string]any)
		elem := "json"
		if items != nil {
			switch schemaType(items["type"]) {
			case "string":
				elem = "text"
			case "number", "integer":
				elem = "number"
			case "boolean":
				elem = "boolean"
			}
		}
		return "list<" + elem + ">"
	default:
		return "json"
	}
}

// schemaType resolves a JSON-schema type that may be a string or a nullable
// array like ["null","string"].
func schemaType(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok && s != "null" {
				return s
			}
		}
	}
	return ""
}

// DynamicManifests returns schema-derived manifests for every registry tool not
// covered by a curated adapter.
func DynamicManifests(reg tools.Registry, exclude map[string]bool) []warpp.Manifest {
	tools := dynamicTools(reg, exclude)
	out := make([]warpp.Manifest, 0, len(tools))
	for _, t := range tools {
		out = append(out, t.manifest)
	}
	return out
}

// DynamicResolver resolves tool.<name> node types for schema-derived tools.
func DynamicResolver(reg tools.Registry, exclude map[string]bool) warpp.Resolver {
	byType := map[string]warpp.Manifest{}
	for _, t := range dynamicTools(reg, exclude) {
		byType[t.manifest.Type] = t.manifest
	}
	return func(nodeType string) (warpp.Manifest, bool) {
		m, ok := byType[nodeType]
		return m, ok
	}
}

// DynamicRunners builds runners for every schema-derived tool. The runner set
// is enumerated from catalogReg (the full tool list, so every catalog node has
// a runner), while dispatch happens through dispatchReg at call time so
// per-user policy still applies. Pass the same registry for both when policy
// is not a concern.
func DynamicRunners(catalogReg, dispatchReg tools.Registry, exclude map[string]bool) map[string]warpp.NodeRunner {
	if dispatchReg == nil {
		dispatchReg = catalogReg
	}
	runners := map[string]warpp.NodeRunner{}
	for _, t := range dynamicTools(catalogReg, exclude) {
		t := t
		runners[t.manifest.Type] = dynamicRunner(dispatchReg, t)
	}
	return runners
}

func dynamicRunner(reg tools.Registry, t dynamicTool) warpp.NodeRunner {
	return func(ctx context.Context, _ warpp.RunnerCtx, in warpp.NodeInputs) (map[string]warpp.Value, error) {
		args := map[string]any{}
		if t.passthrough {
			if v, ok := in.Values["args"]; ok {
				if m, ok := v.Data.(map[string]any); ok {
					args = m
				}
			}
		} else {
			for _, port := range t.argPorts {
				if v, ok := in.Values[port]; ok {
					args[port] = v.Data
				}
			}
		}
		parsed, err := dispatch(ctx, reg, t.name, args)
		if err != nil {
			return nil, err
		}
		return map[string]warpp.Value{"result": {Type: warpp.Type{Kind: warpp.KindJSON}, Data: parsed}}, nil
	}
}
