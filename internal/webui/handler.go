package webui

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Options struct {
	DevProxy string
	// AuthGate, when provided, will be called for UI asset requests. If it returns false
	// the request will be redirected to UnauthedRedirect (or /auth/login by default).
	AuthGate func(r *http.Request) bool
	// UnauthedRedirect target path. Default: /auth/login
	UnauthedRedirect string
}

func RegisterFrontend(mux *http.ServeMux, opts Options) error {
	if mux == nil {
		return fmt.Errorf("webui: mux is nil")
	}

	if opts.DevProxy != "" {
		target, err := url.Parse(opts.DevProxy)
		if err != nil {
			return fmt.Errorf("webui: parse dev proxy url: %w", err)
		}
		h := newDevProxy(target)
		if opts.AuthGate != nil {
			h = authWrapper(h, opts)
		}
		mux.Handle("/", h)
		return nil
	}

	fsys, err := DistFS()
	if err != nil {
		return err
	}

	handler, err := newSPAHandler(fsys)
	if err != nil {
		return err
	}

	if opts.AuthGate != nil {
		handler = authWrapper(handler, opts)
	}
	mux.Handle("/", handler)
	return nil
}

type spaHandler struct {
	fsys       fs.FS
	assetCache sync.Map
	index      []byte
	indexETag  string
}

type cachedAsset struct {
	data []byte
	mime string
}

func newSPAHandler(fsys fs.FS) (http.Handler, error) {
	index, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		return nil, fmt.Errorf("webui: load index: %w", err)
	}
	sum := sha256.Sum256(index)
	return &spaHandler{
		fsys:      fsys,
		index:     index,
		indexETag: fmt.Sprintf("\"%x\"", sum),
	}, nil
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cleanPath := strings.TrimPrefix(r.URL.Path, "/")
	cleanPath = path.Clean(cleanPath)

	if cleanPath == "." || !strings.Contains(cleanPath, ".") {
		h.serveIndex(w, r)
		return
	}

	if err := h.serveAsset(w, r, cleanPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// authWrapper enforces authentication on UI routes by delegating to opts.AuthGate.
func authWrapper(next http.Handler, opts Options) http.Handler {
	redirect := opts.UnauthedRedirect
	if redirect == "" {
		redirect = "/auth/login"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if opts.AuthGate != nil {
			if ok := opts.AuthGate(r); !ok {
				// Allow auth endpoints themselves to be reachable
				if strings.HasPrefix(r.URL.Path, "/auth/") {
					next.ServeHTTP(w, r)
					return
				}
				http.Redirect(w, r, redirect, http.StatusFound)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (h *spaHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", h.indexETag)

	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, h.indexETag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", strconv.Itoa(len(h.index)))
		w.WriteHeader(http.StatusOK)
		return
	}

	http.ServeContent(w, r, "index.html", time.Unix(0, 0), bytes.NewReader(h.index))
}

func (h *spaHandler) serveAsset(w http.ResponseWriter, r *http.Request, name string) error {
	asset, err := h.loadAsset(name)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", asset.mime)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", strconv.Itoa(len(asset.data)))
		w.WriteHeader(http.StatusOK)
		return nil
	}

	http.ServeContent(w, r, filepath.Base(name), time.Unix(0, 0), bytes.NewReader(asset.data))
	return nil
}

func (h *spaHandler) loadAsset(name string) (*cachedAsset, error) {
	if cached, ok := h.assetCache.Load(name); ok {
		return cached.(*cachedAsset), nil
	}

	data, err := fs.ReadFile(h.fsys, name)
	if err != nil {
		return nil, err
	}

	mimeType := mime.TypeByExtension(filepath.Ext(name))
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}

	asset := &cachedAsset{data: data, mime: mimeType}
	h.assetCache.Store(name, asset)
	return asset, nil
}

func newDevProxy(target *url.URL) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, fmt.Sprintf("frontend proxy error: %v", err), http.StatusBadGateway)
	}
	proxy.FlushInterval = 100 * time.Millisecond
	return proxy
}
