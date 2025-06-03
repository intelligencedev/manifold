package evolve

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

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
	LLMUsed           string
	PromptUsed        string
	CreationTime      time.Time
	EvaluationDetails map[string]string
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
		ID:                uuid.NewString(),
		Code:              code,
		EvolvableSections: sections,
		SkeletonCode:      skeleton.String(),
		Scores:            make(map[string]float64),
		Generation:        0,
		CreationTime:      time.Now(),
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
	Get(id string) (Program, bool)
	Update(Program) error
	GetAll() []Program
}

// InMemoryDB is a thread safe in-memory ProgramDatabase implementation.
type InMemoryDB struct {
	mu       sync.Mutex
	programs map[string]Program
	order    []string
}

// NewInMemoryDB creates a new database instance.
func NewInMemoryDB() *InMemoryDB {
	return &InMemoryDB{programs: make(map[string]Program)}
}

// Add stores a program.
func (db *InMemoryDB) Add(p Program) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.programs[p.ID] = p
	db.order = append(db.order, p.ID)
	return nil
}

// Get returns a program by id.
func (db *InMemoryDB) Get(id string) (Program, bool) {
	db.mu.Lock()
	defer db.mu.Unlock()
	p, ok := db.programs[id]
	return p, ok
}

// Update stores updated data for a program.
func (db *InMemoryDB) Update(p Program) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if _, ok := db.programs[p.ID]; !ok {
		return fmt.Errorf("program not found")
	}
	db.programs[p.ID] = p
	return nil
}

// GetAll returns all programs in insertion order.
func (db *InMemoryDB) GetAll() []Program {
	db.mu.Lock()
	defer db.mu.Unlock()
	out := make([]Program, 0, len(db.order))
	for _, id := range db.order {
		out = append(out, db.programs[id])
	}
	return out
}

// SampleProgramsFromDatabase selects parents and inspirations.
func SampleProgramsFromDatabase(db ProgramDatabase, numParents, numInspirations int) ([]Program, []Program, error) {
	all := db.GetAll()
	if len(all) == 0 {
		return nil, nil, fmt.Errorf("database empty")
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Scores["score"] > all[j].Scores["score"] })
	parents := []Program{}
	for i := 0; i < numParents && i < len(all); i++ {
		parents = append(parents, all[i])
	}
	inspirations := []Program{}
	for i := numParents; i < len(all) && len(inspirations) < numInspirations; i++ {
		inspirations = append(inspirations, all[i])
	}
	return parents, inspirations, nil
}

// BuildLLMPrompt constructs a basic prompt for the LLM with diversity mechanisms.
func BuildLLMPrompt(parent Program, inspirations []Program, context, meta string, generation int, attemptType string) (string, error) {
	var b strings.Builder
	if context != "" {
		b.WriteString("Context:\n" + context + "\n\n")
	}
	if meta != "" {
		b.WriteString(meta + "\n\n")
	}

	// Add generation info for diversity
	b.WriteString(fmt.Sprintf("Generation: %d\n", generation))
	if attemptType != "" {
		b.WriteString(fmt.Sprintf("Optimization focus: %s\n\n", attemptType))
	}

	b.WriteString(fmt.Sprintf("Parent program (score %.3f):\n", parent.Scores["score"]))
	b.WriteString(parent.Code + "\n\n")
	if len(inspirations) > 0 {
		b.WriteString("Inspirations:\n")
		for _, p := range inspirations {
			b.WriteString(fmt.Sprintf("(score %.3f)\n", p.Scores["score"]))
			b.WriteString(p.Code + "\n---\n")
		}
	}

	// Add diverse optimization strategies based on generation and attempt type
	if attemptType == "radical_refactor" {
		b.WriteString("IMPORTANT: The current approach is stagnating. Please provide a RADICAL REFACTOR with completely different logic.\n\n")
	} else if attemptType == "alternative_algorithm" {
		b.WriteString("IMPORTANT: Try a completely different algorithmic approach (e.g., iterative vs recursive, different data structures).\n\n")
	} else if attemptType == "complete_redesign" {
		b.WriteString("IMPORTANT: Completely redesign the solution from scratch with a fresh perspective.\n\n")
	} else {
		strategies := []string{
			"Focus on performance optimization with memoization or caching",
			"Simplify the algorithm and reduce complexity",
			"Add error handling and edge case management",
			"Optimize for readability and maintainability",
			"Implement iterative instead of recursive approaches",
			"Add input validation and type checking",
			"Optimize memory usage and reduce allocations",
			"Improve variable naming and code structure",
		}
		strategyIndex := generation % len(strategies)
		b.WriteString(fmt.Sprintf("Optimization strategy: %s\n\n", strategies[strategyIndex]))
	}

	b.WriteString("Suggest improvements in diff format using this exact structure:\n\n")
	b.WriteString("<<<<<<< SEARCH\n")
	b.WriteString("code to replace\n")
	b.WriteString("=======\n")
	b.WriteString("improved code\n")
	b.WriteString(">>>>>>> REPLACE\n\n")
	b.WriteString("Please provide specific improvements to the code above:")
	return b.String(), nil
}

