package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	configpkg "manifold/internal/config"
)

func TestSaveFileHandler_PathTraversal(t *testing.T) {
	e := echo.New()
	tmp := t.TempDir()
	body := strings.NewReader(`{"filepath":"../evil.txt","content":"x"}`)
	req := httptest.NewRequest(http.MethodPost, "/save-file", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("config", &configpkg.Config{DataPath: tmp})

	if err := saveFileHandler(c); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 status, got %d", rec.Code)
	}
}

func TestOpenFileHandler_PathTraversal(t *testing.T) {
	e := echo.New()
	tmp := t.TempDir()
	body := strings.NewReader(`{"filepath":"../evil.txt"}`)
	req := httptest.NewRequest(http.MethodPost, "/open-file", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("config", &configpkg.Config{DataPath: tmp})

	if err := openFileHandler(c); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 status, got %d", rec.Code)
	}
}
