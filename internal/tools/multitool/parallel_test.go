package multitool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"manifold/internal/tools"
)

type fakeTool struct {
	name string
	call func(ctx context.Context, raw json.RawMessage) (any, error)
}

func (f fakeTool) Name() string               { return f.name }
func (f fakeTool) JSONSchema() map[string]any { return map[string]any{} }
func (f fakeTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	return f.call(ctx, raw)
}

func TestParallelToolExecutesConcurrently(t *testing.T) {
	reg := tools.NewRegistry()
	start := make(chan string, 2)
	release := make(chan struct{})
	blocker := func(label string) tools.Tool {
		return fakeTool{
			name: label,
			call: func(ctx context.Context, raw json.RawMessage) (any, error) {
				select {
				case start <- label:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				select {
				case <-release:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				return map[string]any{"label": label}, nil
			},
		}
	}
	reg.Register(blocker("first"))
	reg.Register(blocker("second"))

	pt := NewParallel(reg, WithMaxParallel(2))
	reg.Register(pt)

	args := map[string]any{
		"tool_uses": []map[string]any{
			{"recipient_name": "functions.first"},
			{"recipient_name": "functions.second"},
		},
	}
	raw, err := json.Marshal(args)
	require.NoError(t, err)

	type outcome struct {
		payload []byte
		err     error
	}
	resultCh := make(chan outcome, 1)
	go func() {
		payload, callErr := reg.Dispatch(context.Background(), pt.Name(), raw)
		resultCh <- outcome{payload: payload, err: callErr}
	}()

	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case label := <-start:
			seen[label] = true
		case <-time.After(250 * time.Millisecond):
			t.Fatalf("tool %d did not start concurrently", i)
		}
	}
	close(release)
	out := <-resultCh
	require.NoError(t, out.err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(out.payload, &result))

	require.True(t, seen["first"] && seen["second"], "expected both tools to start")
	require.True(t, result["ok"].(bool))

	resSlice, ok := result["results"].([]any)
	require.True(t, ok)
	require.Len(t, resSlice, 2)
}

func TestParallelToolPropagatesErrors(t *testing.T) {
	reg := tools.NewRegistry()
	errTool := fakeTool{
		name: "flaky",
		call: func(ctx context.Context, raw json.RawMessage) (any, error) {
			return nil, errors.New("boom")
		},
	}
	reg.Register(errTool)

	pt := NewParallel(reg, WithMaxParallel(1))
	reg.Register(pt)

	raw, err := json.Marshal(map[string]any{
		"tool_uses": []map[string]any{
			{"recipient_name": "functions.flaky"},
		},
	})
	require.NoError(t, err)

	payload, callErr := reg.Dispatch(context.Background(), pt.Name(), raw)
	require.NoError(t, callErr)

	var parsed struct {
		OK      bool `json:"ok"`
		Results []struct {
			Error string `json:"error"`
		} `json:"results"`
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(payload, &parsed))
	require.False(t, parsed.OK)
	require.Contains(t, parsed.Error, "flaky")
	require.Len(t, parsed.Results, 1)
	require.Contains(t, parsed.Results[0].Error, "boom")
}

func TestParallelToolRejectsInvalidRecipient(t *testing.T) {
	reg := tools.NewRegistry()
	pt := NewParallel(reg)
	reg.Register(pt)

	raw, err := json.Marshal(map[string]any{
		"tool_uses": []map[string]any{
			{"recipient_name": "functions."},
		},
	})
	require.NoError(t, err)

	payload, callErr := reg.Dispatch(context.Background(), pt.Name(), raw)
	require.NoError(t, callErr)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(payload, &parsed))
	require.False(t, parsed["ok"].(bool))
}

func TestParallelToolLimitsConcurrency(t *testing.T) {
	reg := tools.NewRegistry()
	var concurrent int
	var mu sync.Mutex
	maxObserved := 0
	makeTool := func(name string) tools.Tool {
		return fakeTool{
			name: name,
			call: func(ctx context.Context, raw json.RawMessage) (any, error) {
				mu.Lock()
				concurrent++
				if concurrent > maxObserved {
					maxObserved = concurrent
				}
				mu.Unlock()
				time.Sleep(50 * time.Millisecond)
				mu.Lock()
				concurrent--
				mu.Unlock()
				return map[string]string{"name": name}, nil
			},
		}
	}
	for i := 0; i < 3; i++ {
		reg.Register(makeTool(fmt.Sprintf("tool_%d", i)))
	}

	pt := NewParallel(reg, WithMaxParallel(2))
	reg.Register(pt)

	raw, err := json.Marshal(map[string]any{
		"tool_uses": []map[string]any{
			{"recipient_name": "functions.tool_0"},
			{"recipient_name": "functions.tool_1"},
			{"recipient_name": "functions.tool_2"},
		},
	})
	require.NoError(t, err)

	_, callErr := reg.Dispatch(context.Background(), pt.Name(), raw)
	require.NoError(t, callErr)
	require.Equal(t, 2, maxObserved)
}

