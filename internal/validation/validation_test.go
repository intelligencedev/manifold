package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProjectID_ValidAndInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		in    string
		want  string
		errIs error
	}{
		{name: "empty", in: "", want: "", errIs: nil},
		{name: "simple", in: "proj-1", want: "proj-1", errIs: nil},
		{name: "dot", in: ".", want: "", errIs: ErrInvalidProjectID},
		{name: "dotdot", in: "..", want: "", errIs: ErrInvalidProjectID},
		{name: "slash", in: "a/b", want: "", errIs: ErrInvalidProjectID},
		{name: "backslash", in: `a\\b`, want: "", errIs: ErrInvalidProjectID},
		{name: "traversal", in: "../escape", want: "", errIs: ErrInvalidProjectID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ProjectID(tt.in)
			assert.Equal(t, tt.want, got)
			assert.ErrorIs(t, err, tt.errIs)
		})
	}
}

func TestSessionID_ValidAndInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		in    string
		want  string
		errIs error
	}{
		{name: "empty", in: "", want: "", errIs: nil},
		{name: "simple", in: "sess-1", want: "sess-1", errIs: nil},
		{name: "dot", in: ".", want: "", errIs: ErrInvalidSessionID},
		{name: "dotdot", in: "..", want: "", errIs: ErrInvalidSessionID},
		{name: "slash", in: "a/b", want: "", errIs: ErrInvalidSessionID},
		{name: "backslash", in: `a\\b`, want: "", errIs: ErrInvalidSessionID},
		{name: "traversal", in: "../escape", want: "", errIs: ErrInvalidSessionID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SessionID(tt.in)
			assert.Equal(t, tt.want, got)
			assert.ErrorIs(t, err, tt.errIs)
		})
	}
}
