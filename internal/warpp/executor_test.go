package warpp

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type capture struct {
	mu     sync.Mutex
	events []Event
}

func (c *capture) emit(ev Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev)
}

func (c *capture) byType(t EventType) []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []Event
	for _, ev := range c.events {
		if ev.Type == t {
			out = append(out, ev)
		}
	}
	return out
}

func (c *capture) nodeEvent(t EventType, path string) *Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.events {
		if c.events[i].Type == t && c.events[i].NodePath == path {
			ev := c.events[i]
			return &ev
		}
	}
	return nil
}

func testEngine(cap *capture, extra map[string]NodeRunner) *Engine {
	runners := BuiltinRunners()
	for k, v := range extra {
		runners[k] = v
	}
	return &Engine{
		Resolve:        BuiltinResolver(),
		Runners:        runners,
		Emit:           cap.emit,
		MaxConcurrency: 4,
	}
}

func customResolver(m Manifest) Resolver {
	return ChainResolvers(func(nt string) (Manifest, bool) {
		if nt == m.Type {
			return m, true
		}
		return Manifest{}, false
	}, BuiltinResolver())
}

func exec(t *testing.T, e *Engine, src string, input map[string]any) Result {
	t.Helper()
	var doc Document
	if err := json.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatalf("doc: %v", err)
	}
	if diags := Validate(doc, e.Resolve); HasErrors(diags) {
		t.Fatalf("doc invalid: %+v", diags)
	}
	return e.Execute(context.Background(), doc, input)
}

func TestExecuteLinearPipeline(t *testing.T) {
	cap := &capture{}
	res := exec(t, testEngine(cap, nil), `{
	  "id":"lin","name":"lin",
	  "inputs":[{"name":"topic","type":"text","required":true}],
	  "nodes":[
	    {"id":"tpl","type":"data.template",
	     "inputs":{"template":{"value":"about {t}"},
	               "vars":{"t":{"from":"in.topic"}}}}],
	  "outputs":{"out":{"from":"tpl.text"}}}`,
		map[string]any{"topic": "go"})
	if res.Err != nil || res.Status != StatusCompleted {
		t.Fatalf("res=%+v", res)
	}
	if res.Outputs["out"] != "about go" {
		t.Fatalf("outputs=%v", res.Outputs)
	}
	done := cap.nodeEvent(EventNodeCompleted, "tpl")
	if done == nil || done.Outputs["text"] != "about go" {
		t.Fatalf("node_completed must carry port outputs: %+v", done)
	}
	if len(cap.byType(EventRunCompleted)) != 1 {
		t.Fatal("run_completed missing")
	}
}

func TestExecuteMissingRequiredInputFails(t *testing.T) {
	cap := &capture{}
	res := exec(t, testEngine(cap, nil), `{
	  "id":"x","name":"x",
	  "inputs":[{"name":"topic","type":"text","required":true}],
	  "nodes":[{"id":"s","type":"data.stringify","inputs":{"value":{"from":"in.topic"}}}]}`,
		map[string]any{})
	if res.Status != StatusFailed || res.Err == nil {
		t.Fatalf("expected failed, got %+v", res)
	}
}

func TestIfSkipCascadeAndCoalesce(t *testing.T) {
	cap := &capture{}
	res := exec(t, testEngine(cap, nil), `{
	  "id":"br","name":"br",
	  "inputs":[{"name":"flag","type":"boolean","required":true}],
	  "nodes":[
	    {"id":"gate","type":"logic.if",
	     "inputs":{"condition":{"from":"in.flag"},"value":{"value":"yes"}}},
	    {"id":"up","type":"data.stringify","inputs":{"value":{"from":"gate.then"}}},
	    {"id":"down","type":"data.stringify","inputs":{"value":{"from":"gate.else"}}},
	    {"id":"after_down","type":"data.stringify","inputs":{"value":{"from":"down.text"}}},
	    {"id":"join","type":"logic.coalesce",
	     "inputs":{"values":[{"from":"up.text"},{"from":"after_down.text"}]}}],
	  "outputs":{"result":{"from":"join.value"}}}`,
		map[string]any{"flag": true})
	if res.Status != StatusCompletedWithSkips {
		t.Fatalf("status=%s err=%v", res.Status, res.Err)
	}
	if res.Outputs["result"] != "yes" {
		t.Fatalf("outputs=%v", res.Outputs)
	}
	if cap.nodeEvent(EventNodeSkipped, "down") == nil ||
		cap.nodeEvent(EventNodeSkipped, "after_down") == nil {
		t.Fatal("else branch must cascade-skip")
	}
	if cap.nodeEvent(EventNodeCompleted, "join") == nil {
		t.Fatal("coalesce must still complete")
	}
}

