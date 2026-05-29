package agentd

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"manifold/internal/auth"
	persist "manifold/internal/persistence"
	"manifold/internal/projects"
	"manifold/internal/workspaces"
)

var allowedTextExtensions = map[string]struct{}{
	".txt":  {},
	".md":   {},
	".log":  {},
	".json": {},
	".js":   {},
	".ts":   {},
	".go":   {},
	".py":   {},
	".java": {},
	".c":    {},
	".cpp":  {},
	".yml":  {},
	".yaml": {},
	".toml": {},
	".ini":  {},
	".sh":   {},
	".csv":  {},
}

func isAllowedTextFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return false
	}
	_, ok := allowedTextExtensions[ext]
	return ok
}

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
		cleanPID, err := workspaces.ValidateProjectID(projectID)
		if err != nil || cleanPID == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		projectID = cleanPID
		if len(parts) == 1 {
			a.handleProjectRootDetail(w, r, userID, projectID)
			return
		}

		if len(parts) < 2 {
			http.NotFound(w, r)
			return
		}
		switch parts[1] {
		case "archive":
			a.handleProjectArchive(w, r, userID, projectID)
			return
		case "tree":
			a.handleProjectTree(w, r, userID, projectID)
			return
		case "files":
			a.handleProjectFiles(w, r, userID, projectID)
			return
		case "dirs":
			a.handleProjectDirCreate(w, r, userID, projectID)
			return
		case "move":
			a.handleProjectMove(w, r, userID, projectID)
			return
		}
		http.NotFound(w, r)
	}
}

