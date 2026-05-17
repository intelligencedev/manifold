package agentd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"manifold/internal/config"
	"manifold/internal/fleet"
	"manifold/internal/persistence/databases"
	"manifold/internal/trust"
	"manifold/internal/constitution"
)

func TestFleetStateHandlerReturnsSnapshot(t *testing.T) {
	a := &app{
		cfg:             &config.Config{},
		runs:            newRunStore(),
		inputRequests:   newInputRequestBroker(),
		fleetBus:        fleet.NewBus(16),
		specStore:       databases.NewSpecialistsStore(nil),
		teamStore:       databases.NewSpecialistTeamsStore(nil),
		trustService:    trust.NewService(trust.NewStore(nil)),
		constitutionSvc: constitution.NewService(constitution.NewStore(nil)),
	}
	a.runs.create("hello")
	a.fleetBus.Publish(fleet.Event{Kind: fleet.EventDelegation, CallID: "child", ParentCallID: "parent", Agent: "builder", UserID: 0})
	req := httptest.NewRequest(http.MethodGet, "/api/fleet/state", nil)
	rr := httptest.NewRecorder()
	a.fleetStateHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := body["runs"]; !ok {
		t.Fatalf("expected runs field")
	}
}
