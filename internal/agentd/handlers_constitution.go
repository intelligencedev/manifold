package agentd

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (a *app) constitutionVersionsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := a.requireUserID(r); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			versions, err := a.constitutionSvc.List(r.Context())
			if err != nil { writeError(w, http.StatusInternalServerError, err); return }
			writeJSON(w, http.StatusOK, versions)
		case http.MethodPost:
			var body struct { Body string `json:"body"` }
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil { writeError(w, http.StatusBadRequest, err); return }
			version, err := a.constitutionSvc.Create(r.Context(), body.Body, systemUserID)
			if err != nil { writeError(w, http.StatusInternalServerError, err); return }
			writeJSON(w, http.StatusCreated, version)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (a *app) constitutionVersionActionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := a.requireUserID(r); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/constitution/versions/")
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) != 2 || parts[1] != "activate" {
			http.NotFound(w, r)
			return
		}
		version, err := a.constitutionSvc.Activate(r.Context(), parts[0])
		if err != nil { writeError(w, http.StatusInternalServerError, err); return }
		writeJSON(w, http.StatusOK, version)
	}
}
