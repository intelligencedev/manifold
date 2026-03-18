package agentd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"manifold/internal/config"
	"manifold/internal/flow"
	"manifold/internal/persistence/databases"
	"manifold/internal/tools"
	"manifold/internal/tools/utility"
)

func TestFlowV2WorkflowCRUD(t *testing.T) {
	t.Parallel()

	a := &app{
		cfg:    &config.Config{},
		flowV2: newFlowV2Runtime(nil),
	}

	putReqBody, _ := json.Marshal(flow.PutWorkflowRequest{
		Workflow: flow.Workflow{
			ID:   "wf_crud",
			Name: "CRUD Flow",
			Trigger: flow.Trigger{
				Type: flow.TriggerTypeManual,
			},
			Nodes: []flow.Node{
				{
					ID:   "n1",
					Name: "Step One",
					Kind: flow.NodeKindData,
					Type: "set",
				},
			},
		},
	})

	detail := a.flowV2WorkflowDetailHandler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/flows/v2/workflows/wf_crud", bytes.NewReader(putReqBody))
	detail.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 create, got %d body=%s", rec.Code, rec.Body.String())
	}

	listRec := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/api/flows/v2/workflows", nil)
	a.flowV2WorkflowsHandler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 list, got %d", listRec.Code)
	}
	var listResp flow.ListWorkflowsResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(listResp.Workflows) != 1 || listResp.Workflows[0].ID != "wf_crud" {
		t.Fatalf("unexpected list response: %+v", listResp.Workflows)
	}

	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/api/flows/v2/workflows/wf_crud", nil)
	detail.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 get, got %d", getRec.Code)
	}
	var getResp flow.GetWorkflowResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("unmarshal get: %v", err)
	}
	if getResp.Workflow.ID != "wf_crud" {
		t.Fatalf("unexpected workflow id: %s", getResp.Workflow.ID)
	}

	delRec := httptest.NewRecorder()
	delReq := httptest.NewRequest(http.MethodDelete, "/api/flows/v2/workflows/wf_crud", nil)
	detail.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 delete, got %d", delRec.Code)
	}

	getMissingRec := httptest.NewRecorder()
	getMissingReq := httptest.NewRequest(http.MethodGet, "/api/flows/v2/workflows/wf_crud", nil)
	detail.ServeHTTP(getMissingRec, getMissingReq)
	if getMissingRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing workflow, got %d", getMissingRec.Code)
	}
}

func TestFlowV2WorkflowPersistsAcrossRuntimeRestart(t *testing.T) {
	t.Parallel()

	store := databases.NewPostgresFlowV2Store(nil)
	a := &app{
		cfg:    &config.Config{},
		flowV2: newFlowV2Runtime(store),
	}

	putReqBody, _ := json.Marshal(flow.PutWorkflowRequest{
		Workflow: flow.Workflow{
			ID:          "wf_restart",
			Name:        "Restart Flow",
			Description: "persists across runtime instances",
			Trigger:     flow.Trigger{Type: flow.TriggerTypeSchedule, Schedule: &flow.ScheduleTrigger{Cron: "0 * * * *"}},
			Nodes: []flow.Node{{
				ID:   "n1",
				Name: "Step One",
				Kind: flow.NodeKindAction,
				Type: "tool",
				Tool: "utility_textbox",
				Inputs: map[string]flow.InputBinding{
					"text": {Expression: "$run.input.message"},
				},
			}},
		},
		Canvas: flow.WorkflowCanvas{
			Nodes: map[string]flow.CanvasNode{"n1": {X: 48, Y: 96}},
		},
	})

	detail := a.flowV2WorkflowDetailHandler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/flows/v2/workflows/wf_restart", bytes.NewReader(putReqBody))
	detail.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 create, got %d body=%s", rec.Code, rec.Body.String())
	}

	restarted := &app{
		cfg:    &config.Config{},
		flowV2: newFlowV2Runtime(store),
	}

	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/api/flows/v2/workflows/wf_restart", nil)
	restarted.flowV2WorkflowDetailHandler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 get after restart, got %d body=%s", getRec.Code, getRec.Body.String())
	}

	var getResp flow.GetWorkflowResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("unmarshal get: %v", err)
	}
	if getResp.Workflow.Trigger.Type != flow.TriggerTypeSchedule {
		t.Fatalf("unexpected trigger type after restart: %s", getResp.Workflow.Trigger.Type)
	}
	if getResp.Workflow.Nodes[0].Inputs["text"].Expression != "$run.input.message" {
		t.Fatalf("unexpected input binding after restart: %+v", getResp.Workflow.Nodes[0].Inputs["text"])
	}
	if getResp.Canvas.Nodes["n1"].X != 48 || getResp.Canvas.Nodes["n1"].Y != 96 {
		t.Fatalf("unexpected canvas after restart: %+v", getResp.Canvas.Nodes["n1"])
	}
}

