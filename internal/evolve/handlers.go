package evolve

import (
	"context"
	"log"
	"net/http"
	"os"
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

		evalFunc := func(code string) (map[string]float64, error) {
			// More sophisticated scoring function
			baseScore := 1000.0

			// Penalty for longer code (encourage conciseness)
			lengthPenalty := float64(len(code)) * 0.1

			// Bonus for performance optimizations
			performanceBonus := 0.0

			// Check for memoization patterns (much more flexible detection)
			hasMemo := strings.Contains(code, "memo")
			if hasMemo {
				performanceBonus += 200.0
				log.Printf("[EVOLVE] Detected memoization pattern, +200 bonus")
			}

			// Check for cache patterns
			if strings.Contains(code, "cache") {
				performanceBonus += 200.0
				log.Printf("[EVOLVE] Detected cache pattern, +200 bonus")
			}

			// Check for dynamic programming keywords
			if strings.Contains(code, "dynamic") || strings.Contains(code, "dp") {
				performanceBonus += 150.0
				log.Printf("[EVOLVE] Detected dynamic programming, +150 bonus")
			}

			// Check for iterative patterns (for loops)
			if strings.Contains(code, "for ") && (strings.Contains(code, "range") || strings.Contains(code, "in ")) {
				performanceBonus += 100.0
				log.Printf("[EVOLVE] Detected iterative pattern, +100 bonus")
			}

			// Check for efficient variable swapping (a, b = b, a + b)
			if strings.Contains(code, "a, b =") && strings.Contains(code, "b, a + b") {
				performanceBonus += 150.0
				log.Printf("[EVOLVE] Detected efficient variable swapping, +150 bonus")
			}

			// Penalty for inefficient patterns - count recursive calls
			inefficiencyPenalty := 0.0
			recursiveCount := strings.Count(code, "fibonacci(")
			if recursiveCount > 2 {
				penalty := float64(recursiveCount-2) * 50.0
				inefficiencyPenalty += penalty
				log.Printf("[EVOLVE] Too many recursive calls (%d), +%.1f penalty", recursiveCount, penalty)
			}

			// Major penalty for naive recursion (no memoization)
			if recursiveCount >= 2 && !hasMemo && !strings.Contains(code, "cache") {
				inefficiencyPenalty += 200.0
				log.Printf("[EVOLVE] Naive recursion detected, +200 penalty")
			}

			// Bonus for pre-initialized memo or base cases
			if (strings.Contains(code, "{0:") && strings.Contains(code, "1:")) ||
				(strings.Contains(code, "a, b = 0, 1")) {
				performanceBonus += 50.0
				log.Printf("[EVOLVE] Detected optimized base cases, +50 bonus")
			}

			finalScore := baseScore + performanceBonus - lengthPenalty - inefficiencyPenalty

			log.Printf("[EVOLVE] Scoring code: base=%.1f, perf_bonus=%.1f, length_penalty=%.1f, ineffic_penalty=%.1f, final=%.1f",
				baseScore, performanceBonus, lengthPenalty, inefficiencyPenalty, finalScore)

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
