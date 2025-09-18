package webui

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterFrontend(t *testing.T) {
	mux := http.NewServeMux()
	err := RegisterFrontend(mux, Options{})
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			t.Skip("frontend dist is not built; run make frontend")
		}
		t.Fatalf("RegisterFrontend() error = %v", err)
	}

	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/", nil))

	if resp.Code != http.StatusOK && resp.Code != http.StatusNotModified {
		t.Fatalf("unexpected status: %d", resp.Code)
	}
}

func TestRegisterFrontendInvalidProxy(t *testing.T) {
	mux := http.NewServeMux()
	if err := RegisterFrontend(mux, Options{DevProxy: "://invalid::"}); err == nil {
		t.Fatal("expected error for invalid proxy url")
	}
}
