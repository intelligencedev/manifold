package patchtool

import (
	"strings"
)

const (
	beginMarker         = "*** Begin Patch"
	endMarker           = "*** End Patch"
	addFileMarker       = "*** Add File: "
	deleteFileMarker    = "*** Delete File: "
	updateFileMarker    = "*** Update File: "
	moveToMarker        = "*** Move to: "
	eofMarker           = "*** End of File"
	changeContextMarker = "@@ "
	emptyContextMarker  = "@@"
)

// ParsePatch converts the textual patch payload into structured hunks.
func ParsePatch(patch string) (*Patch, error) {
	trimmed := strings.TrimSpace(patch)
	if trimmed == "" {
		return nil, ParseError{Kind: parseErrorInvalidPatch, Message: "patch is empty"}
	}

	lines := splitLines(trimmed)
	body, err := selectPatchBody(lines)
	if err != nil {
		return nil, err
	}

	if len(body) == 0 {
		return &Patch{Hunks: nil, Raw: strings.Join(lines, "\n")}, nil
	}

	hunks := make([]Hunk, 0, 4)
	remaining := body
	lineNumber := 2 // first hunk line (after *** Begin Patch)

	for len(remaining) > 0 {
		// Skip blank lines between hunks if present.
		if isBlankLine(remaining[0]) {
			remaining = remaining[1:]
			lineNumber++
			continue
		}

		h, consumed, parseErr := parseOneHunk(remaining, lineNumber)
		if parseErr != nil {
			return nil, parseErr
		}
		hunks = append(hunks, h)
		remaining = remaining[consumed:]
		lineNumber += consumed
	}

	return &Patch{Hunks: hunks, Raw: strings.Join(lines, "\n")}, nil
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\n")
	return parts
}

func selectPatchBody(lines []string) ([]string, error) {
	if len(lines) < 2 {
		return nil, ParseError{Kind: parseErrorInvalidPatch, Message: "missing patch markers"}
	}

	if lines[0] == beginMarker && lines[len(lines)-1] == endMarker {
		return lines[1 : len(lines)-1], nil
	}

	// Attempt lenient mode by peeling heredoc wrappers.
	if len(lines) >= 4 && isHeredocStart(lines[0]) && strings.HasSuffix(lines[len(lines)-1], "EOF") {
		inner := lines[1 : len(lines)-1]
		return selectPatchBody(inner)
	}

	if lines[0] != beginMarker {
		return nil, ParseError{Kind: parseErrorInvalidPatch, Message: "The first line of the patch must be '*** Begin Patch'"}
	}

	return nil, ParseError{Kind: parseErrorInvalidPatch, Message: "The last line of the patch must be '*** End Patch'"}
}

func isHeredocStart(line string) bool {
	switch strings.TrimSpace(line) {
	case "<<EOF", "<<'EOF'", "<<\"EOF\"":
		return true
	default:
		return false
	}
}

func parseOneHunk(lines []string, baseLine int) (Hunk, int, error) {
	header := strings.TrimSpace(lines[0])
	if strings.HasPrefix(header, addFileMarker) {
		path := strings.TrimPrefix(header, addFileMarker)
		contents, consumed, err := parseAddFile(lines[1:])
		if err != nil {
			return Hunk{}, 0, ParseError{Kind: parseErrorInvalidHunk, Line: baseLine, Message: err.Error()}
		}
		return Hunk{Kind: hunkAdd, Path: path, Contents: contents}, consumed + 1, nil
	}
	if strings.HasPrefix(header, deleteFileMarker) {
		path := strings.TrimPrefix(header, deleteFileMarker)
		return Hunk{Kind: hunkDelete, Path: path}, 1, nil
	}
	if strings.HasPrefix(header, updateFileMarker) {
		path := strings.TrimPrefix(header, updateFileMarker)
		h, consumed, err := parseUpdateFile(path, lines[1:], baseLine+1)
		if err != nil {
			return Hunk{}, 0, err
		}
		return h, consumed + 1, nil
	}

	return Hunk{}, 0, ParseError{
		Kind:    parseErrorInvalidHunk,
		Line:    baseLine,
		Message: "'" + header + "' is not a valid hunk header. Valid hunk headers: '*** Add File: {path}', '*** Delete File: {path}', '*** Update File: {path}'",
	}
}

func parseAddFile(lines []string) (string, int, error) {
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "+") {
		return "", 0, parseAddFileError("Add file hunk must contain at least one '+' line")
	}
	var builder strings.Builder
	consumed := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "+") {
			builder.WriteString(line[1:])
			builder.WriteByte('\n')
			consumed++
			continue
		}
		break
	}
	return builder.String(), consumed, nil
}

