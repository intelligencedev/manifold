package defaultprompt

import _ "embed"

const (
	Name    = "manifold"
	Version = "1.0"
)

// Content is the default playground and specialist system prompt installed at onboarding.
//
//go:embed manifold.txt
var Content string
