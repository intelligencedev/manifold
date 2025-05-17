package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"manifold/internal/a2a/rpc"
	"manifold/internal/a2a/server"
)

type Authenticator interface {
	Authenticate(r *http.Request) error
}

type A2AClient struct {
	baseURL string
	http    *http.Client
	auth    Authenticator
}

func New(baseURL string, client *http.Client, auth Authenticator) *A2AClient {
	return &A2AClient{
		baseURL: baseURL,
		http:    client,
		auth:    auth,
	}
}

// do sends a JSON-RPC request to the server and unmarshals the result into the
// provided result structure. It handles authentication headers via the
// configured Authenticator.
func (c *A2AClient) do(ctx context.Context, method string, params interface{}, result interface{}) error {
	paramBytes, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal params: %w", err)
	}

	rpcReq := rpc.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		ID:      1,
		Params:  paramBytes,
	}

	body, err := json.Marshal(rpcReq)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.auth != nil {
		if err := c.auth.Authenticate(req); err != nil {
			return err
		}
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var rpcResp rpc.JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if rpcResp.Error != nil {
		return fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	if result == nil {
		return nil
	}

	// Re-marshal result field to decode into target structure
	resBytes, err := json.Marshal(rpcResp.Result)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	if err := json.Unmarshal(resBytes, result); err != nil {
		return fmt.Errorf("unmarshal result: %w", err)
	}
	return nil
}

// postStream sends a JSON-RPC request expecting an SSE response. It returns the
// HTTP response so the caller can process the stream.
func (c *A2AClient) postStream(ctx context.Context, method string, params interface{}) (*http.Response, error) {
	paramBytes, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}

	rpcReq := rpc.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		ID:      1,
		Params:  paramBytes,
	}

	body, err := json.Marshal(rpcReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if c.auth != nil {
		if err := c.auth.Authenticate(req); err != nil {
			return nil, err
		}
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return resp, nil
}

// streamResponses reads SSE events from the response body and sends parsed tasks
// on the returned channel.
func (c *A2AClient) streamResponses(resp *http.Response, ch chan<- SendTaskStreamingResponse) {
	defer resp.Body.Close()
	defer close(ch)

	scanner := bufio.NewScanner(resp.Body)
	var dataBuf strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			if strings.Contains(line, "close") {
				ch <- SendTaskStreamingResponse{Done: true}
				return
			}
			continue
		}
		if line == "" {
			if dataBuf.Len() == 0 {
				continue
			}
			payload := strings.TrimSpace(dataBuf.String())
			dataBuf.Reset()
			payload = strings.TrimPrefix(payload, "data:")
			payload = strings.TrimSpace(payload)
			var rpcResp rpc.JSONRPCResponse
			if err := json.Unmarshal([]byte(payload), &rpcResp); err != nil {
				ch <- SendTaskStreamingResponse{Error: err}
				continue
			}
			if rpcResp.Error != nil {
				ch <- SendTaskStreamingResponse{Error: fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)}
				continue
			}
			resBytes, err := json.Marshal(rpcResp.Result)
			if err != nil {
				ch <- SendTaskStreamingResponse{Error: err}
				continue
			}
			var res struct {
				Task *server.Task `json:"task"`
			}
			if err := json.Unmarshal(resBytes, &res); err != nil {
				ch <- SendTaskStreamingResponse{Error: err}
				continue
			}
			ch <- SendTaskStreamingResponse{Task: res.Task}
			continue
		}
		dataBuf.WriteString(line)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		ch <- SendTaskStreamingResponse{Error: err}
	}
}

// SendTask sends a task to the server using the tasks/send method.
func (c *A2AClient) SendTask(ctx context.Context, msg server.Message) (*server.Task, error) {
	req := server.SendRequest{Message: msg}
	var resp struct {
		Task server.Task `json:"task"`
	}
	if err := c.do(ctx, "tasks/send", req, &resp); err != nil {
		return nil, err
	}
	return &resp.Task, nil
}

// SendTaskSubscribe sends a task and subscribes to updates using SSE.
func (c *A2AClient) SendTaskSubscribe(ctx context.Context, msg server.Message) (<-chan SendTaskStreamingResponse, error) {
	req := server.SendRequest{Message: msg}
	resp, err := c.postStream(ctx, "tasks/sendSubscribe", req)
	if err != nil {
		return nil, err
	}
	ch := make(chan SendTaskStreamingResponse)
	go c.streamResponses(resp, ch)
	return ch, nil
}

// GetTask retrieves a task by ID.
func (c *A2AClient) GetTask(ctx context.Context, id string) (*server.Task, error) {
	req := server.GetRequest{TaskID: id}
	var resp struct {
		Task server.Task `json:"task"`
	}
	if err := c.do(ctx, "tasks/get", req, &resp); err != nil {
		return nil, err
	}
	return &resp.Task, nil
}

// CancelTask cancels a task by ID.
func (c *A2AClient) CancelTask(ctx context.Context, id string) (*server.Task, error) {
	req := server.CancelRequest{TaskID: id}
	var resp struct {
		Task server.Task `json:"task"`
	}
	if err := c.do(ctx, "tasks/cancel", req, &resp); err != nil {
		return nil, err
	}
	return &resp.Task, nil
}

// SetPushConfig sets the push notification configuration for a task.
func (c *A2AClient) SetPushConfig(ctx context.Context, id string, cfg server.PushNotificationConfig) error {
	req := server.PushSetRequest{TaskID: id, Config: cfg}
	var resp struct {
		Success bool `json:"success"`
	}
	return c.do(ctx, "tasks/pushNotification/set", req, &resp)
}

// GetPushConfig retrieves the push notification configuration for a task.
func (c *A2AClient) GetPushConfig(ctx context.Context, id string) (*server.PushNotificationConfig, error) {
	req := server.PushGetRequest{TaskID: id}
	var resp struct {
		Config server.PushNotificationConfig `json:"config"`
	}
	if err := c.do(ctx, "tasks/pushNotification/get", req, &resp); err != nil {
		return nil, err
	}
	return &resp.Config, nil
}

// Resubscribe streams task updates for an existing task using SSE.
func (c *A2AClient) Resubscribe(ctx context.Context, id string) (<-chan SendTaskStreamingResponse, error) {
	req := struct {
		TaskID string `json:"taskId"`
	}{TaskID: id}
	resp, err := c.postStream(ctx, "tasks/resubscribe", req)
	if err != nil {
		return nil, err
	}
	ch := make(chan SendTaskStreamingResponse)
	go c.streamResponses(resp, ch)
	return ch, nil
}
