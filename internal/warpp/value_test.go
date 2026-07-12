package warpp

import "testing"

func TestParseType(t *testing.T) {
	cases := []struct {
		in      string
		want    Type
		wantErr bool
	}{
		{"text", Type{Kind: KindText}, false},
		{"number", Type{Kind: KindNumber}, false},
		{"boolean", Type{Kind: KindBoolean}, false},
		{"json", Type{Kind: KindJSON}, false},
		{"file", Type{Kind: KindFile}, false},
		{"list<text>", Type{Kind: KindList, Elem: KindText}, false},
		{"list<json>", Type{Kind: KindList, Elem: KindJSON}, false},
		{"T", Type{Kind: KindVar}, false},
		{"list<T>", Type{Kind: KindList, Elem: KindVar}, false},
		{"list<list<text>>", Type{}, true},
		{"blob", Type{}, true},
		{"list<>", Type{}, true},
		{"", Type{}, true},
	}
	for _, c := range cases {
		got, err := ParseType(c.in)
		if c.wantErr != (err != nil) {
			t.Fatalf("ParseType(%q) err=%v wantErr=%v", c.in, err, c.wantErr)
		}
		if err == nil && got != c.want {
			t.Fatalf("ParseType(%q)=%v want %v", c.in, got, c.want)
		}
		if err == nil && got.String() != c.in {
			t.Fatalf("String() round trip %q != %q", got.String(), c.in)
		}
	}
}

func TestAssignable(t *testing.T) {
	txt := Type{Kind: KindText}
	num := Type{Kind: KindNumber}
	boo := Type{Kind: KindBoolean}
	jsn := Type{Kind: KindJSON}
	fil := Type{Kind: KindFile}
	lstTxt := Type{Kind: KindList, Elem: KindText}
	lstNum := Type{Kind: KindList, Elem: KindNumber}

	if !Assignable(txt, txt) || !Assignable(lstTxt, lstTxt) {
		t.Fatal("identity must be assignable")
	}
	if !Assignable(num, txt) || !Assignable(boo, txt) {
		t.Fatal("number/boolean must coerce to text")
	}
	for _, bad := range [][2]Type{
		{txt, num}, {jsn, txt}, {txt, jsn}, {fil, txt}, {txt, fil},
		{lstNum, lstTxt}, {num, lstNum}, {jsn, boo},
	} {
		if Assignable(bad[0], bad[1]) {
			t.Fatalf("%v -> %v must NOT be assignable", bad[0], bad[1])
		}
	}
}

func TestConforms(t *testing.T) {
	ok := []struct {
		data any
		t    Type
	}{
		{"hi", Type{Kind: KindText}},
		{"path/to.txt", Type{Kind: KindFile}},
		{float64(3), Type{Kind: KindNumber}},
		{int(3), Type{Kind: KindNumber}},
		{true, Type{Kind: KindBoolean}},
		{map[string]any{"a": 1}, Type{Kind: KindJSON}},
		{[]any{"x"}, Type{Kind: KindJSON}},
		{"scalar is valid json value", Type{Kind: KindJSON}},
		{nil, Type{Kind: KindJSON}},
		{[]any{"a", "b"}, Type{Kind: KindList, Elem: KindText}},
		{[]any{}, Type{Kind: KindList, Elem: KindNumber}},
		{[]any{map[string]any{}}, Type{Kind: KindList, Elem: KindJSON}},
	}
	for _, c := range ok {
		if err := Conforms(c.data, c.t); err != nil {
			t.Fatalf("Conforms(%v, %v) unexpected err: %v", c.data, c.t, err)
		}
	}
	bad := []struct {
		data any
		t    Type
	}{
		{3, Type{Kind: KindText}},
		{"x", Type{Kind: KindNumber}},
		{nil, Type{Kind: KindText}},
		{[]any{"a", 1}, Type{Kind: KindList, Elem: KindText}},
		{"notalist", Type{Kind: KindList, Elem: KindText}},
	}
	for _, c := range bad {
		if err := Conforms(c.data, c.t); err == nil {
			t.Fatalf("Conforms(%v, %v) expected error", c.data, c.t)
		}
	}
}

func TestCoerce(t *testing.T) {
	out, err := Coerce(Value{Type: Type{Kind: KindNumber}, Data: float64(4.5)}, Type{Kind: KindText})
	if err != nil || out.Data != "4.5" || out.Type.Kind != KindText {
		t.Fatalf("number->text got %#v err=%v", out, err)
	}
	out, err = Coerce(Value{Type: Type{Kind: KindNumber}, Data: float64(4)}, Type{Kind: KindText})
	if err != nil || out.Data != "4" {
		t.Fatalf("integral float should render without decimals, got %#v", out.Data)
	}
	out, err = Coerce(Value{Type: Type{Kind: KindBoolean}, Data: true}, Type{Kind: KindText})
	if err != nil || out.Data != "true" {
		t.Fatalf("boolean->text got %#v err=%v", out, err)
	}
	if _, err = Coerce(Value{Type: Type{Kind: KindJSON}, Data: map[string]any{}}, Type{Kind: KindText}); err == nil {
		t.Fatal("json->text must not coerce implicitly")
	}
	same, err := Coerce(Value{Type: Type{Kind: KindText}, Data: "x"}, Type{Kind: KindText})
	if err != nil || same.Data != "x" {
		t.Fatal("identity coerce must pass through")
	}
}

func TestInferLiteral(t *testing.T) {
	cases := []struct {
		data any
		want Type
	}{
		{"s", Type{Kind: KindText}},
		{float64(1), Type{Kind: KindNumber}},
		{int(1), Type{Kind: KindNumber}},
		{false, Type{Kind: KindBoolean}},
		{map[string]any{}, Type{Kind: KindJSON}},
		{[]any{"a", "b"}, Type{Kind: KindList, Elem: KindText}},
		{[]any{1.0, 2.0}, Type{Kind: KindList, Elem: KindNumber}},
		{[]any{"a", 1.0}, Type{Kind: KindList, Elem: KindJSON}},
		{[]any{}, Type{Kind: KindList, Elem: KindJSON}},
		{nil, Type{Kind: KindJSON}},
	}
	for _, c := range cases {
		if got := InferLiteral(c.data); got != c.want {
			t.Fatalf("InferLiteral(%v)=%v want %v", c.data, got, c.want)
		}
	}
}