// LLMClient defines minimal interface for code generation.
type LLMClient interface {
	Generate(ctx context.Context, prompt string) (string, error)
	ModelName() string
}

// DefaultLLMClient implements LLMClient using internal llm package.
type DefaultLLMClient struct {
	Endpoint string
	APIKey   string
	Model    string
}

// Generate sends the prompt to the completions endpoint.
func (c DefaultLLMClient) Generate(ctx context.Context, prompt string) (string, error) {
	log.Printf("[EVOLVE] LLM Generate called with endpoint: %s, model: %s", c.Endpoint, c.Model)
	log.Printf("[EVOLVE] Prompt length: %d characters", len(prompt))
	log.Printf("[EVOLVE] Prompt preview: %.200s...", prompt)

	msgs := []llm.Message{{Role: "user", Content: prompt}}
	response, err := llm.CallLLM(ctx, c.Endpoint, c.APIKey, c.Model, msgs, 1024, 0.2)

	if err != nil {
		log.Printf("[EVOLVE] LLM Generate failed: %v", err)
		return "", fmt.Errorf("LLM generation failed: %w", err)
	}

	log.Printf("[EVOLVE] LLM Generate success, response length: %d", len(response))
	log.Printf("[EVOLVE] Response preview: %.200s...", response)
	return response, nil
}

func (c DefaultLLMClient) ModelName() string { return c.Model }

var diffRegexp = regexp.MustCompile(`<<<<<<< SEARCH\n(?s)(.*?)\n=======\n(?s)(.*?)\n>>>>>>> REPLACE`)

// ParseLLMDiffOutput parses diff-style output into DiffBlocks.
func ParseLLMDiffOutput(out string) ([]DiffBlock, error) {
	log.Printf("[EVOLVE] Parsing LLM diff output, length: %d", len(out))
	log.Printf("[EVOLVE] Raw LLM output: %s", out)

	matches := diffRegexp.FindAllStringSubmatch(out, -1)
	if len(matches) == 0 {
		log.Printf("[EVOLVE] No diff blocks found in LLM output")
		return nil, fmt.Errorf("no diff blocks found")
	}

	log.Printf("[EVOLVE] Found %d diff blocks", len(matches))
	diffs := make([]DiffBlock, 0, len(matches))
	for i, m := range matches {
		if len(m) >= 3 {
			diff := DiffBlock{Search: strings.TrimSpace(m[1]), Replace: strings.TrimSpace(m[2])}
			diffs = append(diffs, diff)
			log.Printf("[EVOLVE] Diff block %d: search=%d chars, replace=%d chars", i, len(diff.Search), len(diff.Replace))
		}
	}
	return diffs, nil
}

