package evolve

import (
	"net/http"
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
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
		}
		if req.FilePath == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "file_path required"})
		}
		if req.Generations <= 0 {
			req.Generations = 1
		}

		db := NewInMemoryDB()
		llmClient := DefaultLLMClient{
			Endpoint: config.Completions.DefaultHost,
			APIKey:   config.Completions.APIKey,
			Model:    config.Completions.CompletionsModel,
		}
		evalFunc := func(code string) (map[string]float64, error) {
			return map[string]float64{"score": float64(len(code))}, nil
		}

		runID := uuid.NewString()
		job := &runJob{ID: runID, Status: "running"}
		jobsMu.Lock()
		jobs[runID] = job
		jobsMu.Unlock()

		go func() {
			best, err := RunAlphaEvolve(c.Request().Context(), req.FilePath, req.Context, evalFunc, llmClient, db, req.Generations, func(gen int, prog Program) {
				job.mu.Lock()
				job.Generation = gen
				job.Best = prog
				job.mu.Unlock()
			})
			job.mu.Lock()
			if err != nil {
				job.Status = "error"
				job.Error = err.Error()
			} else {
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
	jobsMu.Lock()
	job, ok := jobs[id]
	jobsMu.Unlock()
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "run not found"})
	}
	job.mu.Lock()
	resp := map[string]interface{}{
		"status":     job.Status,
		"generation": job.Generation,
		"best_score": job.Best.Scores["score"],
	}
	job.mu.Unlock()
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
