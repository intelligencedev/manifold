package skills

// Scope represents the location precedence of a skill.
// Higher-precedence scopes are considered first and win during deduplication.
type Scope string

const (
	ScopeRepo  Scope = "repo"
	ScopeUser  Scope = "user"
	ScopeAdmin Scope = "admin"
)

// Metadata contains the minimal information needed to expose a skill to an LLM.
type Metadata struct {
	Name             string
	Description      string
	ShortDescription string
	Path             string
	Scope            Scope
}

// Error captures a load or parse failure for a single skill file.
type Error struct {
	Path    string
	Message string
}

// LoadOutcome is the aggregated result of a skills load operation.
type LoadOutcome struct {
	Skills []Metadata
	Errors []Error
}
