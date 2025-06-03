package evolve

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	llm "manifold/internal/llm"
)

// Program represents a single code solution in the population.
type Program struct {
	ID                string
	Code              string
	EvolvableSections []string
	SkeletonCode      string
	Scores            map[string]float64
	Generation        int
	ParentID          string
}

// DiffBlock represents a single change proposed by the LLM.
type DiffBlock struct {
	Search  string
	Replace string
}

// ParseInitialProgram reads a file and extracts evolvable blocks marked with
// "# EVOLVE-BLOCK-START" and "# EVOLVE-BLOCK-END".
func ParseInitialProgram(filePath string) (Program, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return Program{}, err
	}
	code := string(data)
	var skeleton strings.Builder
	var sections []string
	scanner := bufio.NewScanner(strings.NewReader(code))
	inBlock := false
	var block strings.Builder
	blockIndex := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "# EVOLVE-BLOCK-START") {
			inBlock = true
			skeleton.WriteString(fmt.Sprintf("{{EVOLVE_BLOCK_%d}}\n", blockIndex))
			continue
		}
		if strings.Contains(line, "# EVOLVE-BLOCK-END") {
			inBlock = false
			sections = append(sections, block.String())
			block.Reset()
			blockIndex++
			continue
		}
		if inBlock {
			block.WriteString(line + "\n")
		} else {
			skeleton.WriteString(line + "\n")
		}
	}
	if err := scanner.Err(); err != nil {
		return Program{}, err
	}
	return Program{
		ID:                "initial-0",
		Code:              code,
		EvolvableSections: sections,
		SkeletonCode:      skeleton.String(),
		Scores:            make(map[string]float64),
		Generation:        0,
	}, nil
}

// ReconstructProgramCode combines the skeleton with evolvable sections back into a full program.
func ReconstructProgramCode(skeleton string, evolvableSections []string) string {
	result := skeleton
	for i, section := range evolvableSections {
		placeholder := fmt.Sprintf("{{EVOLVE_BLOCK_%d}}", i)
		result = strings.ReplaceAll(result, placeholder, section)
	}
	return result
}

// EvaluateProgram uses the provided evaluation function.
func EvaluateProgram(code string, evalFunc func(string) (map[string]float64, error)) (map[string]float64, error) {
	if evalFunc == nil {
		return map[string]float64{"score": float64(len(code))}, nil
	}
	return evalFunc(code)
}

// ProgramDatabase is a minimal in-memory program storage.
type ProgramDatabase interface {
	Add(Program) error
	GetAll() []Program
}

// InMemoryDB is a thread safe in-memory ProgramDatabase implementation.
type InMemoryDB struct {
	mu       sync.Mutex
	programs []Program
}

// NewInMemoryDB creates a new database instance.
func NewInMemoryDB() *InMemoryDB { return &InMemoryDB{} }

// Add stores a program.
func (db *InMemoryDB) Add(p Program) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.programs = append(db.programs, p)
	return nil
}

// GetAll returns all programs.
func (db *InMemoryDB) GetAll() []Program {
	db.mu.Lock()
	defer db.mu.Unlock()
	out := make([]Program, len(db.programs))
	copy(out, db.programs)
	return out
}

// SampleProgramsFromDatabase selects parents and inspirations.
func SampleProgramsFromDatabase(db ProgramDatabase, numParents, numInspirations int) ([]Program, []Program, error) {
	all := db.GetAll()
	if len(all) == 0 {
		return nil, nil, fmt.Errorf("database empty")
	}
	parent := all[len(all)-1]
	inspirations := []Program{}
	for i := len(all) - 2; i >= 0 && len(inspirations) < numInspirations; i-- {
		inspirations = append(inspirations, all[i])
	}
	return []Program{parent}, inspirations, nil
}

// BuildLLMPrompt constructs a basic prompt for the LLM.
func BuildLLMPrompt(parent Program, inspirations []Program, context, meta string) (string, error) {
	var b strings.Builder
	if context != "" {
		b.WriteString("Context:\n" + context + "\n\n")
	}
	if meta != "" {
		b.WriteString(meta + "\n\n")
	}
	b.WriteString("Parent program:\n")
	b.WriteString(parent.Code + "\n\n")
	if len(inspirations) > 0 {
		b.WriteString("Inspirations:\n")
		for _, p := range inspirations {
			b.WriteString(p.Code + "\n---\n")
		}
	}
	b.WriteString("Suggest improvements in diff format:")
	return b.String(), nil
}

