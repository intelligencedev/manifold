package warpp

import "testing"

func TestEvalGuard_Basics(t *testing.T) {
	A := Attrs{"role": "admin", "active": true, "empty": ""}
	cases := []struct {
		g    string
		want bool
	}{
		{"", true},
		{"true", true},
		{"A.role", true},
		{"A.missing", false},
		{"not A.active", false},
		{"A.role == 'admin'", true},
		{"A.role == \"admin\"", true},
		{"A.role != 'user'", true},
		{"A.empty", false},
		{"A.active == true", true},
	}
	for _, c := range cases {
		if got := EvalGuard(c.g, A); got != c.want {
			t.Fatalf("EvalGuard(%q) = %v; want %v", c.g, got, c.want)
		}
	}
}
