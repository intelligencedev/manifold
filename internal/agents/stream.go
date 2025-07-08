package agents

import (
	"fmt"
	"net/http"
	"strings"

	configpkg "manifold/internal/config"

	"github.com/labstack/echo/v4"
)

// RunReActAgentStreamHandler handles POST /api/agents/react/stream.
func RunReActAgentStreamHandler(cfg *configpkg.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req ReActRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
		}
		req.Objective = strings.TrimSpace(req.Objective)
		if req.Objective == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "objective required"})
		}
		if req.MaxSteps <= 0 {
			// use default max steps from config
			req.MaxSteps = cfg.Completions.ReactAgentConfig.MaxSteps
			if req.MaxSteps <= 0 {
				req.MaxSteps = 100
			}
		}

		// ensure SSE headers
		c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
		c.Response().Header().Set("Cache-Control", "no-cache")
		c.Response().Header().Set("Connection", "keep-alive")
		flusher, ok := c.Response().Writer.(http.Flusher)
		if !ok {
			return c.String(http.StatusInternalServerError, "Streaming unsupported")
		}

		ctx := c.Request().Context()
		if cfg.DBPool == nil {
			return c.String(http.StatusInternalServerError, "database connection pool not initialized")
		}
		poolConn, err := cfg.DBPool.Acquire(ctx)
		if err != nil {
			return c.String(http.StatusInternalServerError, "failed to acquire database connection")
		}
		defer poolConn.Release()
		engine, err := NewEngine(ctx, cfg, poolConn.Conn())
		if err != nil {
			return c.String(http.StatusInternalServerError, err.Error())
		}

		// helper to write one SSE data frame
		write := func(data string) {
			// each line must start with "data: "
			for _, ln := range strings.Split(data, "\n") {
				fmt.Fprintf(c.Response(), "data: %s\n", ln)
			}
			fmt.Fprint(c.Response(), "\n")
			flusher.Flush()
		}

		// Stream agent steps synchronously in the handler
		// Use a dedicated hook function to send each thought immediately as it happens
		session, err := engine.RunSessionWithHook(ctx, cfg, req, func(st AgentStep) {
			// Send ONLY the thought wrapped in <think> tags
			payload := fmt.Sprintf("<think>🤔 %s\n</think>", st.Thought)
			write(payload)

			// Flush to ensure client receives it immediately
			flusher.Flush()
		}, false)
		if err != nil {
			return c.String(http.StatusInternalServerError, err.Error())
		}

		// send final summary/result
		if session != nil && session.Completed {
			finalResult := session.Result
			// remove surrounding quotes if present
			finalResult = strings.TrimPrefix(finalResult, "\"")
			finalResult = strings.TrimSuffix(finalResult, "\"")
			write(finalResult)
		}

		// signal completion and close
		write("[[EOF]]")
		return nil
	}
}
