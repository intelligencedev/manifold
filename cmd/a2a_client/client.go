package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

// TaskState represents the state of a task.
type TaskState string

const (
	Submitted     TaskState = "submitted"
	Working       TaskState = "working"
	InputRequired TaskState = "input-required"
	Completed     TaskState = "completed"
	Canceled      TaskState = "canceled"
	Failed        TaskState = "failed"
)

// Task represents a task with its status.
type Task struct {
	ID     string    `json:"id"`
	Status TaskState `json:"status"`
}

// JSONRPCRequest represents a JSON-RPC request.
type JSONRPCRequest struct {
	Jsonrpc string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      int         `json:"id"`
}

// JSONRPCResponse represents a JSON-RPC response.
type JSONRPCResponse struct {
	Jsonrpc string      `json:"jsonrpc"`
	Result  interface{} `json:"result"`
	Error   *RPCError   `json:"error"`
	ID      int         `json:"id"`
}

// RPCError represents a JSON-RPC error.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

// A2AClient represents the A2A client.
type A2AClient struct {
	baseURL string
	client  *http.Client
}

// NewA2AClient creates a new A2A client.
func NewA2AClient(baseURL string) *A2AClient {
	return &A2AClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// SendTask sends a task to the A2A server.
func (c *A2AClient) SendTask(taskID string) error {
	task := Task{ID: taskID, Status: Submitted}
	requestBody, err := json.Marshal(JSONRPCRequest{
		Jsonrpc: "2.0",
		Method:  "tasks/send",
		Params:  task,
		ID:      1,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.client.Post(c.baseURL+"/tasks/send", "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("server error: %s", string(body))
	}

	return nil
}

// GetTask retrieves the status of a task.
func (c *A2AClient) GetTask(taskID string) (Task, error) {
	resp, err := c.client.Get(fmt.Sprintf("%s/tasks/get?id=%s", c.baseURL, taskID))
	if err != nil {
		return Task{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return Task{}, fmt.Errorf("server error: %s", string(body))
	}

	var task Task
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		return Task{}, fmt.Errorf("failed to decode response: %w", err)
	}

	return task, nil
}

func main() {
	client := NewA2AClient("http://localhost:8080")

	taskID := "12345"
	if err := client.SendTask(taskID); err != nil {
		fmt.Println("Error sending task:", err)
		return
	}

	time.Sleep(2 * time.Second) // Wait for the task to process

	task, err := client.GetTask(taskID)
	if err != nil {
		fmt.Println("Error getting task:", err)
		return
	}

	fmt.Printf("Task ID: %s, Status: %s\n", task.ID, task.Status)
}