type parseAddFileError string

func (e parseAddFileError) Error() string { return string(e) }

func parseUpdateFile(path string, lines []string, baseLine int) (Hunk, int, error) {
	remaining := lines
	consumed := 0
	movePath := ""

	if len(remaining) > 0 && strings.HasPrefix(strings.TrimSpace(remaining[0]), moveToMarker) {
		movePath = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(remaining[0]), moveToMarker))
		remaining = remaining[1:]
		consumed++
	}

	if len(remaining) == 0 {
		return Hunk{}, 0, ParseError{Kind: parseErrorInvalidHunk, Line: baseLine, Message: "Update file hunk for path '" + path + "' is empty"}
	}

	chunks := make([]UpdateChunk, 0, 4)
	firstChunk := true
	currentLine := baseLine + consumed

	for len(remaining) > 0 {
		if isBlankLine(remaining[0]) {
			remaining = remaining[1:]
			consumed++
			currentLine++
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(remaining[0]), "***") {
			break
		}
		chunk, used, err := parseUpdateChunk(remaining, currentLine, firstChunk)
		if err != nil {
			return Hunk{}, 0, err
		}
		chunks = append(chunks, chunk)
		remaining = remaining[used:]
		consumed += used
		currentLine += used
		firstChunk = false
	}

	if len(chunks) == 0 {
		return Hunk{}, 0, ParseError{Kind: parseErrorInvalidHunk, Line: baseLine, Message: "Update file hunk for path '" + path + "' is empty"}
	}

	return Hunk{Kind: hunkUpdate, Path: path, MovePath: movePath, Chunks: chunks}, consumed, nil
}

func parseUpdateChunk(lines []string, baseLine int, allowMissingContext bool) (UpdateChunk, int, error) {
	if len(lines) == 0 {
		return UpdateChunk{}, 0, ParseError{Kind: parseErrorInvalidHunk, Line: baseLine, Message: "Update hunk does not contain any lines"}
	}

	start := 0
	var context *string
	first := lines[0]
	switch {
	case first == emptyContextMarker:
		start = 1
	case strings.HasPrefix(first, changeContextMarker):
		ctx := first[len(changeContextMarker):]
		context = &ctx
		start = 1
	default:
		if !allowMissingContext {
			return UpdateChunk{}, 0, ParseError{Kind: parseErrorInvalidHunk, Line: baseLine, Message: "Expected update hunk to start with a @@ context marker, got: '" + first + "'"}
		}
	}

	if start >= len(lines) {
		return UpdateChunk{}, 0, ParseError{Kind: parseErrorInvalidHunk, Line: baseLine + 1, Message: "Update hunk does not contain any lines"}
	}

	chunk := UpdateChunk{ChangeContext: context, OldLines: make([]string, 0, 8), NewLines: make([]string, 0, 8)}
	consumed := start

	for _, line := range lines[start:] {
		trimmed := line
		switch {
		case trimmed == eofMarker:
			if consumed == start {
				return UpdateChunk{}, 0, ParseError{Kind: parseErrorInvalidHunk, Line: baseLine + 1, Message: "Update hunk does not contain any lines"}
			}
			chunk.IsEndOfFile = true
			consumed++
			return chunk, consumed, nil
		case strings.HasPrefix(trimmed, " "):
			chunk.OldLines = append(chunk.OldLines, trimmed[1:])
			chunk.NewLines = append(chunk.NewLines, trimmed[1:])
		case strings.HasPrefix(trimmed, "+"):
			chunk.NewLines = append(chunk.NewLines, trimmed[1:])
		case strings.HasPrefix(trimmed, "-"):
			chunk.OldLines = append(chunk.OldLines, trimmed[1:])
		case trimmed == "":
			chunk.OldLines = append(chunk.OldLines, "")
			chunk.NewLines = append(chunk.NewLines, "")
		default:
			if consumed == start {
				return UpdateChunk{}, 0, ParseError{Kind: parseErrorInvalidHunk, Line: baseLine + 1, Message: "Unexpected line found in update hunk: '" + trimmed + "'. Every line should start with ' ' (context line), '+' (added line), or '-' (removed line)"}
			}
			return chunk, consumed, nil
		}
		consumed++
	}

	return chunk, consumed, nil
}

func isBlankLine(line string) bool {
	return strings.TrimSpace(line) == ""
}
