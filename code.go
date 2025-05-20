// manifold/code.go
package main

import (
	"log"
	"net/http"
	"strings"

	configpkg "manifold/internal/config"
	codeeval "manifold/internal/tools"

	"github.com/labstack/echo/v4"
)

// evaluateCodeHandler is the HTTP handler that dispatches based on language.
func evaluateCodeHandler(c echo.Context) error {
	var req codeeval.CodeEvalRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Received language: [%s]", req.Language)
		return c.JSON(http.StatusBadRequest, codeeval.CodeEvalResponse{
			Error: "Invalid request body: " + err.Error(),
		})
	}

	// Trim and lower-case to avoid trailing spaces, etc.
	lang := strings.ToLower(strings.TrimSpace(req.Language))
	log.Printf("Received language: [%s]", lang)

	var (
		resp *codeeval.CodeEvalResponse
		err  error
	)

	cfg, _ := c.Get("config").(*configpkg.Config)

	switch lang {
	case "python":
		resp, err = codeeval.RunPython(cfg, req.Code, req.Dependencies)
	case "go":
		resp, err = codeeval.RunGo(cfg, req.Code, req.Dependencies)
	case "javascript":
		resp, err = codeeval.RunNode(cfg, req.Code, req.Dependencies)
	default:
		return c.JSON(http.StatusBadRequest, codeeval.CodeEvalResponse{
			Error: "Unsupported language: " + req.Language,
		})
	}

	if err != nil {
		return c.JSON(http.StatusInternalServerError, codeeval.CodeEvalResponse{
			Error: err.Error(),
		})
	}

	return c.JSON(http.StatusOK, resp)
}
