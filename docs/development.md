# Development

## Setup

### Prerequisites
- Go 1.21+ (recommended: Go 1.24+)
- Node.js and pnpm (for frontend development)
- Docker and Docker Compose (for local development with services)

### Installation

1. Clone the repository:
```bash
git clone https://github.com/yourusername/manifold.git
cd manifold
```

2. Install Go dependencies:
```bash
go mod download
```

3. Install frontend dependencies:
```bash
cd web/agentd-ui
pnpm install
```

## Building

### Backend

```bash
# Build all binaries
make build

# Or build specific binaries
go build ./cmd/agent
go build ./cmd/agentd
go build ./cmd/orchestrator
```

### Frontend

```bash
cd web/agentd-ui

# Development server
pnpm dev

# Production build
pnpm build

# Preview production build
pnpm preview
```

## Testing

### Backend Tests

```bash
# Run all tests
go test ./...

# Run tests with race detection
go test -race ./...

# Run tests with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Frontend Tests

```bash
cd web/agentd-ui

# Unit tests
pnpm test:unit

# End-to-end tests
pnpm test:e2e
```

## Code Quality

### Linting

```bash
# Go linting
go vet ./...
make lint  # Requires golangci-lint

# Frontend linting
cd web/agentd-ui
pnpm lint
```

### Formatting

```bash
# Go formatting
go fmt ./...
gofmt -w .

# Frontend formatting
cd web/agentd-ui
pnpm format
```

## Frontend Development

### Development Workflow

1. Start the backend in development mode:
```bash
go run ./cmd/agentd
```

2. Start the frontend development server:
```bash
cd web/agentd-ui
pnpm dev
```

3. Set up development proxy:
```bash
FRONTEND_DEV_PROXY=http://localhost:5173 go run ./cmd/agentd
```

### Technology Stack

- **Framework**: Vue.js 3 with Composition API
- **Build Tool**: Vite
- **Styling**: Tailwind CSS
- **Testing**: Vitest (unit) + Playwright (e2e)

## Contributing

### Workflow

1. Open an issue to discuss significant changes
2. Fork the repository and create a feature branch
3. Make your changes following the coding conventions
4. Add tests for new functionality
5. Run the full test suite
6. Submit a pull request with a clear description

### Coding Conventions

See [AGENTS.md](../AGENTS.md) for detailed Go coding conventions and best practices.

### Git Workflow

- Use conventional commits for commit messages
- Keep commits small and focused

## Database Migration Notes

Upgrading from a pre-multi-tenant release requires adding ownership metadata to
existing chat transcripts. For Postgres deployments run:

```sql
ALTER TABLE chat_sessions ADD COLUMN IF NOT EXISTS user_id BIGINT;
CREATE INDEX IF NOT EXISTS chat_sessions_user_updated_idx ON chat_sessions(user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS chat_sessions_user_created_idx ON chat_sessions(user_id, created_at DESC);
```

Existing sessions will have `user_id = NULL` and are only visible to admins.
As users sign in, new sessions are created with their user ID. Optionally backfill
legacy sessions by updating `chat_sessions.user_id` with the appropriate `users.id`
if ownership is known.
- Rebase feature branches before merging
- Use descriptive branch names
