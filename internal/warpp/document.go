package warpp

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// PortSpec declares one port on a manifest or one workflow-level input.
type PortSpec struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // "text", "list<json>", "T", "list<T>", "dynamic:<port>"
	Required    bool   `json:"required,omitempty"`
	Default     any    `json:"default,omitempty"`
	Variadic    string `json:"variadic,omitempty"` // "", "list", "named"
	Description string `json:"description,omitempty"`
}

// Document is the canonical workflow definition (spec §4).
type Document struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	ProjectID   string             `json:"project_id,omitempty"`
	Inputs      []PortSpec         `json:"inputs,omitempty"`
	Nodes       []Node             `json:"nodes"`
	Outputs     map[string]Binding `json:"outputs,omitempty"`
	Settings    Settings           `json:"settings,omitempty"`
	Publish     Publish            `json:"publish,omitempty"`
}

type Node struct {
	ID     string           `json:"id"`
	Type   string           `json:"type"`
	Inputs map[string]Input `json:"inputs,omitempty"`
	Policy *Policy          `json:"policy,omitempty"`
	Body   *Body            `json:"body,omitempty"`
}

// Body is the nested subgraph of a control.map node.
type Body struct {
	Nodes   []Node             `json:"nodes"`
	Outputs map[string]Binding `json:"outputs"`
}

type Policy struct {
	Timeout string  `json:"timeout,omitempty"`
	Retries Retries `json:"retries,omitempty"`
	OnError string  `json:"on_error,omitempty"` // "", "fail", "skip"
}

type Retries struct {
	Max     int    `json:"max,omitempty"`
	Backoff string `json:"backoff,omitempty"` // "", "fixed", "exponential"
}

type Settings struct {
	MaxConcurrency int    `json:"max_concurrency,omitempty"`
	DefaultPolicy  Policy `json:"default_policy,omitempty"`
}

type Publish struct {
	Tool bool `json:"tool,omitempty"`
}

// Binding wires an input from an upstream port or fixes it to a literal.
// Exactly one of From / Value is set; HasValue records an explicit literal
// so that null/false/0/"" literals are representable.
type Binding struct {
	From     string
	Value    any
	HasValue bool
}

func (b Binding) MarshalJSON() ([]byte, error) {
	if b.HasValue {
		return json.Marshal(map[string]any{"value": b.Value})
	}
	return json.Marshal(map[string]string{"from": b.From})
}

func (b *Binding) UnmarshalJSON(data []byte) error {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("binding must be an object: %w", err)
	}
	_, hasFrom := probe["from"]
	rawValue, hasValue := probe["value"]
	if hasFrom == hasValue {
		return fmt.Errorf("binding must set exactly one of \"from\" or \"value\"")
	}
	for key := range probe {
		if key != "from" && key != "value" {
			return fmt.Errorf("binding has unknown key %q", key)
		}
	}
	if hasFrom {
		if err := json.Unmarshal(probe["from"], &b.From); err != nil {
			return fmt.Errorf("binding \"from\" must be a string: %w", err)
		}
		b.HasValue = false
		b.Value = nil
		return nil
	}
	b.From = ""
	b.HasValue = true
	return json.Unmarshal(rawValue, &b.Value)
}

// Input is the value of one entry in Node.Inputs. Scalar ports use One;
// list-variadic ports use List; named-variadic ports use Named.
type Input struct {
	One   *Binding
	List  []Binding
	Named map[string]Binding
}

func (in Input) MarshalJSON() ([]byte, error) {
	switch {
	case in.One != nil:
		return json.Marshal(in.One)
	case in.List != nil:
		return json.Marshal(in.List)
	case in.Named != nil:
		return json.Marshal(in.Named)
	}
	return nil, fmt.Errorf("empty input binding")
}

func (in *Input) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return fmt.Errorf("empty input binding")
	}
	*in = Input{}
	if trimmed[0] == '[' {
		return json.Unmarshal(trimmed, &in.List)
	}
	if trimmed[0] != '{' {
		return fmt.Errorf("input binding must be an object or array")
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &probe); err != nil {
		return err
	}
	_, hasFrom := probe["from"]
	_, hasValue := probe["value"]
	if hasFrom || hasValue {
		in.One = &Binding{}
		return json.Unmarshal(trimmed, in.One)
	}
	return json.Unmarshal(trimmed, &in.Named)
}

// Canvas is editor-only layout metadata, persisted as a sidecar. It never
// affects execution.
type Canvas struct {
	Nodes  map[string]CanvasNode `json:"nodes,omitempty"`
	Groups []CanvasGroup         `json:"groups,omitempty"`
	Notes  []CanvasNote          `json:"notes,omitempty"`
}

type CanvasNode struct {
	X      float64  `json:"x"`
	Y      float64  `json:"y"`
	Width  *float64 `json:"width,omitempty"`
	Height *float64 `json:"height,omitempty"`
	Label  string   `json:"label,omitempty"`
}

type CanvasGroup struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Color     string `json:"color,omitempty"`
	Collapsed bool   `json:"collapsed,omitempty"`
}

type CanvasNote struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
	Note  string `json:"note,omitempty"`
	Color string `json:"color,omitempty"`
}