func TestOptionalInputSourceSkippedUsesDefault(t *testing.T) {
	man := Manifest{Type: "test.opt", Title: "Opt", Category: "test",
		Inputs: []PortSpec{
			{Name: "maybe", Type: "text", Default: "fallback"},
		},
		Outputs: []PortSpec{{Name: "out", Type: "text"}}}
	cap := &capture{}
	e := testEngine(cap, map[string]NodeRunner{
		"test.opt": func(ctx context.Context, rc RunnerCtx, in NodeInputs) (map[string]Value, error) {
			return map[string]Value{"out": in.Values["maybe"]}, nil
		}})
	e.Resolve = customResolver(man)
	res := exec(t, e, `{
	  "id":"opt","name":"opt",
	  "inputs":[{"name":"flag","type":"boolean","required":true}],
	  "nodes":[
	    {"id":"gate","type":"logic.if",
	     "inputs":{"condition":{"from":"in.flag"},"value":{"value":"wired"}}},
	    {"id":"n","type":"test.opt","inputs":{"maybe":{"from":"gate.then"}}}],
	  "outputs":{"o":{"from":"n.out"}}}`,
		map[string]any{"flag": false})
	// No node is skipped here (nothing else consumes gate.then), so the run
	// completes cleanly; the point is that n fell back to its default.
	if res.Status != StatusCompleted || res.Outputs["o"] != "fallback" {
		t.Fatalf("res=%+v", res)
	}
}

func TestRetriesThenSuccess(t *testing.T) {
	var calls atomic.Int32
	man := Manifest{Type: "test.flaky", Title: "F", Category: "test",
		Outputs: []PortSpec{{Name: "out", Type: "text"}}}
	cap := &capture{}
	e := testEngine(cap, map[string]NodeRunner{
		"test.flaky": func(ctx context.Context, rc RunnerCtx, in NodeInputs) (map[string]Value, error) {
			if calls.Add(1) < 3 {
				return nil, errors.New("boom")
			}
			return map[string]Value{"out": NewText("ok")}, nil
		}})
	e.Resolve = customResolver(man)
	res := exec(t, e, `{"id":"r","name":"r","nodes":[
	  {"id":"f","type":"test.flaky","policy":{"retries":{"max":2,"backoff":"fixed"}}}],
	  "outputs":{"o":{"from":"f.out"}}}`, nil)
	if res.Status != StatusCompleted || calls.Load() != 3 {
		t.Fatalf("res=%+v calls=%d", res, calls.Load())
	}
	if len(cap.byType(EventNodeRetrying)) != 2 {
		t.Fatalf("expected 2 retry events, got %d", len(cap.byType(EventNodeRetrying)))
	}
}

func TestOnErrorSkipContinuesRun(t *testing.T) {
	man := Manifest{Type: "test.fail", Title: "F", Category: "test",
		Outputs: []PortSpec{{Name: "out", Type: "text"}}}
	cap := &capture{}
	e := testEngine(cap, map[string]NodeRunner{
		"test.fail": func(ctx context.Context, rc RunnerCtx, in NodeInputs) (map[string]Value, error) {
			return nil, errors.New("always")
		}})
	e.Resolve = customResolver(man)
	res := exec(t, e, `{"id":"s","name":"s","nodes":[
	  {"id":"bad","type":"test.fail","policy":{"on_error":"skip"}},
	  {"id":"dep","type":"data.stringify","inputs":{"value":{"from":"bad.out"}}},
	  {"id":"solo","type":"data.constant","inputs":{"value":{"value":"alive"},"as":{"value":"text"}}}],
	  "outputs":{"o":{"from":"solo.value"}}}`, nil)
	if res.Status != StatusCompletedWithSkips {
		t.Fatalf("res=%+v", res)
	}
	if res.Outputs["o"] != "alive" {
		t.Fatalf("outputs=%v", res.Outputs)
	}
	if cap.nodeEvent(EventNodeFailed, "bad") == nil || cap.nodeEvent(EventNodeSkipped, "dep") == nil {
		t.Fatal("failed node must emit node_failed and cascade-skip dependents")
	}
}

