// Package toolnode adapts registry tools into warpp typed-port nodes.
package toolnode

import (
	"context"
	"encoding/json"
	"fmt"

	"manifold/internal/tools"
	"manifold/internal/warpp"
)

// ArgMap maps a node input port to a tool argument name. An empty Arg means
// the argument name matches the port name.
type ArgMap struct {
	Port string
	Arg  string
}

func (a ArgMap) arg() string {
	if a.Arg != "" {
		return a.Arg
	}
	return a.Port
}

// OutMap maps a tool result path to a node output port. An empty Path binds
// the whole parsed result.
type OutMap struct {
	Port string
	Path string
}

// Adapter wires one registry tool into a warpp node type.
type Adapter struct {
	NodeType string
	Tool     string
	Manifest warpp.Manifest
	Args     []ArgMap
	Outs     []OutMap
	Post     func(map[string]any) map[string]any
}

// GenericManifest is the escape-hatch node for any registry tool.
func GenericManifest() warpp.Manifest {
	return warpp.Manifest{
		Type: "tool.generic", Title: "Tool (generic)", Category: "tool",
		Description: "Call any registered tool by name with a JSON args object.",
		Inputs: []warpp.PortSpec{
			{Name: "tool", Type: "text", Required: true, Description: "Registered tool name."},
			{Name: "args", Type: "json", Default: map[string]any{}, Description: "Tool arguments."},
		},
		Outputs: []warpp.PortSpec{{Name: "result", Type: "json"}},
	}
}

// Manifests returns the manifests for the given adapters plus tool.generic.
func Manifests(adapters []Adapter) []warpp.Manifest {
	out := make([]warpp.Manifest, 0, len(adapters)+1)
	for _, a := range adapters {
		out = append(out, a.Manifest)
	}
	out = append(out, GenericManifest())
	return out
}

// Resolver resolves the adapter node types plus tool.generic.
func Resolver(adapters []Adapter) warpp.Resolver {
	byType := map[string]warpp.Manifest{}
	for _, a := range adapters {
		byType[a.NodeType] = a.Manifest
	}
	byType["tool.generic"] = GenericManifest()
	return func(nodeType string) (warpp.Manifest, bool) {
		m, ok := byType[nodeType]
		return m, ok
	}
}

// Runners builds node runners for the adapters plus tool.generic, dispatching
// through the given registry.
func Runners(reg tools.Registry, adapters []Adapter) map[string]warpp.NodeRunner {
	runners := map[string]warpp.NodeRunner{}
	for _, a := range adapters {
		a := a
		runners[a.NodeType] = a.runner(reg)
	}
	runners["tool.generic"] = genericRunner(reg)
	return runners
}

func (a Adapter) runner(reg tools.Registry) warpp.NodeRunner {
	return func(ctx context.Context, _ warpp.RunnerCtx, in warpp.NodeInputs) (map[string]warpp.Value, error) {
		args := map[string]any{}
		for _, am := range a.Args {
			if v, ok := in.Values[am.Port]; ok {
				args[am.arg()] = v.Data
			}
		}
		parsed, err := dispatch(ctx, reg, a.Tool, args)
		if err != nil {
			return nil, err
		}
		if obj, ok := parsed.(map[string]any); ok && a.Post != nil {
			parsed = a.Post(obj)
		}
		return mapOutputs(a.Tool, a.Manifest, a.Outs, parsed)
	}
}

func genericRunner(reg tools.Registry) warpp.NodeRunner {
	return func(ctx context.Context, _ warpp.RunnerCtx, in warpp.NodeInputs) (map[string]warpp.Value, error) {
		toolName, _ := in.Values["tool"].Data.(string)
		if toolName == "" {
			return nil, fmt.Errorf("tool.generic: tool name required")
		}
		args := map[string]any{}
		if v, ok := in.Values["args"]; ok {
			if m, ok := v.Data.(map[string]any); ok {
				args = m
			}
		}
		parsed, err := dispatch(ctx, reg, toolName, args)
		if err != nil {
			return nil, err
		}
		return map[string]warpp.Value{"result": {Type: warpp.Type{Kind: warpp.KindJSON}, Data: parsed}}, nil
	}
}

func dispatch(ctx context.Context, reg tools.Registry, name string, args map[string]any) (any, error) {
	if reg == nil {
		return nil, fmt.Errorf("tool registry unavailable")
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	payload, err := reg.Dispatch(ctx, name, raw)
	if err != nil {
		return nil, err
	}
	var parsed any
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return map[string]any{"text": string(payload)}, nil
	}
	if obj, ok := parsed.(map[string]any); ok {
		if okv, has := obj["ok"].(bool); has && !okv {
			if msg, _ := obj["error"].(string); msg != "" {
				return nil, fmt.Errorf("tool %s: %s", name, msg)
			}
		}
	}
	return parsed, nil
}

func mapOutputs(toolName string, m warpp.Manifest, outs []OutMap, parsed any) (map[string]warpp.Value, error) {
	result := map[string]warpp.Value{}
	for _, om := range outs {
		spec, ok := m.Output(om.Port)
		if !ok {
			return nil, fmt.Errorf("tool %s adapter references undeclared output %q", toolName, om.Port)
		}
		var raw any
		if om.Path == "" {
			raw = parsed
		} else {
			v, found := warpp.SelectPath(parsed, om.Path)
			if !found {
				return nil, fmt.Errorf("tool %s result missing %q for port %q (contract violation)", toolName, om.Path, om.Port)
			}
			raw = v
		}
		portType, err := warpp.ParseType(spec.Type)
		if err != nil {
			portType = warpp.Type{Kind: warpp.KindJSON}
		}
		v, err := warpp.CoerceRaw(raw, portType)
		if err != nil {
			return nil, fmt.Errorf("tool %s output %q (contract violation): %w", toolName, om.Port, err)
		}
		result[om.Port] = v
	}
	return result, nil
}
