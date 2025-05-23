// manifold/code.go
package main

import (
	"log"
	"net/http"
	"strings"

	agentspkg "manifold/internal/agents"
	configpkg "manifold/internal/config"

	"github.com/labstack/echo/v4"
)

// evaluateCodeHandler is the HTTP handler that dispatches based on language.
func evaluateCodeHandler(c echo.Context) error {
	var req agentspkg.CodeEvalRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Received language: [%s]", req.Language)
		return c.JSON(http.StatusBadRequest, agentspkg.CodeEvalResponse{
			Error: "Invalid request body: " + err.Error(),
		})
	}

	// Trim and lower-case to avoid trailing spaces, etc.
	lang := strings.ToLower(strings.TrimSpace(req.Language))
	log.Printf("Received language: [%s]", lang)

	var (
		resp *agentspkg.CodeEvalResponse
		err  error
	)

	cfg, _ := c.Get("config").(*configpkg.Config)

	switch lang {
	case "python":
		resp, err = agentspkg.RunPythonInContainer(cfg, req.Code, req.Dependencies)
	case "go":
		resp, err = agentspkg.RunGoInContainer(cfg, req.Code, req.Dependencies)
	case "javascript":
		resp, err = agentspkg.RunNodeInContainer(cfg, req.Code, req.Dependencies)
	default:
		return c.JSON(http.StatusBadRequest, agentspkg.CodeEvalResponse{
			Error: "Unsupported language: " + req.Language,
		})
	}

	if err != nil {
		return c.JSON(http.StatusInternalServerError, agentspkg.CodeEvalResponse{
			Error: err.Error(),
		})
	}

	return c.JSON(http.StatusOK, resp)
}
