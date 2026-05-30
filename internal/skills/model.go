package skills

// Source represents the location precedence of a skill.
// Higher-precedence sources are considered first and win during deduplication.
type Source string

const (
	SourceProject  Source = "project"
	SourceManifold Source = "manifold"
	SourceAgents   Source = "agents"
)

// Metadata contains the minimal information needed to expose a skill to an LLM.
type Metadata struct {
	SkillID          string
	Name             string
	Description      string
	ShortDescription string
	Path             string
	Source           Source
	SkillDir         string
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