func TestFatalErrorFailsRun(t *testing.T) {
	man := Manifest{Type: "test.fail", Title: "F", Category: "test",
		Outputs: []PortSpec{{Name: "out", Type: "text"}}}
	cap := &capture{}
	e := testEngine(cap, map[string]NodeRunner{
		"test.fail": func(ctx context.Context, rc RunnerCtx, in NodeInputs) (map[string]Value, error) {
			return nil, errors.New("fatal")
		}})
	e.Resolve = customResolver(man)
	res := exec(t, e, `{"id":"f","name":"f","nodes":[
	  {"id":"bad","type":"test.fail"}]}`, nil)
	if res.Status != StatusFailed || res.Err == nil {
		t.Fatalf("res=%+v", res)
	}
	if len(cap.byType(EventRunFailed)) != 1 {
		t.Fatal("run_failed missing")
	}
}

func TestTimeoutPolicy(t *testing.T) {
	man := Manifest{Type: "test.slow", Title: "S", Category: "test",
		Outputs: []PortSpec{{Name: "out", Type: "text"}}}
	cap := &capture{}
	e := testEngine(cap, map[string]NodeRunner{
		"test.slow": func(ctx context.Context, rc RunnerCtx, in NodeInputs) (map[string]Value, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
				return map[string]Value{"out": NewText("late")}, nil
			}
		}})
	e.Resolve = customResolver(man)
	start := time.Now()
	res := exec(t, e, `{"id":"t","name":"t","nodes":[
	  {"id":"slow","type":"test.slow","policy":{"timeout":"50ms"}}]}`, nil)
	if res.Status != StatusFailed {
		t.Fatalf("res=%+v", res)
	}
	if time.Since(start) > time.Second {
		t.Fatal("timeout did not apply")
	}
}

func TestStepFuncWrapsNodesWithPathKeys(t *testing.T) {
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
	res := exec(t, e, `{"id":"d","name":"d","nodes":[
	  {"id":"c","type":"data.constant","inputs":{"value":{"value":"x"},"as":{"value":"text"}}},
	  {"id":"s","type":"data.stringify","inputs":{"value":{"from":"c.value"}}}],
	  "outputs":{"o":{"from":"s.text"}}}`, nil)
	if res.Status != StatusCompleted {
		t.Fatalf("res=%+v", res)
	}
	want := map[string]bool{"node:c": true, "node:s": true}
	for _, k := range keys {
		delete(want, k)
	}
	if len(want) != 0 {
		t.Fatalf("missing step keys %v (got %v)", want, keys)
	}
}

func TestDiamondJoinBothBranchesFeedOneNode(t *testing.T) {
	cap := &capture{}
	res := exec(t, testEngine(cap, nil), `{
	  "id":"dia","name":"dia",
	  "inputs":[{"name":"x","type":"text","required":true}],
	  "nodes":[
	    {"id":"l","type":"data.template","inputs":{"template":{"value":"L{v}"},"vars":{"v":{"from":"in.x"}}}},
	    {"id":"r","type":"data.template","inputs":{"template":{"value":"R{v}"},"vars":{"v":{"from":"in.x"}}}},
	    {"id":"joinT","type":"data.template",
	     "inputs":{"template":{"value":"{a}|{b}"},
	               "vars":{"a":{"from":"l.text"},"b":{"from":"r.text"}}}}],
	  "outputs":{"o":{"from":"joinT.text"}}}`,
		map[string]any{"x": "1"})
	if res.Status != StatusCompleted || res.Outputs["o"] != "L1|R1" {
		t.Fatalf("res=%+v", res)
	}
}
