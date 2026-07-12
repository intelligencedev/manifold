package agentd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"manifold/internal/config"
	"manifold/internal/warpp"
)

const warppLinearDoc = `{
  "id":"lin","name":"Linear",
  "inputs":[{"name":"topic","type":"text","required":true}],
  "nodes":[{"id":"tpl","type":"data.template",
    "inputs":{"template":{"value":"about {t}"},"vars":{"t":{"from":"in.topic"}}}}],
  "outputs":{"out":{"from":"tpl.text"}}}`

func newWarppTestApp() *app {
	return &app{cfg: &config.Config{}, warpp: newWarppRuntime(newFakeWarppStore())}
}

func doWarppReq(t *testing.T, a *app, method, target string, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body == "" {
		rdr = bytes.NewReader(nil)
	} else {
		rdr = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, target, rdr)
	rec := httptest.NewRecorder()
	switch {
	case target == "/api/warpp/workflows":
		a.warppWorkflowsHandler().ServeHTTP(rec, req)
	case strings.HasPrefix(target, "/api/warpp/workflows/"):
		a.warppWorkflowDetailHandler().ServeHTTP(rec, req)
	case target == "/api/warpp/validate":
		a.warppValidateHandler().ServeHTTP(rec, req)
	case target == "/api/warpp/runs":
		a.warppRunsHandler().ServeHTTP(rec, req)
	case strings.HasPrefix(target, "/api/warpp/runs/"):
		a.warppRunEventsHandler().ServeHTTP(rec, req)
	case target == "/api/warpp/catalog":
		a.warppCatalogHandler().ServeHTTP(rec, req)
	default:
		t.Fatalf("no route for %s", target)
	}
	return rec
}

func TestWarppPutAndGetWorkflow(t *testing.T) {
	a := newWarppTestApp()
	rec := doWarppReq(t, a, http.MethodPut, "/api/warpp/workflows/lin",
		`{"document":`+warppLinearDoc+`,"canvas":{}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("put: %d %s", rec.Code, rec.Body.String())
	}
	rec = doWarppReq(t, a, http.MethodGet, "/api/warpp/workflows/lin", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d", rec.Code)
	}
	var resp warppGetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.Document.ID != "lin" {
		t.Fatalf("get body: %v %+v", err, resp.Document)
	}
	// id mismatch
	rec = doWarppReq(t, a, http.MethodPut, "/api/warpp/workflows/other",
		`{"document":`+warppLinearDoc+`,"canvas":{}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("mismatch should be 400, got %d", rec.Code)
	}
}

func TestWarppPutRejectsInvalidDocument(t *testing.T) {
	a := newWarppTestApp()
	rec := doWarppReq(t, a, http.MethodPut, "/api/warpp/workflows/bad",
		`{"document":{"id":"bad","name":"Bad","nodes":[{"id":"a","type":"nope.x"}]},"canvas":{}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	var resp warppValidateResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Valid || !hasDiagCode(resp.Diagnostics, "node.type.unknown") {
		t.Fatalf("want node.type.unknown, got %+v", resp.Diagnostics)
	}
}

func TestWarppValidateEndpoint(t *testing.T) {
	a := newWarppTestApp()
	rec := doWarppReq(t, a, http.MethodPost, "/api/warpp/validate",
		`{"document":{"id":"bad","name":"Bad","nodes":[{"id":"a","type":"nope.x"}]}}`)
	var resp warppValidateResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Valid {
		t.Fatal("invalid doc must not validate")
	}
	rec = doWarppReq(t, a, http.MethodPost, "/api/warpp/validate", `{"document":`+warppLinearDoc+`}`)
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if !resp.Valid {
		t.Fatalf("valid doc should validate: %+v", resp.Diagnostics)
	}
}

func TestWarppRunLifecycleLocal(t *testing.T) {
	a := newWarppTestApp()
	if rec := doWarppReq(t, a, http.MethodPut, "/api/warpp/workflows/lin",
		`{"document":`+warppLinearDoc+`,"canvas":{}}`); rec.Code != http.StatusCreated {
		t.Fatalf("put: %d %s", rec.Code, rec.Body.String())
	}
	rec := doWarppReq(t, a, http.MethodPost, "/api/warpp/runs", `{"workflow_id":"lin","input":{"topic":"go"}}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("run: %d %s", rec.Code, rec.Body.String())
	}
	var run warppRunHTTPResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil || run.RunID == "" {
		t.Fatalf("run resp: %v %+v", err, run)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		events, status, ok := a.warppState().getRunEvents(systemUserID, run.RunID)
		if ok && (status == warpp.StatusCompleted || status == warpp.StatusCompletedWithSkips) {
			var found bool
			for _, ev := range events {
				if ev.Type == warpp.EventNodeCompleted && ev.NodePath == "tpl" && ev.Outputs["text"] == "about go" {
					found = true
				}
			}
			if !found {
				t.Fatalf("tpl node_completed with output not found: %+v", events)
			}
			return
		}
		if ok && status == warpp.StatusFailed {
			t.Fatalf("run failed: %+v", events)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("run did not complete in time")
}

func TestWarppCatalog(t *testing.T) {
	a := newWarppTestApp()
	rec := doWarppReq(t, a, http.MethodGet, "/api/warpp/catalog", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("catalog: %d", rec.Code)
	}
	var resp warppCatalogResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	need := map[string]bool{"data.extract": false, "control.map": false, "llm.generate": false, "tool.web_search": false, "tool.generic": false}
	for _, m := range resp.Manifests {
		if _, ok := need[m.Type]; ok {
			need[m.Type] = true
		}
	}
	for typ, found := range need {
		if !found {
			t.Fatalf("catalog missing %s", typ)
		}
	}
	if len(resp.Coercions) != 2 {
		t.Fatalf("coercions=%v", resp.Coercions)
	}
}

func TestWarppRunsRequireWorkflowID(t *testing.T) {
	a := newWarppTestApp()
	rec := doWarppReq(t, a, http.MethodPost, "/api/warpp/runs", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func hasDiagCode(diags []warpp.Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}
