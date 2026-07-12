// Package warpp implements the typed-port dataflow workflow engine.
package warpp

import (
	"fmt"
	"strconv"
	"strings"
)

type Kind string

const (
	KindText    Kind = "text"
	KindNumber  Kind = "number"
	KindBoolean Kind = "boolean"
	KindJSON    Kind = "json"
	KindFile    Kind = "file"
	KindList    Kind = "list"
	// KindVar is the single type variable allowed in builtin manifests.
	KindVar Kind = "T"
)

// Type is a port type. Elem is set only when Kind == KindList.
type Type struct {
	Kind Kind `json:"kind"`
	Elem Kind `json:"elem,omitempty"`
}

func scalarKind(s string) (Kind, bool) {
	switch Kind(s) {
	case KindText, KindNumber, KindBoolean, KindJSON, KindFile, KindVar:
		return Kind(s), true
	}
	return "", false
}

// ParseType parses "text", "list<json>", "T", "list<T>".
func ParseType(s string) (Type, error) {
	s = strings.TrimSpace(s)
	if inner, ok := strings.CutPrefix(s, "list<"); ok {
		inner, ok = strings.CutSuffix(inner, ">")
		if !ok {
			return Type{}, fmt.Errorf("invalid type %q", s)
		}
		k, ok := scalarKind(inner)
		if !ok {
			return Type{}, fmt.Errorf("invalid list element type %q", inner)
		}
		return Type{Kind: KindList, Elem: k}, nil
	}
	k, ok := scalarKind(s)
	if !ok {
		return Type{}, fmt.Errorf("invalid type %q", s)
	}
	return Type{Kind: k}, nil
}

func (t Type) String() string {
	if t.Kind == KindList {
		return "list<" + string(t.Elem) + ">"
	}
	return string(t.Kind)
}

func (t Type) HasVar() bool {
	return t.Kind == KindVar || (t.Kind == KindList && t.Elem == KindVar)
}

// Assignable reports whether a value of type `from` may be wired into a port
// of type `to`. The implicit coercion table is exactly: number→text,
// boolean→text (spec §5).
func Assignable(from, to Type) bool {
	if from == to {
		return true
	}
	if to.Kind == KindText && (from.Kind == KindNumber || from.Kind == KindBoolean) {
		return true
	}
	return false
}

// Value is a typed runtime value flowing on a wire.
type Value struct {
	Type Type `json:"type"`
	Data any  `json:"data"`
}

func NewValue(t Type, data any) Value { return Value{Type: t, Data: data} }
func NewText(s string) Value          { return Value{Type: Type{Kind: KindText}, Data: s} }

func asNumber(data any) (float64, bool) {
	switch n := data.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// Conforms checks that a raw JSON value structurally matches a concrete type.
func Conforms(data any, t Type) error {
	switch t.Kind {
	case KindText, KindFile:
		if _, ok := data.(string); !ok {
			return fmt.Errorf("expected %s (string), got %T", t.Kind, data)
		}
	case KindNumber:
		if _, ok := asNumber(data); !ok {
			return fmt.Errorf("expected number, got %T", data)
		}
	case KindBoolean:
		if _, ok := data.(bool); !ok {
			return fmt.Errorf("expected boolean, got %T", data)
		}
	case KindJSON:
		return nil // any JSON value, including null
	case KindList:
		items, ok := data.([]any)
		if !ok {
			return fmt.Errorf("expected list, got %T", data)
		}
		for i, item := range items {
			if err := Conforms(item, Type{Kind: t.Elem}); err != nil {
				return fmt.Errorf("item %d: %w", i, err)
			}
		}
	default:
		return fmt.Errorf("cannot conform to type %q", t)
	}
	return nil
}

func stringify(v Value) (string, error) {
	switch v.Type.Kind {
	case KindText, KindFile:
		s, _ := v.Data.(string)
		return s, nil
	case KindNumber:
		n, ok := asNumber(v.Data)
		if !ok {
			return "", fmt.Errorf("number value is %T", v.Data)
		}
		return strconv.FormatFloat(n, 'f', -1, 64), nil
	case KindBoolean:
		b, _ := v.Data.(bool)
		return strconv.FormatBool(b), nil
	}
	return "", fmt.Errorf("cannot stringify %s", v.Type)
}

// Coerce converts v to the target type using only the implicit coercion
// table. Identity passes through after a Conforms check.
func Coerce(v Value, to Type) (Value, error) {
	if v.Type == to {
		if err := Conforms(v.Data, to); err != nil {
			return Value{}, err
		}
		return v, nil
	}
	if !Assignable(v.Type, to) {
		return Value{}, fmt.Errorf("cannot assign %s to %s", v.Type, to)
	}
	s, err := stringify(v)
	if err != nil {
		return Value{}, err
	}
	return Value{Type: to, Data: s}, nil
}

// InferLiteral infers the type of a literal JSON value. Homogeneous arrays
// infer a typed list; mixed or empty arrays infer list<json>; null infers json.
func InferLiteral(data any) Type {
	switch d := data.(type) {
	case string:
		return Type{Kind: KindText}
	case bool:
		return Type{Kind: KindBoolean}
	case float64, int, int64:
		return Type{Kind: KindNumber}
	case []any:
		elem := Kind("")
		for _, item := range d {
			k := InferLiteral(item).Kind
			if k == KindList {
				k = KindJSON
			}
			if elem == "" {
				elem = k
			} else if elem != k {
				elem = KindJSON
			}
		}
		if elem == "" {
			elem = KindJSON
		}
		return Type{Kind: KindList, Elem: elem}
	default:
		return Type{Kind: KindJSON}
	}
}
