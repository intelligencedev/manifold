package multitool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"manifold/internal/agent/inputrequest"
	"manifold/internal/durable"
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
	for i := range 2 {
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
	for i := range 3 {
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
	var count atomic.Int64
	reg.Register(fakeTool{
		name: "alpha",
		call: func(ctx context.Context, raw json.RawMessage) (any, error) {
			return map[string]int64{"call": count.Add(1)}, nil
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

func TestParallelToolLiftsFlatAskAgentShape(t *testing.T) {
	reg := tools.NewRegistry()
	type call struct {
		To     string `json:"to"`
		Prompt string `json:"prompt"`
	}
	var mu sync.Mutex
	seen := make([]call, 0, 2)
	reg.Register(fakeTool{
		name: "ask_agent",
		call: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var payload call
			require.NoError(t, json.Unmarshal(raw, &payload))
			mu.Lock()
			seen = append(seen, payload)
			mu.Unlock()
			return map[string]any{"ok": true}, nil
		},
	})

	pt := NewParallel(reg)
	reg.Register(pt)

	// Local models often emit this flat shape (siblings instead of nested
	// parameters). Mix in a fully-correct call to ensure both work together.
	raw := json.RawMessage(`{"tool_uses":[
		{"recipient_name":"functions.ask_agent","to":"researcher","prompt":"find X"},
		{"recipient_name":"ask_agent","parameters":{"to":"writer","prompt":"draft Y"}}
	]}`)

	payload, err := reg.Dispatch(context.Background(), pt.Name(), raw)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(payload, &parsed))
	require.True(t, parsed["ok"].(bool), "payload: %s", string(payload))

	mu.Lock()
	defer mu.Unlock()
	require.ElementsMatch(t, []call{
		{To: "researcher", Prompt: "find X"},
		{To: "writer", Prompt: "draft Y"},
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

func TestParallelToolUsesDispatchRegistryFromContext(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(fakeTool{
		name: "allowed",
		call: func(ctx context.Context, raw json.RawMessage) (any, error) {
			return map[string]any{"ok": true}, nil
		},
	})
	reg.Register(fakeTool{
		name: "blocked",
		call: func(ctx context.Context, raw json.RawMessage) (any, error) {
			return map[string]any{"ok": true}, nil
		},
	})
	pt := NewParallel(reg)
	reg.Register(pt)

	filtered := tools.NewFilteredRegistry(reg, []string{pt.Name(), "allowed"})
	raw := json.RawMessage(`{"tool_uses":[{"recipient_name":"functions.blocked","parameters":{}}]}`)
	payload, err := filtered.Dispatch(context.Background(), pt.Name(), raw)
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
	require.Contains(t, parsed.Error, "blocked")
	require.Len(t, parsed.Results, 1)
	require.Contains(t, parsed.Results[0].Error, "tool not allowed")
}

func TestParallelToolVisibleInFilteredRegistryWhenAllowListed(t *testing.T) {
	reg := tools.NewRegistry()
	pt := NewParallel(reg)
	reg.Register(pt)

	filtered := tools.NewFilteredRegistry(reg, []string{pt.Name()})
	require.Contains(t, tools.SchemaNames(filtered), pt.Name())
}

func TestParallelToolUsesNestedToolDispatcher(t *testing.T) {
	reg := tools.NewRegistry()
	pt := NewParallel(reg)
	reg.Register(pt)

	raw := json.RawMessage(`{"tool_uses":[{"recipient_name":"functions.agent_call","tool_call_id":"child-1","parameters":{"prompt":"write"}}]}`)
	ctx := tools.WithNestedToolDispatcher(context.Background(), func(ctx context.Context, name string, raw json.RawMessage, toolCallID string) ([]byte, bool) {
		require.Equal(t, "agent_call", name)
		require.Equal(t, "child-1", toolCallID)
		return []byte(`{"ok":true,"output":"nested"}`), true
	})

	payload, err := reg.Dispatch(ctx, pt.Name(), raw)
	require.NoError(t, err)

	var parsed struct {
		OK      bool `json:"ok"`
		Results []struct {
			Payload json.RawMessage `json:"payload"`
			Error   string          `json:"error"`
		} `json:"results"`
	}
	require.NoError(t, json.Unmarshal(payload, &parsed))
	require.True(t, parsed.OK)
	require.Len(t, parsed.Results, 1)
	require.Empty(t, parsed.Results[0].Error)
	require.JSONEq(t, `{"ok":true,"output":"nested"}`, string(parsed.Results[0].Payload))
}

func TestParallelToolPropagatesErrSuspended(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(fakeTool{
		name: "needs_approval",
		call: func(ctx context.Context, raw json.RawMessage) (any, error) {
			return nil, durable.ErrSuspended
		},
	})
	reg.Register(fakeTool{
		name: "quick",
		call: func(ctx context.Context, raw json.RawMessage) (any, error) {
			return map[string]any{"ok": true}, nil
		},
	})
	pt := NewParallel(reg, WithMaxParallel(2))
	reg.Register(pt)

	raw, err := json.Marshal(map[string]any{
		"tool_uses": []map[string]any{
			{"recipient_name": "functions.quick", "tool_call_id": "ok-1"},
			{"recipient_name": "functions.needs_approval", "tool_call_id": "suspend-1"},
		},
	})
	require.NoError(t, err)

	payload, callErr := reg.Dispatch(context.Background(), pt.Name(), raw)
	require.ErrorIs(t, callErr, durable.ErrSuspended)
	require.Nil(t, payload)
}

func TestParallelToolIsolatesNestedToolMetadata(t *testing.T) {
	reg := tools.NewRegistry()
	var mu sync.Mutex
	toolIDs := make([]string, 0, 2)
	reg.Register(fakeTool{
		name: "probe",
		call: func(ctx context.Context, raw json.RawMessage) (any, error) {
			meta := inputrequest.RunMetadataFromContext(ctx)
			mu.Lock()
			toolIDs = append(toolIDs, meta.ToolID)
			mu.Unlock()
			return map[string]any{"ok": true, "tool_id": meta.ToolID}, nil
		},
	})
	pt := NewParallel(reg)
	reg.Register(pt)

	ctx := inputrequest.WithRunMetadata(context.Background(), inputrequest.RunMetadata{
		ToolID: "parent-parallel-id",
	})
	raw, err := json.Marshal(map[string]any{
		"tool_uses": []map[string]any{
			{"recipient_name": "functions.probe", "tool_call_id": "child-a"},
			{"recipient_name": "functions.probe", "tool_call_id": "child-b"},
		},
	})
	require.NoError(t, err)

	payload, callErr := reg.Dispatch(ctx, pt.Name(), raw)
	require.NoError(t, callErr)
	require.NotNil(t, payload)

	mu.Lock()
	defer mu.Unlock()
	require.ElementsMatch(t, []string{"child-a", "child-b"}, toolIDs)
}

func TestParallelToolCheckpointsCompletedChildrenAcrossSuspend(t *testing.T) {
	ctx := context.Background()
	store := durable.NewMemoryStore()
	client := durable.NewClient(store)
	registry := durable.NewRegistry()
	var calls atomic.Int64
	registry.Register(durable.DefaultQueue, "parallel_suspend", func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		reg := tools.NewRegistry()
		reg.Register(fakeTool{
			name: "once",
			call: func(ctx context.Context, raw json.RawMessage) (any, error) {
				n := calls.Add(1)
				return map[string]any{"calls": n}, nil
			},
		})
		reg.Register(fakeTool{
			name: "gate",
			call: func(ctx context.Context, raw json.RawMessage) (any, error) {
				return durable.AwaitEvent[map[string]any](ctx, "continue", 0)
			},
		})
		pt := NewParallel(reg, WithMaxParallel(2))
		reg.Register(pt)

		raw := json.RawMessage(`{"tool_uses":[{"recipient_name":"functions.once","tool_call_id":"once-1"},{"recipient_name":"functions.gate","tool_call_id":"gate-1"}]}`)
		payload, err := reg.Dispatch(ctx, pt.Name(), raw)
		if err != nil {
			return nil, err
		}
		var parsed map[string]any
		if err := json.Unmarshal(payload, &parsed); err != nil {
			return nil, err
		}
		return parsed, nil
	})

	worker := durable.NewWorker(store, client, registry, durable.WorkerOptions{WorkerID: "worker-parallel", Lease: time.Minute, PollInterval: 20 * time.Millisecond})
	spawn, err := client.Spawn(ctx, durable.SpawnRequest{
		UserID: 7,
		Queue:  durable.DefaultQueue,
		Name:   "parallel_suspend",
		Params: map[string]any{},
	})
	require.NoError(t, err)
	require.NoError(t, worker.RunOnce(ctx))

	task, found, err := store.GetTask(ctx, 7, spawn.TaskID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, durable.TaskStatusWaiting, task.Status)

	_, err = client.EmitEvent(ctx, 7, durable.DefaultQueue, "continue", map[string]any{"approved": true})
	require.NoError(t, err)
	require.NoError(t, worker.RunOnce(ctx))

	snapshot, err := client.FetchResult(ctx, 7, spawn.TaskID)
	require.NoError(t, err)
	require.Equal(t, durable.TaskStatusCompleted, snapshot.State)

	var parsed struct {
		OK      bool `json:"ok"`
		Results []struct {
			ToolCallID string          `json:"tool_call_id"`
			Payload    json.RawMessage `json:"payload"`
			Error      string          `json:"error"`
		} `json:"results"`
	}
	require.NoError(t, json.Unmarshal(snapshot.Result, &parsed))
	require.True(t, parsed.OK)
	require.Len(t, parsed.Results, 2)
	require.Equal(t, int64(1), calls.Load())

	for _, item := range parsed.Results {
		require.Empty(t, item.Error)
		if item.ToolCallID == "once-1" {
			require.JSONEq(t, `{"calls":1}`, string(item.Payload))
		}
	}
}