// ApplyDiffsToCode applies search/replace operations sequentially.
func ApplyDiffsToCode(code string, diffs []DiffBlock) (string, error) {
	updated := code
	for _, d := range diffs {
		if strings.Contains(updated, d.Search) {
			updated = strings.Replace(updated, d.Search, d.Replace, 1)
		} else {
			// skip diff if search text not found
			continue
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
func RunAlphaEvolve(ctx context.Context, initialPath, problemContext string, evalFunc func(string) (map[string]float64, error), llmClient LLMClient, db ProgramDatabase, generations int, progress func(int, Program)) (Program, error) {
	log.Printf("[EVOLVE] Starting RunAlphaEvolve with file: %s, generations: %d", initialPath, generations)

	prog, err := ParseInitialProgram(initialPath)
	if err != nil {
		log.Printf("[EVOLVE] Failed to parse initial program: %v", err)
		return Program{}, fmt.Errorf("failed to parse initial program: %w", err)
	}

	log.Printf("[EVOLVE] Parsed initial program: %d evolvable sections", len(prog.EvolvableSections))

	scores, err := EvaluateProgram(prog.Code, evalFunc)
	if err != nil {
		log.Printf("[EVOLVE] Failed to evaluate initial program: %v", err)
		return Program{}, fmt.Errorf("failed to evaluate initial program: %w", err)
	}

	prog.Scores = scores
	prog.LLMUsed = llmClient.ModelName()
	_ = db.Add(prog)
	best := prog
	stagnationCount := 0

	log.Printf("[EVOLVE] Initial program score: %.3f", scores["score"])

	for i := 0; i < generations; i++ {
		log.Printf("[EVOLVE] Starting generation %d/%d (stagnation: %d)", i+1, generations, stagnationCount)

		parents, inspirations, err := SampleProgramsFromDatabase(db, 1, 2)
		if err != nil {
			log.Printf("[EVOLVE] Failed to sample programs: %v", err)
			return Program{}, fmt.Errorf("failed to sample programs: %w", err)
		}
		parent := parents[0]
		log.Printf("[EVOLVE] Using parent program (generation %d, score %.3f)", parent.Generation, parent.Scores["score"])

		// Add diversity to prompt based on generation and stagnation
		attemptType := ""
		if stagnationCount > 2 {
			// High diversity when stagnating
			diverseTypes := []string{"radical_refactor", "alternative_algorithm", "complete_redesign"}
			attemptType = diverseTypes[i%len(diverseTypes)]
		} else if i%3 == 0 {
			attemptType = "performance"
		} else if i%3 == 1 {
			attemptType = "simplicity"
		} else {
			attemptType = "robustness"
		}

		prompt, _ := BuildLLMPrompt(parent, inspirations, problemContext, "", i+1, attemptType)
		log.Printf("[EVOLVE] Built prompt for LLM (%d characters) with focus: %s", len(prompt), attemptType)

		if progress != nil {
			progress(i, best)
		}

		llmOut, err := llmClient.Generate(ctx, prompt)
		if err != nil {
			log.Printf("[EVOLVE] LLM generation failed in generation %d: %v", i+1, err)
			return Program{}, fmt.Errorf("LLM generation failed in generation %d: %w", i+1, err)
		}

		diffs, err := ParseLLMDiffOutput(llmOut)
		if err != nil {
			log.Printf("[EVOLVE] Failed to parse LLM output in generation %d: %v", i+1, err)
			continue
		}

		log.Printf("[EVOLVE] Applying %d diffs to %d evolvable sections", len(diffs), len(parent.EvolvableSections))
		newSections := make([]string, len(parent.EvolvableSections))
		for j, sec := range parent.EvolvableSections {
			updated, _ := ApplyDiffsToCode(sec, diffs)
			newSections[j] = updated
		}
		childCode := ReconstructProgramCode(parent.SkeletonCode, newSections)
		log.Printf("[EVOLVE] Generated child program (%d characters)", len(childCode))

		childScores, err := EvaluateProgram(childCode, evalFunc)
		if err != nil {
			log.Printf("[EVOLVE] Failed to evaluate child program in generation %d: %v", i+1, err)
			continue
		}

		child := Program{
			ID:                uuid.NewString(),
			Code:              childCode,
			EvolvableSections: newSections,
			SkeletonCode:      parent.SkeletonCode,
			Scores:            childScores,
			Generation:        parent.Generation + 1,
			ParentID:          parent.ID,
			LLMUsed:           llmClient.ModelName(),
			PromptUsed:        prompt,
			CreationTime:      time.Now(),
		}
		_ = db.Add(child)

		log.Printf("[EVOLVE] Child program score: %.3f (parent: %.3f)", childScores["score"], parent.Scores["score"])
		if child.Scores["score"] > best.Scores["score"] {
			log.Printf("[EVOLVE] New best program found! Score improved from %.3f to %.3f", best.Scores["score"], child.Scores["score"])
			best = child
			stagnationCount = 0 // Reset stagnation counter
		} else {
			stagnationCount++
			log.Printf("[EVOLVE] No improvement this generation. Stagnation count: %d", stagnationCount)
		}
		if progress != nil {
			progress(i+1, best)
		}
	}

	log.Printf("[EVOLVE] Evolution completed. Final best score: %.3f (generation %d)", best.Scores["score"], best.Generation)
	return best, nil
}
