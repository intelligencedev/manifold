package warpp

import "testing"

func TestRegistryUpsertAndRemove(t *testing.T) {
	r := &Registry{}
	w := Workflow{Intent: "flow", Steps: []Step{{ID: "s1", Text: "x"}}}
	r.Upsert(w, "/tmp/flow.json")
	if _, err := r.Get("flow"); err != nil {
		t.Fatalf("Get after upsert: %v", err)
	}
	if got := r.Path("flow"); got != "/tmp/flow.json" {
		t.Fatalf("unexpected path: %s", got)
	}
	r.Remove("flow")
	if _, err := r.Get("flow"); err == nil {
		t.Fatalf("expected missing workflow after remove")
	}
}
