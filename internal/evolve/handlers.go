package evolve

import (
	"net/http"

	"github.com/labstack/echo/v4"

	cfg "manifold/internal/config"
)

// RunRequest defines the JSON payload for the /evolve/run endpoint.
type RunRequest struct {
	FilePath    string `json:"file_path"`
	Context     string `json:"context"`
	Generations int    `json:"generations"`
}

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

		best, err := RunAlphaEvolve(c.Request().Context(), req.FilePath, req.Context, evalFunc, llmClient, db, req.Generations)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, best)
	}
}