func (a *app) handleProjectRootDetail(w http.ResponseWriter, r *http.Request, userID int64, projectID string) {
	switch r.Method {
	case http.MethodDelete:
		if err := a.projectsService.DeleteProject(r.Context(), userID, projectID); err != nil {
			log.Error().Err(err).Str("project", projectID).Msg("delete_project")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodGet:
		a.writeProjectTree(w, r, userID, projectID, ".", "list_tree_root")
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *app) handleProjectArchive(w http.ResponseWriter, r *http.Request, userID int64, projectID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.streamProjectArchive(w, r, userID, projectID)
}

func (a *app) handleProjectTree(w http.ResponseWriter, r *http.Request, userID int64, projectID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.writeProjectTree(w, r, userID, projectID, r.URL.Query().Get("path"), "list_tree")
}

func (a *app) writeProjectTree(w http.ResponseWriter, r *http.Request, userID int64, projectID string, path string, logMessage string) {
	entries, err := a.projectsService.ListTree(r.Context(), userID, projectID, path)
	if err != nil {
		log.Error().Err(err).Str("project", projectID).Str("path", path).Msg(logMessage)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"entries": projectTreeRows(entries)})
}

func projectTreeRows(entries []projects.FileEntry) []map[string]any {
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
	return rows
}

func (a *app) handleProjectFiles(w http.ResponseWriter, r *http.Request, userID int64, projectID string) {
	switch r.Method {
	case http.MethodGet:
		a.handleProjectFileRead(w, r, userID, projectID)
	case http.MethodPost:
		a.handleProjectFileUpload(w, r, userID, projectID)
	case http.MethodDelete:
		a.handleProjectFileDelete(w, r, userID, projectID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *app) handleProjectFileRead(w http.ResponseWriter, r *http.Request, userID int64, projectID string) {
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
	streamProjectFile(w, rc, p, projectID)
}

func streamProjectFile(w http.ResponseWriter, rc io.Reader, path, projectID string) {
	var sniff [512]byte
	n, _ := io.ReadFull(rc, sniff[:])
	w.Header().Set("Content-Type", projectFileContentType(path, sniff[:n]))
	if n > 0 {
		_, _ = w.Write(sniff[:n])
	}
	if _, err := io.Copy(w, rc); err != nil {
		log.Error().Err(err).Str("project", projectID).Str("path", path).Msg("stream_file")
	}
}

func projectFileContentType(path string, sniff []byte) string {
	ct := "application/octet-stream"
	if ext := filepath.Ext(path); ext != "" {
		if mt := mime.TypeByExtension(ext); mt != "" {
			ct = mt
		}
	}
	if ct == "application/octet-stream" && len(sniff) > 0 {
		ct = http.DetectContentType(sniff)
	}
	return ct
}

func (a *app) handleProjectFileUpload(w http.ResponseWriter, r *http.Request, userID int64, projectID string) {
	p := r.URL.Query().Get("path")
	name := r.URL.Query().Get("name")
	if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/") {
		a.handleProjectMultipartUpload(w, r, userID, projectID, p, name)
		return
	}
	if name == "" {
		http.Error(w, "missing name", http.StatusBadRequest)
		return
	}
	if !isAllowedTextFile(name) {
		http.Error(w, "unsupported file type", http.StatusBadRequest)
		return
	}
	if err := a.projectsService.UploadFile(r.Context(), userID, projectID, p, name, r.Body); err != nil {
		log.Error().Err(err).Str("project", projectID).Str("path", p).Str("name", name).Msg("upload_file_raw")
		http.Error(w, "error", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *app) handleProjectMultipartUpload(w http.ResponseWriter, r *http.Request, userID int64, projectID, path, name string) {
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
	if err := a.projectsService.UploadFile(r.Context(), userID, projectID, path, name, file); err != nil {
		log.Error().Err(err).Str("project", projectID).Str("path", path).Str("name", name).Msg("upload_file")
		http.Error(w, "error", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *app) handleProjectFileDelete(w http.ResponseWriter, r *http.Request, userID int64, projectID string) {
	p := r.URL.Query().Get("path")
	if err := a.projectsService.DeleteFile(r.Context(), userID, projectID, p); err != nil {
		log.Error().Err(err).Str("project", projectID).Str("path", p).Msg("delete_file")
		http.Error(w, "error", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) handleProjectDirCreate(w http.ResponseWriter, r *http.Request, userID int64, projectID string) {
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
}

func (a *app) handleProjectMove(w http.ResponseWriter, r *http.Request, userID int64, projectID string) {
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
	// RBAC: Admins are treated like regular users for projects; no cross-user access.
	// Always scope to the current user's own projects, ignoring any userId overrides.
	return u.ID, true, nil
}

func sanitizeArchiveFilename(name string) string {
	safeName := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return '-'
		}
		return r
	}, strings.TrimSpace(name))
	if safeName == "" {
		return "project"
	}
	return safeName
}

func archivePathBaseName(p string) string {
	clean := strings.Trim(filepath.ToSlash(strings.TrimSpace(p)), "/")
	if clean == "" || clean == "." {
		return ""
	}
	idx := strings.LastIndex(clean, "/")
	if idx == -1 {
		return clean
	}
	return clean[idx+1:]
}

type archiveSingleFileRequest struct {
	UserID      int64
	ProjectID   string
	ProjectPath string
	ArchivePath string
	ModTime     time.Time
}

func (a *app) archiveSingleFile(ctx context.Context, tw *tar.Writer, req archiveSingleFileRequest) error {
	if strings.TrimSpace(req.ArchivePath) == "" {
		req.ArchivePath = archivePathBaseName(req.ProjectPath)
	}
	if strings.TrimSpace(req.ArchivePath) == "" {
		return fmt.Errorf("invalid archive path")
	}
	rc, err := a.projectsService.ReadFile(ctx, req.UserID, req.ProjectID, req.ProjectPath)
	if err != nil {
		return fmt.Errorf("read file %s: %w", req.ProjectPath, err)
	}
	defer rc.Close()
	content, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("read file content %s: %w", req.ProjectPath, err)
	}
	if req.ModTime.IsZero() {
		req.ModTime = time.Now().UTC()
	}
	hdr := &tar.Header{
		Name:    req.ArchivePath,
		Mode:    0644,
		Size:    int64(len(content)),
		ModTime: req.ModTime,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("write file header %s: %w", req.ArchivePath, err)
	}
	if _, err := tw.Write(content); err != nil {
		return fmt.Errorf("write file content %s: %w", req.ArchivePath, err)
	}
	return nil
}

// streamProjectArchive creates a tar.gz archive of a project or subpath and streams it.
func (a *app) streamProjectArchive(w http.ResponseWriter, r *http.Request, userID int64, projectID string) {
	ctx := r.Context()
	sourcePath := strings.TrimSpace(r.URL.Query().Get("path"))
	if sourcePath == "" {
		sourcePath = "."
	}

	target, err := a.resolveArchiveTarget(ctx, userID, projectID, sourcePath)
	if err != nil {
		if errors.Is(err, persist.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("project", projectID).Msg("archive_resolve_target")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	safeName := sanitizeArchiveFilename(target.Name)
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.tar.gz"`, safeName))

	gzw := gzip.NewWriter(w)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	if err := a.writeProjectArchive(ctx, tw, userID, projectID, target); err != nil {
		log.Error().
			Err(err).
			Str("project", projectID).
			Str("path", sourcePath).
			Msg("archive_project")
		return
	}
}

type archiveTarget struct {
	Mode       string
	Name       string
	SourcePath string
}

func (a *app) resolveArchiveTarget(ctx context.Context, userID int64, projectID, sourcePath string) (archiveTarget, error) {
	projects, err := a.projectsService.ListProjects(ctx, userID)
	if err != nil {
		return archiveTarget{}, err
	}
	target := archiveTarget{Mode: "project", Name: projectID, SourcePath: sourcePath}
	for _, p := range projects {
		if p.ID == projectID {
			target.Name = p.Name
			break
		}
	}
	if sourcePath != "." {
		if _, listErr := a.projectsService.ListTree(ctx, userID, projectID, sourcePath); listErr == nil {
			target.Mode = "dir"
		} else {
			rc, readErr := a.projectsService.ReadFile(ctx, userID, projectID, sourcePath)
			if readErr != nil {
				log.Warn().
					Err(readErr).
					Str("project", projectID).
					Str("path", sourcePath).
					Msg("archive_path_not_found")
				return archiveTarget{}, persist.ErrNotFound
			}
			_ = rc.Close()
			target.Mode = "file"
		}
		if base := archivePathBaseName(sourcePath); base != "" {
			target.Name = base
		}
	}
	return target, nil
}

func (a *app) writeProjectArchive(ctx context.Context, tw *tar.Writer, userID int64, projectID string, target archiveTarget) error {
	switch target.Mode {
	case "project":
		return a.archiveDir(ctx, tw, userID, projectID, ".", "")
	case "dir":
		rootName := archivePathBaseName(target.SourcePath)
		if rootName == "" {
			rootName = "folder"
		}
		if err := tw.WriteHeader(&tar.Header{
			Name:     rootName + "/",
			Mode:     0755,
			Typeflag: tar.TypeDir,
			ModTime:  time.Now().UTC(),
		}); err != nil {
			return fmt.Errorf("write dir header %s: %w", rootName, err)
		}
		return a.archiveDir(ctx, tw, userID, projectID, target.SourcePath, rootName)
	case "file":
		return a.archiveSingleFile(ctx, tw, archiveSingleFileRequest{
			UserID:      userID,
			ProjectID:   projectID,
			ProjectPath: target.SourcePath,
			ArchivePath: archivePathBaseName(target.SourcePath),
			ModTime:     time.Now().UTC(),
		})
	}
	return fmt.Errorf("unsupported archive mode %q", target.Mode)
}

// archiveDir recursively adds all files and directories to the tar archive.
func (a *app) archiveDir(ctx context.Context, tw *tar.Writer, userID int64, projectID, treePath, archivePath string) error {
	entries, err := a.projectsService.ListTree(ctx, userID, projectID, treePath)
	if err != nil {
		return fmt.Errorf("list tree %s: %w", treePath, err)
	}

	for _, entry := range entries {
		entryArchivePath := entry.Name
		if archivePath != "" {
			entryArchivePath = archivePath + "/" + entry.Name
		}

		if entry.Type == "dir" {
			// Add directory header
			hdr := &tar.Header{
				Name:     entryArchivePath + "/",
				Mode:     0755,
				Typeflag: tar.TypeDir,
				ModTime:  entry.ModTime,
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return fmt.Errorf("write dir header %s: %w", entryArchivePath, err)
			}
			if err := a.archiveDir(ctx, tw, userID, projectID, entry.Path, entryArchivePath); err != nil {
				return err
			}
		} else {
			// Read file content
			rc, err := a.projectsService.ReadFile(ctx, userID, projectID, entry.Path)
			if err != nil {
				log.Warn().Err(err).Str("path", entry.Path).Msg("archive_skip_file")
				continue // Skip files we can't read
			}

			// Read all content to get size (needed for tar header)
			content, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				log.Warn().Err(err).Str("path", entry.Path).Msg("archive_read_file")
				continue
			}

			// Add file header
			hdr := &tar.Header{
				Name:    entryArchivePath,
				Mode:    0644,
				Size:    int64(len(content)),
				ModTime: entry.ModTime,
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return fmt.Errorf("write file header %s: %w", entryArchivePath, err)
			}
			if _, err := tw.Write(content); err != nil {
				return fmt.Errorf("write file content %s: %w", entryArchivePath, err)
			}
		}
	}
	return nil
}
