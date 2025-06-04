package evolve

import (
	"context"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	cfg "manifold/internal/config"
)

// RunRequest defines the JSON payload for the /evolve/run endpoint.
type RunRequest struct {
	FilePath    string `json:"file_path"`
	Context     string `json:"context"`
	Generations int    `json:"generations"`
}

type runJob struct {
	ID         string
	Status     string
	Best       Program
	Generation int
	Error      string
	mu         sync.Mutex
}

var (
	jobsMu sync.Mutex
	jobs   = map[string]*runJob{}
)

// RunHandler executes the AlphaEvolve loop with simplified defaults.
func RunHandler(config *cfg.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req RunRequest
		if err := c.Bind(&req); err != nil {
			log.Printf("[EVOLVE] Failed to bind request: %v", err)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
		}
		if req.FilePath == "" {
			log.Printf("[EVOLVE] Missing file_path in request")
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "file_path required"})
		}
		if req.Generations <= 0 {
			req.Generations = 1
		}

		log.Printf("[EVOLVE] Starting evolution run: file=%s, generations=%d, context=%s", req.FilePath, req.Generations, req.Context)

		db := NewInMemoryDB()
		llmClient := DefaultLLMClient{
			Endpoint: config.Completions.DefaultHost,
			APIKey:   config.Completions.APIKey,
			Model:    config.Completions.CompletionsModel,
		}

		log.Printf("[EVOLVE] LLM Client configured: endpoint=%s, model=%s", llmClient.Endpoint, llmClient.Model)

		// evalFunc now accepts fullCode and evolvableSections
		evalFunc := func(fullCode string, evolvableSections []string) (map[string]float64, error) {
			baseScore := 1000.0

			// Penalty for longer code (encourage conciseness) - based on full code length
			lengthPenalty := float64(len(fullCode)) * 0.1
			performanceBonus := 0.0
			inefficiencyPenalty := 0.0

			// Join evolvable sections to analyze the evolved logic specifically
			evolvedLogic := strings.Join(evolvableSections, "\\\\n")

			// --- Checks performed on the evolvedLogic string ---

			// Check for memoization patterns
			hasMemo := false
			// Pattern 1: memo = {} or cache = {}
			if strings.Contains(evolvedLogic, "memo = {") || strings.Contains(evolvedLogic, "memo={") ||
				strings.Contains(evolvedLogic, "cache = {") || strings.Contains(evolvedLogic, "cache={") {
				hasMemo = true
			}
			// Pattern 2: memo: dict or cache: dict (for type hints)
			if strings.Contains(evolvedLogic, "memo: dict") || strings.Contains(evolvedLogic, "cache: dict") {
				hasMemo = true
			}
			// Pattern 3: memo is None: memo = {}
			if strings.Contains(evolvedLogic, "memo is None") && strings.Contains(evolvedLogic, "memo = {}") {
				hasMemo = true
			}

			if hasMemo {
				performanceBonus += 200.0
				log.Printf("[EVOLVE] Detected memoization/cache pattern in evolved logic, +200 bonus")
			}

			// Check for dynamic programming with arrays/lists
			if (strings.Contains(evolvedLogic, "fib = [") || strings.Contains(evolvedLogic, "dp = [") || strings.Contains(evolvedLogic, "fib_array = [") || strings.Contains(evolvedLogic, "dp_table = [")) &&
				(strings.Contains(evolvedLogic, ".append(") || strings.Contains(evolvedLogic, "append(")) {
				performanceBonus += 150.0
				log.Printf("[EVOLVE] Detected array-based dynamic programming in evolved logic, +150 bonus")
			}

			// Check for iterative patterns (for loops)
			hasIteration := strings.Contains(evolvedLogic, "for ") && (strings.Contains(evolvedLogic, "range(") || strings.Contains(evolvedLogic, " in "))

			// Count recursive calls within the evolved logic
			// Attempt to find the function name defined within the block if it's a simple def.
			var funcNameInBlock string
			re := regexp.MustCompile(`def\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)
			matches := re.FindStringSubmatch(evolvedLogic)
			if len(matches) > 1 {
				funcNameInBlock = matches[1]
			}

			localRecursiveCalls := 0
			if funcNameInBlock != "" && funcNameInBlock != "fibonacci" { // If a new function is defined, count its calls
				localRecursiveCalls = strings.Count(evolvedLogic, funcNameInBlock+"(")
				// also check for common helper names if the main func name wasn't found or is different
				if strings.Contains(evolvedLogic, "fib_memo(") {
					localRecursiveCalls += strings.Count(evolvedLogic, "fib_memo(")
				}
				if strings.Contains(evolvedLogic, "helper(") {
					localRecursiveCalls += strings.Count(evolvedLogic, "helper(")
				}

			} else { // Fallback to counting "fibonacci(" if no clear inner function or it's still named fibonacci
				localRecursiveCalls = strings.Count(evolvedLogic, "fibonacci(")
			}

			if hasIteration && localRecursiveCalls == 0 { // Pure iteration gets full bonus
				performanceBonus += 100.0
				log.Printf("[EVOLVE] Detected pure iterative pattern in evolved logic, +100 bonus")
			} else if hasIteration && localRecursiveCalls < 2 { // Iteration with minimal recursion (e.g. setup)
				performanceBonus += 50.0 // Reduced bonus
				log.Printf("[EVOLVE] Detected iterative pattern with minimal recursion in evolved logic, +50 bonus")
			}

			// Check for efficient variable swapping (specific to iterative Fibonacci)
			if strings.Contains(evolvedLogic, "a, b =") && strings.Contains(evolvedLogic, "b, a + b") && hasIteration {
				performanceBonus += 150.0
				log.Printf("[EVOLVE] Detected efficient variable swapping in iterative solution, +150 bonus")
			}

			// Penalty for inefficient patterns within evolved logic
			if localRecursiveCalls > 1 {
				// If memoization is present, recursive calls are okay (part of memoized recursion)
				// So, only penalize if NOT memoized.
				if !hasMemo {
					penalty := float64(localRecursiveCalls-1) * 75.0 // Increased penalty for non-memoized recursion
					inefficiencyPenalty += penalty
					log.Printf("[EVOLVE] Detected %d non-memoized recursive calls in evolved logic, +%.1f penalty", localRecursiveCalls, penalty)
				} else {
					// If memoized, still a small penalty for complexity if too many internal calls are visible
					// This encourages cleaner memoization.
					if localRecursiveCalls > 2 { // e.g. fib_memo(k-1) + fib_memo(k-2) is 2 calls.
						penalty := float64(localRecursiveCalls-2) * 25.0
						inefficiencyPenalty += penalty
						log.Printf("[EVOLVE] Detected %d recursive calls within memoized logic, +%.1f complexity penalty", localRecursiveCalls, penalty)
					}
				}
			}

			// Major penalty for naive recursion (recursive calls in evolved logic without memoization)
			// This is somewhat redundant if the above localRecursiveCalls penalty handles it, but keep for emphasis.
			if localRecursiveCalls > 1 && !hasMemo && !hasIteration { // only if not iterative and not memoized
				inefficiencyPenalty += 100.0
				log.Printf("[EVOLVE] Additional penalty for naive recursion (calls=%d, no memo/iteration), +100 penalty", localRecursiveCalls)
			}

			// Bonus for pre-initialized memo or base cases in evolved logic
			if (strings.Contains(evolvedLogic, "memo = {0:") && strings.Contains(evolvedLogic, "1:")) || // Python dict init
				(strings.Contains(evolvedLogic, "memo={0:") && strings.Contains(evolvedLogic, "1:")) ||
				(strings.Contains(evolvedLogic, "a, b = 0, 1")) { // Iterative base cases
				performanceBonus += 50.0
				log.Printf("[EVOLVE] Detected optimized base cases/initialization in evolved logic, +50 bonus")
			}

			finalScore := baseScore + performanceBonus - lengthPenalty - inefficiencyPenalty

			log.Printf("[EVOLVE] Scoring: full_len=%d, evolved_logic_len=%d. Score: base=%.1f, perf_bonus=%.1f, len_penalty=%.1f, ineffic_penalty=%.1f, final=%.1f",
				len(fullCode), len(evolvedLogic), baseScore, performanceBonus, lengthPenalty, inefficiencyPenalty, finalScore)

			return map[string]float64{"score": finalScore}, nil
		}

		runID := uuid.NewString()
		job := &runJob{ID: runID, Status: "running"}
		jobsMu.Lock()
		jobs[runID] = job
		jobsMu.Unlock()

		log.Printf("[EVOLVE] Created job with ID: %s", runID)

		go func() {
			log.Printf("[EVOLVE] Starting background evolution for job %s", runID)
			// Use background context instead of request context to avoid cancellation
			ctx := context.Background()
			best, err := RunAlphaEvolve(ctx, req.FilePath, req.Context, evalFunc, llmClient, db, req.Generations, func(gen int, prog Program) {
				job.mu.Lock()
				job.Generation = gen
				job.Best = prog
				job.mu.Unlock()
				log.Printf("[EVOLVE] Progress update for job %s: generation %d, score %.3f", runID, gen, prog.Scores["score"])
			})
			job.mu.Lock()
			if err != nil {
				log.Printf("[EVOLVE] Evolution failed for job %s: %v", runID, err)
				job.Status = "error"
				job.Error = err.Error()
			} else {
				log.Printf("[EVOLVE] Evolution completed for job %s: final score %.3f", runID, best.Scores["score"])
				job.Status = "completed"
				job.Best = best
			}
			job.mu.Unlock()
		}()

		return c.JSON(http.StatusOK, map[string]string{"run_id": runID})
	}
}

// StatusHandler returns current status of a run.
func StatusHandler(c echo.Context) error {
	id := c.Param("id")
	log.Printf("[EVOLVE] Status request for job ID: %s", id)

	jobsMu.Lock()
	job, ok := jobs[id]
	jobsMu.Unlock()
	if !ok {
		log.Printf("[EVOLVE] Job not found: %s", id)
		return c.JSON(http.StatusNotFound, map[string]string{"error": "run not found"})
	}

	job.mu.Lock()
	resp := map[string]interface{}{
		"status":     job.Status,
		"generation": job.Generation,
		"best_score": job.Best.Scores["score"],
	}

	// Include error information if the job failed
	if job.Status == "error" && job.Error != "" {
		resp["error"] = job.Error
		log.Printf("[EVOLVE] Returning error status for job %s: %s", id, job.Error)
	}

	job.mu.Unlock()
	log.Printf("[EVOLVE] Status response for job %s: status=%s, generation=%d, score=%.3f",
		id, job.Status, job.Generation, job.Best.Scores["score"])

	return c.JSON(http.StatusOK, resp)
}

// ResultHandler returns the best program for a completed run.
func ResultHandler(c echo.Context) error {
	id := c.Param("id")
	jobsMu.Lock()
	job, ok := jobs[id]
	jobsMu.Unlock()
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "run not found"})
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	if job.Status != "completed" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "run not completed"})
	}
	return c.JSON(http.StatusOK, job.Best)
}

// SaveRequest defines the JSON payload for saving evolved code
type SaveRequest struct {
	FilePath string `json:"file_path"`
}

// SaveHandler saves the best evolved code to a file
func SaveHandler(c echo.Context) error {
	id := c.Param("id")

	var req SaveRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("[EVOLVE] Failed to parse save request: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.FilePath == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "file_path required"})
	}

	log.Printf("[EVOLVE] Save request for job %s to file: %s", id, req.FilePath)

	jobsMu.Lock()
	job, ok := jobs[id]
	jobsMu.Unlock()
	if !ok {
		log.Printf("[EVOLVE] Job not found for save: %s", id)
		return c.JSON(http.StatusNotFound, map[string]string{"error": "run not found"})
	}

	job.mu.Lock()
	defer job.mu.Unlock()

	if job.Status != "completed" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "run not completed"})
	}

	// Write the evolved code to the specified file
	err := os.WriteFile(req.FilePath, []byte(job.Best.Code), 0644)
	if err != nil {
		log.Printf("[EVOLVE] Failed to save evolved code to %s: %v", req.FilePath, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save file"})
	}

	log.Printf("[EVOLVE] Successfully saved evolved code to: %s", req.FilePath)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":    "Evolved code saved successfully",
		"file_path":  req.FilePath,
		"score":      job.Best.Scores["score"],
		"generation": job.Best.Generation,
	})
}
