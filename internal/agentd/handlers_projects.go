package agentd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"manifold/internal/auth"
	persist "manifold/internal/persistence"
)

func (a *app) projectsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.projectsCORS(w, r, "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		userID, ok, err := a.resolveProjectsUser(r)
		if !ok || err != nil {
			if errors.Is(err, persist.ErrForbidden) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			list, err := a.projectsService.ListProjects(r.Context(), userID)
			if err != nil {
				log.Error().Err(err).Msg("list_projects")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			out := make([]map[string]any, 0, len(list))
			for _, p := range list {
				out = append(out, map[string]any{
					"id":        p.ID,
					"name":      p.Name,
					"createdAt": p.CreatedAt,
					"updatedAt": p.UpdatedAt,
					"sizeBytes": p.Bytes,
					"files":     p.FileCount,
				})
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"projects": out})
		case http.MethodPost:
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()
			var in struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil && !errors.Is(err, io.EOF) {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			p, err := a.projectsService.CreateProject(r.Context(), userID, in.Name)
			if err != nil {
				log.Error().Err(err).Msg("create_project")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(p)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (a *app) projectDetailHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.projectsCORS(w, r, "GET, POST, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		userID, ok, err := a.resolveProjectsUser(r)
		if !ok || err != nil {
			if errors.Is(err, persist.ErrForbidden) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/projects/")
		path = strings.Trim(path, "/")
		if path == "" {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(path, "/")
		projectID := parts[0]
		if len(parts) == 1 {
			switch r.Method {
			case http.MethodDelete:
				if err := a.projectsService.DeleteProject(r.Context(), userID, projectID); err != nil {
					log.Error().Err(err).Str("project", projectID).Msg("delete_project")
					http.Error(w, "internal server error", http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			case http.MethodGet:
				entries, err := a.projectsService.ListTree(r.Context(), userID, projectID, ".")
				if err != nil {
					log.Error().Err(err).Str("project", projectID).Msg("list_tree_root")
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				rows := make([]map[string]any, 0, len(entries))
				for _, e := range entries {
					rows = append(rows, map[string]any{
						"name":      e.Name,
						"path":      e.Path,
						"isDir":     e.Type == "dir",
						"sizeBytes": e.Size,
						"modTime":   e.ModTime,
					})
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"entries": rows})
				return
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
		}

		if len(parts) < 2 {
			http.NotFound(w, r)
			return
		}
		switch parts[1] {
		case "tree":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			p := r.URL.Query().Get("path")
			entries, err := a.projectsService.ListTree(r.Context(), userID, projectID, p)
			if err != nil {
				log.Error().Err(err).Str("project", projectID).Str("path", p).Msg("list_tree")
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			rows := make([]map[string]any, 0, len(entries))
			for _, e := range entries {
				rows = append(rows, map[string]any{
					"name":      e.Name,
					"path":      e.Path,
					"isDir":     e.Type == "dir",
					"sizeBytes": e.Size,
					"modTime":   e.ModTime,
				})
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"entries": rows})
			return
		case "files":
			switch r.Method {
			case http.MethodGet:
				p := r.URL.Query().Get("path")
				if p == "" {
					http.Error(w, "missing path", http.StatusBadRequest)
					return
				}
				rc, err := a.projectsService.ReadFile(r.Context(), userID, projectID, p)
				if err != nil {
					log.Error().Err(err).Str("project", projectID).Str("path", p).Msg("read_file")
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				defer rc.Close()
				var sniff [512]byte
				n, _ := io.ReadFull(rc, sniff[:])
				ct := "application/octet-stream"
				if ext := filepath.Ext(p); ext != "" {
					if mt := mime.TypeByExtension(ext); mt != "" {
						ct = mt
					}
				}
				if ct == "application/octet-stream" && n > 0 {
					ct = http.DetectContentType(sniff[:n])
				}
				w.Header().Set("Content-Type", ct)
				if n > 0 {
					_, _ = w.Write(sniff[:n])
				}
				if _, err := io.Copy(w, rc); err != nil {
					log.Error().Err(err).Str("project", projectID).Str("path", p).Msg("stream_file")
				}
			case http.MethodPost:
				p := r.URL.Query().Get("path")
				name := r.URL.Query().Get("name")
				ct := r.Header.Get("Content-Type")
				if strings.HasPrefix(strings.ToLower(ct), "multipart/") {
					if err := r.ParseMultipartForm(64 << 20); err != nil {
						http.Error(w, "bad request", http.StatusBadRequest)
						return
					}
					file, fh, err := r.FormFile("file")
					if err != nil {
						http.Error(w, "bad request", http.StatusBadRequest)
						return
					}
					defer file.Close()
					if name == "" {
						name = r.FormValue("name")
						if name == "" && fh != nil {
							name = fh.Filename
						}
					}
					if err := a.projectsService.UploadFile(r.Context(), userID, projectID, p, name, file); err != nil {
						log.Error().Err(err).Str("project", projectID).Str("path", p).Str("name", name).Msg("upload_file")
						http.Error(w, "error", http.StatusBadRequest)
						return
					}
					w.WriteHeader(http.StatusCreated)
					return
				}
				if name == "" {
					http.Error(w, "missing name", http.StatusBadRequest)
					return
				}
				if err := a.projectsService.UploadFile(r.Context(), userID, projectID, p, name, r.Body); err != nil {
					log.Error().Err(err).Str("project", projectID).Str("path", p).Str("name", name).Msg("upload_file_raw")
					http.Error(w, "error", http.StatusBadRequest)
					return
				}
				w.WriteHeader(http.StatusCreated)
			case http.MethodDelete:
				p := r.URL.Query().Get("path")
				if err := a.projectsService.DeleteFile(r.Context(), userID, projectID, p); err != nil {
					log.Error().Err(err).Str("project", projectID).Str("path", p).Msg("delete_file")
					http.Error(w, "error", http.StatusBadRequest)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		case "dirs":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			p := r.URL.Query().Get("path")
			if err := a.projectsService.CreateDir(r.Context(), userID, projectID, p); err != nil {
				log.Error().Err(err).Str("project", projectID).Str("path", p).Msg("create_dir")
				http.Error(w, "error", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusCreated)
			return
		case "move":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			defer r.Body.Close()
			var in struct {
				From string `json:"from"`
				To   string `json:"to"`
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if err := a.projectsService.MovePath(r.Context(), userID, projectID, in.From, in.To); err != nil {
				log.Error().Err(err).Str("project", projectID).Str("from", in.From).Str("to", in.To).Msg("move_path")
				http.Error(w, "error", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}
}

func (a *app) projectsCORS(w http.ResponseWriter, r *http.Request, methods string) {
	if origin := r.Header.Get("Origin"); origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	} else {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept")
	if methods != "" {
		w.Header().Set("Access-Control-Allow-Methods", methods)
	}
}

func (a *app) resolveProjectsUser(r *http.Request) (int64, bool, error) {
	if !a.cfg.Auth.Enabled {
		return 0, true, nil
	}
	u, ok := auth.CurrentUser(r.Context())
	if !ok || u == nil {
		return 0, false, errors.New("unauthorized")
	}
	userID := u.ID
	if q := strings.TrimSpace(r.URL.Query().Get("userId")); q != "" {
		okRole, err := a.authStore.HasRole(r.Context(), u.ID, "admin")
		if err != nil {
			return 0, false, err
		}
		if !okRole {
			return 0, false, persist.ErrForbidden
		}
		var id int64
		if _, err := fmt.Sscan(q, &id); err != nil {
			return 0, false, fmt.Errorf("bad userId")
		}
		userID = id
	}
	return userID, true, nil
}
