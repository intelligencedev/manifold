package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"intelligence.dev/internal/playground"
	"intelligence.dev/internal/playground/dataset"
	"intelligence.dev/internal/playground/experiment"
	"intelligence.dev/internal/playground/registry"
)

func (s *Server) handleListPrompts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))

	prompts, err := s.service.ListPrompts(ctx, registry.ListFilter{
		Query:   r.URL.Query().Get("q"),
		Tag:     r.URL.Query().Get("tag"),
		Page:    page,
		PerPage: perPage,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"prompts": prompts})
}

func (s *Server) handleCreatePrompt(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var prompt registry.Prompt
	if err := json.NewDecoder(r.Body).Decode(&prompt); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	if prompt.ID == "" {
		prompt.ID = uuid.NewString()
	}
	created, err := s.service.CreatePrompt(ctx, prompt)
	if err != nil {
		respondError(w, statusFromError(err), err)
		return
	}
	respondJSON(w, http.StatusCreated, created)
}

func (s *Server) handleGetPrompt(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	promptID := r.PathValue("promptID")
	prompt, ok, err := s.service.GetPrompt(ctx, promptID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	if !ok {
		respondError(w, http.StatusNotFound, errors.New("prompt not found"))
		return
	}
	respondJSON(w, http.StatusOK, prompt)
}

func (s *Server) handleCreatePromptVersion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	promptID := r.PathValue("promptID")
	var version registry.PromptVersion
	if err := json.NewDecoder(r.Body).Decode(&version); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	if version.ID == "" {
		version.ID = uuid.NewString()
	}
	created, err := s.service.CreatePromptVersion(ctx, promptID, version)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusCreated, created)
}

func (s *Server) handleListPromptVersions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	promptID := r.PathValue("promptID")
	versions, err := s.service.ListPromptVersions(ctx, promptID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"versions": versions})
}

func (s *Server) handleCreateDataset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var payload struct {
		Dataset dataset.Dataset `json:"dataset"`
		Rows    []dataset.Row   `json:"rows"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	if payload.Dataset.ID == "" {
		payload.Dataset.ID = uuid.NewString()
	}
	created, err := s.service.RegisterDataset(ctx, payload.Dataset, payload.Rows)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusCreated, created)
}

func (s *Server) handleListDatasets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	datasets, err := s.service.ListDatasets(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"datasets": datasets})
}

func (s *Server) handleGetDataset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("datasetID")
	ds, ok, err := s.service.GetDataset(ctx, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	if !ok {
		respondError(w, http.StatusNotFound, errors.New("dataset not found"))
		return
	}
	rows, err := s.service.ListDatasetRows(ctx, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, struct {
		dataset.Dataset
		Rows []dataset.Row `json:"rows"`
	}{Dataset: ds, Rows: rows})
}

func (s *Server) handleUpdateDataset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("datasetID")
	var payload struct {
		Dataset dataset.Dataset `json:"dataset"`
		Rows    []dataset.Row   `json:"rows"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	if payload.Dataset.ID == "" {
		payload.Dataset.ID = id
	}
	if payload.Dataset.ID != id {
		respondError(w, http.StatusBadRequest, errors.New("dataset ID mismatch"))
		return
	}
	updated, err := s.service.UpdateDataset(ctx, payload.Dataset, payload.Rows)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, dataset.ErrDatasetNotFound) {
			status = http.StatusNotFound
		}
		respondError(w, status, err)
		return
	}
	rows, err := s.service.ListDatasetRows(ctx, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, struct {
		dataset.Dataset
		Rows []dataset.Row `json:"rows"`
	}{Dataset: updated, Rows: rows})
}

func (s *Server) handleCreateExperiment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var spec experiment.ExperimentSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	if spec.ID == "" {
		spec.ID = uuid.NewString()
	}
	if spec.CreatedAt.IsZero() {
		spec.CreatedAt = time.Now().UTC()
	}
	created, err := s.service.CreateExperiment(ctx, spec)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusCreated, created)
}

func (s *Server) handleListExperiments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	experiments, err := s.service.ListExperiments(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"experiments": experiments})
}

func (s *Server) handleGetExperiment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	experimentID := r.PathValue("experimentID")
	spec, ok, err := s.service.GetExperiment(ctx, experimentID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	if !ok {
		respondError(w, http.StatusNotFound, errors.New("experiment not found"))
		return
	}
	respondJSON(w, http.StatusOK, spec)
}

func (s *Server) handleStartRun(w http.ResponseWriter, r *http.Request) {
	experimentID := r.PathValue("experimentID")
	run, err := s.service.StartRun(r.Context(), experimentID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, playground.ErrActiveRun) {
			status = http.StatusConflict
		} else if errors.Is(err, playground.ErrUnknownExperiment) {
			status = http.StatusNotFound
		}
		respondError(w, status, err)
		return
	}
	respondJSON(w, http.StatusAccepted, run)
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	experimentID := r.PathValue("experimentID")
	runs, err := s.service.ListRuns(ctx, experimentID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func respondError(w http.ResponseWriter, status int, err error) {
	respondJSON(w, status, map[string]any{"error": err.Error()})
}

func statusFromError(err error) int {
	switch {
	case errors.Is(err, registry.ErrPromptExists):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