func TestParallelToolParsesArrayArguments(t *testing.T) {
	reg := tools.NewRegistry()
	calls := make(chan string, 2)
	reg.Register(fakeTool{
		name: "first",
		call: func(ctx context.Context, raw json.RawMessage) (any, error) {
			calls <- "first"
			return map[string]string{"ok": "true"}, nil
		},
	})
	reg.Register(fakeTool{
		name: "second",
		call: func(ctx context.Context, raw json.RawMessage) (any, error) {
			calls <- "second"
			return map[string]string{"ok": "true"}, nil
		},
	})

	pt := NewParallel(reg)
	reg.Register(pt)

	raw := json.RawMessage(`[
		{"recipient_name":"functions.first","parameters":{}},
		{"recipient_name":"functions.second","parameters":{}}
	]`)

	payload, err := reg.Dispatch(context.Background(), pt.Name(), raw)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(payload, &parsed))
	require.True(t, parsed["ok"].(bool))
	require.Len(t, parsed["results"].([]any), 2)
	got := []string{<-calls, <-calls}
	require.ElementsMatch(t, []string{"first", "second"}, got)
}

func TestParallelToolParsesStreamedArguments(t *testing.T) {
	reg := tools.NewRegistry()
	count := 0
	reg.Register(fakeTool{
		name: "alpha",
		call: func(ctx context.Context, raw json.RawMessage) (any, error) {
			count++
			return map[string]int{"call": count}, nil
		},
	})

	pt := NewParallel(reg)
	reg.Register(pt)

	raw := json.RawMessage(`{"recipient_name":"functions.alpha","parameters":{}}
{"recipient_name":"functions.alpha","parameters":{}}`)

	payload, err := reg.Dispatch(context.Background(), pt.Name(), raw)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(payload, &parsed))
	require.True(t, parsed["ok"].(bool))
	require.Len(t, parsed["results"].([]any), 2)
}

func TestParallelToolInfersRunCLI(t *testing.T) {
	reg := tools.NewRegistry()
	type call struct {
		Command string
		Args    []string
	}
	var mu sync.Mutex
	seen := make([]call, 0, 2)
	reg.Register(fakeTool{
		name: "run_cli",
		call: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var payload struct {
				Command string   `json:"command"`
				Args    []string `json:"args"`
			}
			require.NoError(t, json.Unmarshal(raw, &payload))
			mu.Lock()
			seen = append(seen, call{Command: payload.Command, Args: payload.Args})
			mu.Unlock()
			return map[string]any{"ok": true}, nil
		},
	})

	pt := NewParallel(reg)
	reg.Register(pt)

	raw := json.RawMessage(`[
		{"command":"echo","args":["hi"]},
		{"command":"echo","args":["there"]}
	]`)

	payload, err := reg.Dispatch(context.Background(), pt.Name(), raw)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(payload, &parsed))
	require.True(t, parsed["ok"].(bool))
	require.Len(t, parsed["results"].([]any), 2)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, seen, 2)
	require.ElementsMatch(t, []call{
		{Command: "echo", Args: []string{"hi"}},
		{Command: "echo", Args: []string{"there"}},
	}, seen)
}

func TestParallelToolDetectsEmbeddedErrors(t *testing.T) {
	reg := tools.NewRegistry()
	// register a tool that always returns an error payload without returning Go error
	reg.Register(fakeTool{
		name: "faulty",
		call: func(ctx context.Context, raw json.RawMessage) (any, error) {
			// mimic default registry behaviour when tool fails internally
			return map[string]any{"ok": false, "error": "simulated failure"}, nil
		},
	})

	pt := NewParallel(reg)
	reg.Register(pt)

	raw := json.RawMessage(`[{"recipient_name":"functions.faulty","parameters":{}}]`)
	payload, err := reg.Dispatch(context.Background(), pt.Name(), raw)
	require.NoError(t, err)

	var parsed struct {
		OK      bool `json:"ok"`
		Results []struct {
			Error string `json:"error"`
		} `json:"results"`
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(payload, &parsed))
	require.False(t, parsed.OK)
	require.Contains(t, parsed.Error, "faulty")
	require.Len(t, parsed.Results, 1)
	require.Contains(t, parsed.Results[0].Error, "simulated failure")
}

func TestDetectEmbeddedError(t *testing.T) {
	require.Equal(t, "", detectEmbeddedError([]byte(`null`)))
	require.Equal(t, "tool not found", detectEmbeddedError([]byte(`{"error":"tool not found"}`)))
	require.Equal(t, "simulated", detectEmbeddedError([]byte(`{"ok":false,"error":"simulated"}`)))
	require.Equal(t, "", detectEmbeddedError([]byte(`{"ok":true}`)))
}
