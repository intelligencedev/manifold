package patchtool

import (
	"fmt"
)

type parseErrorKind int

const (
	parseErrorInvalidPatch parseErrorKind = iota + 1
	parseErrorInvalidHunk
)

// ParseError mirrors the error structure from the Rust apply-patch implementation.
type ParseError struct {
	Kind    parseErrorKind
	Message string
	Line    int
}

func (e ParseError) Error() string {
	switch e.Kind {
	case parseErrorInvalidPatch:
		return fmt.Sprintf("invalid patch: %s", e.Message)
	case parseErrorInvalidHunk:
		return fmt.Sprintf("invalid hunk at line %d, %s", e.Line, e.Message)
	default:
		return e.Message
	}
}

// UpdateChunk describes a single change chunk inside an update hunk.
type UpdateChunk struct {
	ChangeContext *string
	OldLines      []string
	NewLines      []string
	IsEndOfFile   bool
}

type hunkKind int

const (
	hunkAdd hunkKind = iota + 1
	hunkDelete
	hunkUpdate
)

// Hunk is one logical change entry (add/delete/update) extracted from the patch.
type Hunk struct {
	Kind     hunkKind
	Path     string
	Contents string // for add hunks
	MovePath string // optional for update hunks
	Chunks   []UpdateChunk
}

// Patch is the parsed representation of a patch payload.
type Patch struct {
	Hunks []Hunk
	Raw   string
}

// moveSummary captures a rename performed via an update hunk.
type moveSummary struct {
	From string
	To   string
}
