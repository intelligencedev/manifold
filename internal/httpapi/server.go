package httpapi

import (
	"net/http"

	"manifold/internal/playground"
)

// Server exposes HTTP endpoints for the playground API.
type Server struct {
	service *playground.Service
	mux     *http.ServeMux
}

// NewServer creates the HTTP API server wired to the playground service.
func NewServer(service *playground.Service) *Server {
	s := &Server{service: service, mux: http.NewServeMux()}
	s.registerRoutes()
	return s
}

// ServeHTTP satisfies http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) registerRoutes() {
	// Prompts
	s.mux.HandleFunc("GET /api/v1/playground/prompts", s.handleListPrompts)
	s.mux.HandleFunc("POST /api/v1/playground/prompts", s.handleCreatePrompt)
	s.mux.HandleFunc("GET /api/v1/playground/prompts/{promptID}", s.handleGetPrompt)
	s.mux.HandleFunc("DELETE /api/v1/playground/prompts/{promptID}", s.handleDeletePrompt)
	s.mux.HandleFunc("POST /api/v1/playground/prompts/{promptID}/versions", s.handleCreatePromptVersion)
	s.mux.HandleFunc("GET /api/v1/playground/prompts/{promptID}/versions", s.handleListPromptVersions)

	// Datasets
	s.mux.HandleFunc("GET /api/v1/playground/datasets", s.handleListDatasets)
	s.mux.HandleFunc("GET /api/v1/playground/datasets/{datasetID}", s.handleGetDataset)
	s.mux.HandleFunc("POST /api/v1/playground/datasets", s.handleCreateDataset)
	s.mux.HandleFunc("PUT /api/v1/playground/datasets/{datasetID}", s.handleUpdateDataset)
	s.mux.HandleFunc("DELETE /api/v1/playground/datasets/{datasetID}", s.handleDeleteDataset)
	// Experiments
	s.mux.HandleFunc("GET /api/v1/playground/experiments", s.handleListExperiments)
	s.mux.HandleFunc("POST /api/v1/playground/experiments", s.handleCreateExperiment)
	s.mux.HandleFunc("GET /api/v1/playground/experiments/{experimentID}", s.handleGetExperiment)
	s.mux.HandleFunc("DELETE /api/v1/playground/experiments/{experimentID}", s.handleDeleteExperiment)
	s.mux.HandleFunc("POST /api/v1/playground/experiments/{experimentID}/runs", s.handleStartRun)
	s.mux.HandleFunc("GET /api/v1/playground/experiments/{experimentID}/runs", s.handleListRuns)
	s.mux.HandleFunc("GET /api/v1/playground/runs/{runID}/results", s.handleListRunResults)
}