func TestFlowV2RunLifecycle(t *testing.T) {
	t.Parallel()

	reg := tools.NewRegistry()
	reg.Register(utility.NewTextboxTool())
	a := &app{
		cfg:          &config.Config{},
		toolRegistry: reg,
		flowV2:       newFlowV2Runtime(nil),
	}

	_, _, _ = a.flowV2.upsertWorkflow(context.Background(), 0, flow.Workflow{
		ID:   "wf_run",
		Name: "Run Flow",
		Trigger: flow.Trigger{
			Type: flow.TriggerTypeManual,
		},
		Nodes: []flow.Node{
			{
				ID:   "textbox",
				Name: "Textbox",
				Kind: flow.NodeKindAction,
				Type: "tool",
				Tool: "utility_textbox",
				Inputs: map[string]flow.InputBinding{
					"text": {Literal: "hello"},
				},
				Execution: flow.NodeExecution{
					OnError: flow.ErrorStrategyFail,
				},
			},
		},
	}, flow.WorkflowCanvas{})

	runBody, _ := json.Marshal(flow.RunRequest{WorkflowID: "wf_run"})
	runRec := httptest.NewRecorder()
	runReq := httptest.NewRequest(http.MethodPost, "/api/flows/v2/run", bytes.NewReader(runBody))
	a.flowV2RunHandler().ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 run start, got %d body=%s", runRec.Code, runRec.Body.String())
	}
	var runResp flow.RunResponse
	if err := json.Unmarshal(runRec.Body.Bytes(), &runResp); err != nil {
		t.Fatalf("unmarshal run response: %v", err)
	}
	if runResp.RunID == "" {
		t.Fatal("expected run id")
	}

	var eventsResp struct {
		RunID  string          `json:"run_id"`
		Status string          `json:"status"`
		Events []flow.RunEvent `json:"events"`
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		eventsRec := httptest.NewRecorder()
		eventsReq := httptest.NewRequest(http.MethodGet, "/api/flows/v2/runs/"+runResp.RunID+"/events", nil)
		a.flowV2RunEventsHandler().ServeHTTP(eventsRec, eventsReq)
		if eventsRec.Code != http.StatusOK {
			t.Fatalf("expected 200 events, got %d body=%s", eventsRec.Code, eventsRec.Body.String())
		}
		if err := json.Unmarshal(eventsRec.Body.Bytes(), &eventsResp); err != nil {
			t.Fatalf("unmarshal events response: %v", err)
		}
		if eventsResp.Status != "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run did not complete in time; events=%+v", eventsResp.Events)
		}
		time.Sleep(20 * time.Millisecond)
	}

	if eventsResp.Status != "completed" {
		t.Fatalf("expected completed status, got %s", eventsResp.Status)
	}
	if !hasRunEvent(eventsResp.Events, flow.RunEventTypeRunStarted) {
		t.Fatalf("missing run_started event: %+v", eventsResp.Events)
	}
	if !hasRunEvent(eventsResp.Events, flow.RunEventTypeNodeCompleted) {
		t.Fatalf("missing node_completed event: %+v", eventsResp.Events)
	}
	if !hasRunEvent(eventsResp.Events, flow.RunEventTypeRunCompleted) {
		t.Fatalf("missing run_completed event: %+v", eventsResp.Events)
	}
}

func TestFlowV2RunUsesBaseToolRegistry(t *testing.T) {
	t.Parallel()

	baseReg := tools.NewRegistry()
	baseReg.Register(utility.NewTextboxTool())
	filteredReg := tools.NewRegistry() // intentionally empty

	a := &app{
		cfg:              &config.Config{},
		baseToolRegistry: baseReg,
		toolRegistry:     filteredReg,
		flowV2:           newFlowV2Runtime(nil),
	}

	_, _, _ = a.flowV2.upsertWorkflow(context.Background(), 0, flow.Workflow{
		ID:   "wf_base_tools",
		Name: "Base Tool Flow",
		Trigger: flow.Trigger{
			Type: flow.TriggerTypeManual,
		},
		Nodes: []flow.Node{
			{
				ID:   "textbox",
				Name: "Textbox",
				Kind: flow.NodeKindAction,
				Type: "tool",
				Tool: "utility_textbox",
				Inputs: map[string]flow.InputBinding{
					"text": {Literal: "hello"},
				},
			},
		},
	}, flow.WorkflowCanvas{})

	runBody, _ := json.Marshal(flow.RunRequest{WorkflowID: "wf_base_tools"})
	runRec := httptest.NewRecorder()
	runReq := httptest.NewRequest(http.MethodPost, "/api/flows/v2/run", bytes.NewReader(runBody))
	a.flowV2RunHandler().ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 run start, got %d body=%s", runRec.Code, runRec.Body.String())
	}

	var runResp flow.RunResponse
	if err := json.Unmarshal(runRec.Body.Bytes(), &runResp); err != nil {
		t.Fatalf("unmarshal run response: %v", err)
	}
	if runResp.RunID == "" {
		t.Fatal("expected run id")
	}

	var eventsResp struct {
		Status string          `json:"status"`
		Events []flow.RunEvent `json:"events"`
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		eventsRec := httptest.NewRecorder()
		eventsReq := httptest.NewRequest(http.MethodGet, "/api/flows/v2/runs/"+runResp.RunID+"/events", nil)
		a.flowV2RunEventsHandler().ServeHTTP(eventsRec, eventsReq)
		if eventsRec.Code != http.StatusOK {
			t.Fatalf("expected 200 events, got %d body=%s", eventsRec.Code, eventsRec.Body.String())
		}
		if err := json.Unmarshal(eventsRec.Body.Bytes(), &eventsResp); err != nil {
			t.Fatalf("unmarshal events response: %v", err)
		}
		if eventsResp.Status != "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run did not complete in time; events=%+v", eventsResp.Events)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if eventsResp.Status != "completed" {
		t.Fatalf("expected completed status, got %s with events=%+v", eventsResp.Status, eventsResp.Events)
	}

	var completed *flow.RunEvent
	for i := range eventsResp.Events {
		event := &eventsResp.Events[i]
		if event.Type == flow.RunEventTypeNodeCompleted && event.NodeID == "agent_response" {
			completed = event
			break
		}
	}
	if completed == nil {
		t.Fatalf("expected node_completed event for agent_response, got %+v", eventsResp.Events)
	}
	inputs, ok := completed.Output["inputs"].(map[string]any)
	if !ok {
		t.Fatalf("expected completed event inputs map, got %+v", completed.Output)
	}
	if got := inputs["text"]; got != "hello" {
		t.Fatalf("expected rendered text input hello, got %#v", got)
	}
	if got := inputs["render_mode"]; got != "markdown" {
		t.Fatalf("expected rendered mode markdown, got %#v", got)
	}
}

