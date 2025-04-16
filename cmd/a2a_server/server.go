package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
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

// InMemoryTaskStore stores tasks in memory.
type InMemoryTaskStore struct {
	sync.Mutex
	tasks map[string]Task
}

// NewInMemoryTaskStore initializes a new in-memory task store.
func NewInMemoryTaskStore() *InMemoryTaskStore {
	return &InMemoryTaskStore{
		tasks: make(map[string]Task),
	}
}

// AddTask adds a task to the store.
func (store *InMemoryTaskStore) AddTask(task Task) {
	store.Lock()
	defer store.Unlock()
	store.tasks[task.ID] = task
}

// UpdateTask updates the status of a task.
func (store *InMemoryTaskStore) UpdateTask(id string, state TaskState) error {
	store.Lock()
	defer store.Unlock()
	if task, exists := store.tasks[id]; exists {
		task.Status = state
		store.tasks[id] = task
		return nil
	}
	return fmt.Errorf("task not found")
}

// GetTask retrieves a task by its ID.
func (store *InMemoryTaskStore) GetTask(id string) (Task, error) {
	store.Lock()
	defer store.Unlock()
	if task, exists := store.tasks[id]; exists {
		return task, nil
	}
	return Task{}, fmt.Errorf("task not found")
}

// A2AServer represents the A2A server.
type A2AServer struct {
	store *InMemoryTaskStore
}

// NewA2AServer creates a new A2A server instance.
func NewA2AServer(store *InMemoryTaskStore) *A2AServer {
	return &A2AServer{store: store}
}

// handleTaskSend handles task/send requests.
func (server *A2AServer) handleTaskSend(w http.ResponseWriter, r *http.Request) {
	var task Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	server.store.AddTask(Task{ID: task.ID, Status: Submitted})

	go server.processTask(task.ID)

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(task)
}

// processTask simulates task processing.
func (server *A2AServer) processTask(taskID string) {
	time.Sleep(1 * time.Second) // Simulate processing time
	server.store.UpdateTask(taskID, Working)

	// Simulate task completion
	time.Sleep(1 * time.Second)
	server.store.UpdateTask(taskID, Completed)
}

// handleTaskGet handles task/get requests.
func (server *A2AServer) handleTaskGet(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("id")
	task, err := server.store.GetTask(taskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(task)
}

// Start starts the A2A server.
func (server *A2AServer) Start() {
	http.HandleFunc("/tasks/send", server.handleTaskSend)
	http.HandleFunc("/tasks/get", server.handleTaskGet)
	log.Println("A2A Server started on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func main() {
	store := NewInMemoryTaskStore()
	server := NewA2AServer(store)
	server.Start()
}
