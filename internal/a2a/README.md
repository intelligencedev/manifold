# A2A Package

## Overview
The `a2a` package provides a comprehensive framework for asynchronous-to-asynchronous (A2A) task processing and communication in distributed systems. It offers a robust infrastructure to manage tasks, agents, and messaging using JSON-RPC 2.0 over HTTP, along with support for Server-Sent Events (SSE) and push notifications.

## Key Components

### Models
Defines core data structures such as `AgentCard`, `AgentProvider`, `AgentCapabilities`, `AgentAuthentication`, `AgentSkill`, `Task`, `Message`, and various content parts (e.g., `TextPart`, `FilePart`). These models represent the entities involved in the A2A interactions.

### Server
Implements an HTTP server that uses JSON-RPC 2.0 to handle remote procedure calls for managing tasks and agents. Key features include:
- RPC router for method registration and dispatching
- Authentication middleware supporting pluggable authenticators
- In-memory task store for task management

### Client
A client library to interact with the A2A server, supporting authenticated HTTP requests and task operations.

### Auth
Provides interface and implementations for authenticators, including a no-op authenticator.

### Push
Handles sending push notifications to configured webhook URLs with optional authentication.

### SSE
Implements Server-Sent Events (SSE) support to stream JSON-RPC responses over HTTP.

### RPC
Defines JSON-RPC 2.0 request and response formats, router, and handler functions to process RPC calls.

### Errors
Defines custom JSON-RPC error codes and structures for standardized error reporting.

## Usage

### Server Setup
To create a new server instance:

```go
store := server.NewInMemory() // or other implementation
auth := auth.NewNoop() // or custom authenticator
srv := server.NewServer(store, auth)

http.Handle("/rpc", server.Authenticate(srv, auth))
http.ListenAndServe(":8080", nil)
```

### Client Usage
Create a new A2A client to invoke RPC methods:

```go
client := client.New("http://localhost:8080/rpc", &http.Client{}, auth.NewNoop())
// Use client to send tasks, subscribe, etc.
```

### Push Notifications
Configure push notifications for tasks and send notifications:

```go
cfg := models.PushNotificationConfig{
    WebhookURL: "https://example.com/webhook",
    AuthToken: "token",
    Events: []string{"task.completed"},
}
err := push.SendPush(ctx, cfg, payload)
```

### SSE Streaming
Use SSEWriter to stream JSON-RPC responses over HTTP:

```go
sseWriter := sse.NewSSEWriter(w)
sseWriter.Send(resp)
```

## Contributing
Contributions are welcome. Please ensure code quality and tests for new features.

## License
Specify your license here.

---

This README provides an overview and guide to the `a2a` package, helping developers understand and use its features effectively.