// LLMClient defines minimal interface for code generation.
type LLMClient interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

// DefaultLLMClient implements LLMClient using internal llm package.
type DefaultLLMClient struct {
	Endpoint string
	APIKey   string
	Model    string
}

// Generate sends the prompt to the completions endpoint.
func (c DefaultLLMClient) Generate(ctx context.Context, prompt string) (string, error) {
	msgs := []llm.Message{{Role: "user", Content: prompt}}
	return llm.CallLLM(ctx, c.Endpoint, c.APIKey, c.Model, msgs, 1024, 0.2)
}

var diffRegexp = regexp.MustCompile(`<<<<<<< SEARCH\n(?s)(.*?)\n=======\n(?s)(.*?)\n>>>>>>> REPLACE`)

// ParseLLMDiffOutput parses diff-style output into DiffBlocks.
func ParseLLMDiffOutput(out string) ([]DiffBlock, error) {
	matches := diffRegexp.FindAllStringSubmatch(out, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no diff blocks found")
	}
	diffs := make([]DiffBlock, 0, len(matches))
	for _, m := range matches {
		diffs = append(diffs, DiffBlock{Search: strings.TrimSpace(m[1]), Replace: strings.TrimSpace(m[2])})
	}
	return diffs, nil
}

// ApplyDiffsToCode applies search/replace operations sequentially.
func ApplyDiffsToCode(code string, diffs []DiffBlock) (string, error) {
	updated := code
	for _, d := range diffs {
		if strings.Contains(updated, d.Search) {
			updated = strings.Replace(updated, d.Search, d.Replace, 1)
		}
	}
	return updated, nil
}

// SelectBestProgram returns the program with highest score for metric "score".
func SelectBestProgram(db ProgramDatabase, metric string) (Program, error) {
	all := db.GetAll()
	if len(all) == 0 {
		return Program{}, fmt.Errorf("database empty")
	}
	best := all[0]
	for _, p := range all {
		if p.Scores[metric] > best.Scores[metric] {
			best = p
		}
	}
	return best, nil
}

// RunAlphaEvolve executes a simplified evolutionary loop.
func RunAlphaEvolve(ctx context.Context, initialPath, problemContext string, evalFunc func(string) (map[string]float64, error), llmClient LLMClient, db ProgramDatabase, generations int) (Program, error) {
	prog, err := ParseInitialProgram(initialPath)
	if err != nil {
		return Program{}, err
	}
	scores, err := EvaluateProgram(prog.Code, evalFunc)
	if err != nil {
		return Program{}, err
	}
	prog.Scores = scores
	_ = db.Add(prog)
	best := prog

	for i := 0; i < generations; i++ {
		parents, inspirations, err := SampleProgramsFromDatabase(db, 1, 2)
		if err != nil {
			return Program{}, err
		}
		parent := parents[0]
		prompt, _ := BuildLLMPrompt(parent, inspirations, problemContext, "")
		llmOut, err := llmClient.Generate(ctx, prompt)
		if err != nil {
			return Program{}, err
		}
		diffs, err := ParseLLMDiffOutput(llmOut)
		if err != nil {
			continue
		}
		newSections := make([]string, len(parent.EvolvableSections))
		for j, sec := range parent.EvolvableSections {
			updated, _ := ApplyDiffsToCode(sec, diffs)
			newSections[j] = updated
		}
		childCode := ReconstructProgramCode(parent.SkeletonCode, newSections)
		childScores, err := EvaluateProgram(childCode, evalFunc)
		if err != nil {
			continue
		}
		child := Program{
			ID:                fmt.Sprintf("gen%d", i+1),
			Code:              childCode,
			EvolvableSections: newSections,
			SkeletonCode:      parent.SkeletonCode,
			Scores:            childScores,
			Generation:        parent.Generation + 1,
			ParentID:          parent.ID,
		}
		_ = db.Add(child)
		if child.Scores["score"] > best.Scores["score"] {
			best = child
		}
	}
	return best, nil
}
