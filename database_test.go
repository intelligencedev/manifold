package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v4"
)

type fakeQuerier struct {
	rows pgx.Rows
	err  error
}

func (f *fakeQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return f.rows, f.err
}

func (f *fakeQuerier) Close(ctx context.Context) error { return nil }

type fakeRows struct {
	fields []pgconn.FieldDescription
	data   [][]any
	idx    int
}

func (r *fakeRows) Close()                                       {}
func (r *fakeRows) Err() error                                   { return nil }
func (r *fakeRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return r.fields }
func (r *fakeRows) Conn() *pgx.Conn                              { return nil }
func (r *fakeRows) Next() bool {
	if r.idx >= len(r.data) {
		return false
	}
	r.idx++
	return true
}
func (r *fakeRows) Scan(dest ...any) error { return nil }
func (r *fakeRows) Values() ([]any, error) { return r.data[r.idx-1], nil }
func (r *fakeRows) RawValues() [][]byte    { return nil }

func TestPostgresQueryHandler_InvalidBody(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/db/query", bytes.NewBufferString("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("config", &Config{})

	if err := postgresQueryHandler(c); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

func TestPostgresQueryHandler_Success(t *testing.T) {
	rows := &fakeRows{fields: []pgconn.FieldDescription{{Name: "id"}}, data: [][]any{{1}}}
	fakeConn := &fakeQuerier{rows: rows}
	connectFunc = func(ctx context.Context, cs string) (connector, error) { return fakeConn, nil }
	defer func() { connectFunc = func(ctx context.Context, cs string) (connector, error) { return nil, nil } }()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/db/query", strings.NewReader(`{"conn_string":"cs","query":"SELECT 1"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("config", &Config{})

	if err := postgresQueryHandler(c); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !strings.Contains(rec.Body.String(), "\"id\":1") {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}
