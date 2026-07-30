package warpp

import (
	"context"
	"sync"
	"testing"
)

func TestMapFanOutGathersInOrder(t *testing.T) {
	cap := &capture{}
	res := exec(t, testEngine(cap, nil), `{
	  "id":"m","name":"m",
	  "inputs":[{"name":"names","type":"list<text>","required":true},
	            {"name":"suffix","type":"text","required":true}],
	  "nodes":[
	    {"id":"per","type":"control.map",
	     "inputs":{"items":{"from":"in.names"},"concurrency":{"value":3}},
	     "body":{"nodes":[
	        {"id":"t","type":"data.template",
	         "inputs":{"template":{"value":"{n}-{s}"},
	                   "vars":{"n":{"from":"item.value"},"s":{"from":"in.suffix"}}}}],
	      "outputs":{"result":{"from":"t.text"}}}}],
	  "outputs":{"all":{"from":"per.results"}}}`,
		map[string]any{"names": []any{"a", "b", "c"}, "suffix": "z"})
	if res.Err != nil || res.Status != StatusCompleted {
		t.Fatalf("res=%+v", res)
	}
	got, _ := res.Outputs["all"].([]any)
	if len(got) != 3 || got[0] != "a-z" || got[1] != "b-z" || got[2] != "c-z" {
		t.Fatalf("results=%v", got)
	}
	if cap.nodeEvent(EventNodeCompleted, "per[1].t") == nil {
		t.Fatal("body events must be path-prefixed per iteration")
	}
}

func TestMapOnItemErrorSkipOmitsItem(t *testing.T) {
	cap := &capture{}
	res := exec(t, testEngine(cap, nil), `{
	  "id":"m2","name":"m2",
	  "inputs":[{"name":"docs","type":"list<text>","required":true}],
	  "nodes":[
	    {"id":"per","type":"control.map",
	     "inputs":{"items":{"from":"in.docs"},"on_item_error":{"value":"skip"}},
	     "body":{"nodes":[
	        {"id":"p","type":"data.parse","inputs":{"text":{"from":"item.value"}}}],
	      "outputs":{"result":{"from":"p.json"}}}}],
	  "outputs":{"parsed":{"from":"per.results"}}}`,
		map[string]any{"docs": []any{`{"n":1}`, `broken`, `{"n":3}`}})
	if res.Status != StatusCompletedWithSkips {
		t.Fatalf("res=%+v", res)
	}
	got, _ := res.Outputs["parsed"].([]any)
	if len(got) != 2 {
		t.Fatalf("skipped item must be omitted, got %v", got)
	}
}

func TestMapOnItemErrorFailFailsRun(t *testing.T) {
	cap := &capture{}
	res := exec(t, testEngine(cap, nil), `{
	  "id":"m3","name":"m3",
	  "inputs":[{"name":"docs","type":"list<text>","required":true}],
	  "nodes":[
	    {"id":"per","type":"control.map",
	     "inputs":{"items":{"from":"in.docs"}},
	     "body":{"nodes":[
	        {"id":"p","type":"data.parse","inputs":{"text":{"from":"item.value"}}}],
	      "outputs":{"result":{"from":"p.json"}}}}]}`,
		map[string]any{"docs": []any{`broken`}})
	if res.Status != StatusFailed {
		t.Fatalf("res=%+v", res)
	}
}

func TestNestedMap(t *testing.T) {
	cap := &capture{}
	res := exec(t, testEngine(cap, nil), `{
	  "id":"nest","name":"nest",
	  "inputs":[{"name":"grid","type":"list<json>","required":true}],
	  "nodes":[
	    {"id":"rows","type":"control.map",
	     "inputs":{"items":{"from":"in.grid"}},
	     "body":{"nodes":[
	        {"id":"cells","type":"control.map",
	         "inputs":{"items":{"from":"item.value"}},
	         "body":{"nodes":[
	            {"id":"s","type":"data.stringify","inputs":{"value":{"from":"item.value"}}}],
	          "outputs":{"result":{"from":"s.text"}}}}],
	      "outputs":{"result":{"from":"cells.results"}}}}],
	  "outputs":{"o":{"from":"rows.results"}}}`,
		map[string]any{"grid": []any{[]any{1.0, 2.0}, []any{3.0}}})
	if res.Err != nil {
		t.Fatalf("res=%+v", res)
	}
	rows, _ := res.Outputs["o"].([]any)
	if len(rows) != 2 {
		t.Fatalf("rows=%v", rows)
	}
	first, _ := rows[0].([]any)
	if len(first) != 2 || first[0] != "1" {
		t.Fatalf("inner=%v", first)
	}
	if cap.nodeEvent(EventNodeCompleted, "rows[0].cells[1].s") == nil {
		t.Fatal("nested iteration paths must compose")
	}
}

func TestMapStepKeysIncludeIteration(t *testing.T) {
	var mu sync.Mutex
	var keys []string
	cap := &capture{}
	e := testEngine(cap, nil)
	e.Step = func(ctx context.Context, key string, fn func(context.Context) (map[string]Value, error)) (map[string]Value, error) {
		mu.Lock()
		keys = append(keys, key)
		mu.Unlock()
		return fn(ctx)
	}
	res := exec(t, e, `{
	  "id":"m","name":"m",
	  "inputs":[{"name":"names","type":"list<text>","required":true}],
	  "nodes":[
	    {"id":"per","type":"control.map",
	     "inputs":{"items":{"from":"in.names"}},
	     "body":{"nodes":[
	        {"id":"t","type":"data.stringify","inputs":{"value":{"from":"item.value"}}}],
	      "outputs":{"result":{"from":"t.text"}}}}],
	  "outputs":{"o":{"from":"per.results"}}}`,
		map[string]any{"names": []any{"a", "b"}})
	if res.Err != nil {
		t.Fatalf("res=%+v", res)
	}
	want := map[string]bool{"node:per[0].t": true, "node:per[1].t": true}
	mu.Lock()
	defer mu.Unlock()
	for _, k := range keys {
		if k == "node:per" {
			t.Fatal("map node itself must not be step-wrapped")
		}
		delete(want, k)
	}
	if len(want) != 0 {
		t.Fatalf("missing keys %v in %v", want, keys)
	}
}
