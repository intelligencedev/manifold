package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"manifold/internal/mcp"

	"github.com/labstack/echo/v4"
)

// runReActAgentStreamHandler handles POST /api/agents/react/stream.
func runReActAgentStreamHandler(cfg *Config) echo.HandlerFunc {
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
		conn, err := Connect(ctx, cfg.Database.ConnectionString)
		if err != nil {
			return c.String(http.StatusInternalServerError, err.Error())
		}
		defer conn.Close(ctx)

		mgr, err := mcp.NewManager(ctx, "config.yaml")
		if err != nil {
			return c.String(http.StatusInternalServerError, err.Error())
		}

		engine := &AgentEngine{
			Config:     cfg,
			DB:         conn,
			HTTPClient: &http.Client{Timeout: 180 * time.Second},
			mcpMgr:     mgr,
			mcpTools:   make(map[string]ToolInfo),
		}

		// Configure memory engine based on config
		if cfg.AgenticMemory.Enabled {
			engine.MemoryEngine = NewAgenticEngine(conn)
			if err := engine.MemoryEngine.EnsureAgenticMemoryTable(ctx, cfg.Embeddings.Dimensions); err != nil {
				return c.String(http.StatusInternalServerError, err.Error())
			}
		} else {
			// Use the no-op implementation when agentic memory is disabled
			engine.MemoryEngine = &NilMemoryEngine{}
		}

		_ = engine.discoverMCPTools(ctx)

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
		session, err := engine.RunSessionWithHook(ctx, req, func(st AgentStep) {
			// Send ONLY the thought wrapped in <think> tags
			payload := fmt.Sprintf("<think>%s</think>", st.Thought)
			write(payload)

			// Flush to ensure client receives it immediately
			flusher.Flush()
		})
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