func TestFlowV2RunAgentResponseTool(t *testing.T) {
	t.Parallel()

	reg := tools.NewRegistry()
	reg.Register(utility.NewAgentResponseTool())
	a := &app{
		cfg:              &config.Config{},
		baseToolRegistry: reg,
		toolRegistry:     reg,
		flowV2:           newFlowV2Runtime(nil),
	}

	_, _, _ = a.flowV2.upsertWorkflow(context.Background(), 0, flow.Workflow{
		ID:   "wf_agent_response",
		Name: "Agent Response Flow",
		Trigger: flow.Trigger{
			Type: flow.TriggerTypeManual,
		},
		Nodes: []flow.Node{
			{
				ID:   "agent_response",
				Name: "Agent Response",
				Kind: flow.NodeKindAction,
				Type: "tool",
				Tool: "agent_response",
				Inputs: map[string]flow.InputBinding{
					"text":        {Literal: "hello"},
					"render_mode": {Literal: "markdown"},
				},
			},
		},
	}, flow.WorkflowCanvas{})

	runBody, _ := json.Marshal(flow.RunRequest{WorkflowID: "wf_agent_response"})
	runRec := httptest.NewRecorder()
	runReq := httptest.NewRequest(http.MethodPost, "/api/flows/v2/run", bytes.NewReader(runBody))
	a.flowV2RunHandler().ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 run start, got %d body=%s", runRec.Code, runRec.Body.String())
	}

	var runResp flow.RunResponse
	if err := json.Unmarshal(runRec.Body.Bytes(), &runResp); err != nil {
		t.Fatalf("unmarshal run response: %v", err)
	}
	if runResp.RunID == "" {
		t.Fatal("expected run id")
	}

	var eventsResp struct {
		Status string          `json:"status"`
		Events []flow.RunEvent `json:"events"`
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		eventsRec := httptest.NewRecorder()
		eventsReq := httptest.NewRequest(http.MethodGet, "/api/flows/v2/runs/"+runResp.RunID+"/events", nil)
		a.flowV2RunEventsHandler().ServeHTTP(eventsRec, eventsReq)
		if eventsRec.Code != http.StatusOK {
			t.Fatalf("expected 200 events, got %d body=%s", eventsRec.Code, eventsRec.Body.String())
		}
		if err := json.Unmarshal(eventsRec.Body.Bytes(), &eventsResp); err != nil {
			t.Fatalf("unmarshal events response: %v", err)
		}
		if eventsResp.Status != "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run did not complete in time; events=%+v", eventsResp.Events)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if eventsResp.Status != "completed" {
		t.Fatalf("expected completed status, got %s with events=%+v", eventsResp.Status, eventsResp.Events)
	}
}

func TestFlowV2ToolsEndpoint(t *testing.T) {
	t.Parallel()

	reg := tools.NewRegistry()
	reg.Register(utility.NewTextboxTool())
	a := &app{
		cfg:              &config.Config{},
		baseToolRegistry: reg,
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/flows/v2/tools", nil)
	a.flowV2ToolsHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 tools, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal tools payload: %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("expected non-empty tools payload")
	}
	found := false
	for _, item := range payload {
		if item["name"] == "utility_textbox" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected utility_textbox in tools payload: %+v", payload)
	}
}

func hasRunEvent(events []flow.RunEvent, typ flow.RunEventType) bool {
	for _, ev := range events {
		if ev.Type == typ {
			return true
		}
	}
	return false
}
