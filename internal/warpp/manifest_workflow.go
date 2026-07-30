package warpp

import "sort"

// WorkflowManifest derives a node manifest from a saved workflow document so
// the workflow can be used as a node (subflow) or published as a tool. Its
// input ports are the document inputs; its output ports are the document
// outputs, typed via ResolveOutputTypes.
func WorkflowManifest(doc Document, resolve Resolver) (Manifest, []Diagnostic) {
	types, diags := ResolveOutputTypes(doc, resolve)
	m := Manifest{
		Type:        "flow." + doc.ID,
		Title:       doc.Name,
		Category:    "flow",
		Description: doc.Description,
		Inputs:      append([]PortSpec{}, doc.Inputs...),
	}
	names := make([]string, 0, len(doc.Outputs))
	for name := range doc.Outputs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		b := doc.Outputs[name]
		t := Type{Kind: KindJSON}
		if b.HasValue {
			t = InferLiteral(b.Value)
		} else if ref, err := ParseRef(b.From); err == nil {
			if ports, ok := types[ref.Node]; ok {
				if rt, ok := ports[ref.Port]; ok && rt.Kind != "" {
					t = rt
				}
			}
		}
		m.Outputs = append(m.Outputs, PortSpec{Name: name, Type: t.String()})
	}
	return m, diags
}
