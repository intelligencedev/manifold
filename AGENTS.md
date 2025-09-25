# Go/intelligence.dev

## Coding Conventions

## Project Structure & Module Organization

Source lives in `internal/` (e.g., `internal/agent`, `internal/orchestrator`); keep new packages focused on one concern and avoid import cycles. CLI/Server entrypoints sit under `cmd/` with binaries for the agent, HTTP server (`agentd`), and `embedctl` (if present). Docs reside in `docs/`, assets in `assets/`, and deployment scaffolding in `docker/`, `configs/`, and top-level `example.env`. Co-locate tests with their packages and share fakes through `internal/testhelpers`.

### Package Organization

* Keep packages small and focused on a single responsibility.
* Avoid cyclical dependencies; extract interfaces when necessary.

### Dependency Injection

* Promote testability via interface‑driven design.
* Use constructor functions (e.g., `NewService(...)`) to inject dependencies.
* Consider a lightweight DI library **only** after weighing complexity (e.g., `uber-go/fx`).

## Build, Test, and Development Commands
`go run ./cmd/agent -q "status"` exercises the CLI; `go run ./cmd/agentd` starts the HTTP + web UI server. Use `make build` for host binaries (rebuilds Whisper artifacts). `make test` wraps `go test -race -coverprofile=coverage.out ./...`; view coverage with `go tool cover -func coverage.out`. `make ci` chains format, import checks, lint, and tests. If Whisper bindings drift, rerun `make whisper-go-bindings`.

## Essential Go CLI Tools

The following Go command-line tools are essential for development, testing, and maintenance in this project. Use them as described to ensure code quality and consistency:

| Tool         | Purpose                                                                                                 |
| ------------ | ------------------------------------------------------------------------------------------------------- |
| `go build`   | Compiles packages and their dependencies into an executable.                                            |
| `go run`     | Compiles and runs the specified Go program.                                                             |
| `go fmt`     | Formats Go source code according to the language's style guidelines.                                    |
| `gofmt`      | Standalone formatter; also available as an executable.                                                  |
| `go test`    | Runs tests and benchmarks. Use `-coverprofile` with `go tool cover` to analyze test coverage.           |
| `go vet`     | Examines Go source code and reports suspicious constructs that could be bugs.                           |
| `go doc`     | Extracts and generates documentation for Go packages.                                                   |
| `go get`     | Adds, updates, or removes dependencies in the `go.mod` file.                                            |
| `go mod`     | Provides access to module operations (e.g., `go mod tidy` to clean up dependencies).                    |
| `go tool`    | Runs the specified Go tool (see below for examples).                                                    |
| `cgo`        | Enables the creation of Go packages that call C code.                                                   |
| `pprof`      | For profiling Go programs.                                                                              |
| `fix`        | Rewrites Go programs that use old language and library features.                                        |

> **Tip:** Refer to this table whenever you need to build, test, format, or analyze Go code in this project.

## Coding Style & Naming Conventions
Target Go 1.24.5 and keep files `gofmt` clean with tabs. Maintain import order with `goimports` and keep `golangci-lint run` (via `make lint`) silent. Name packages after their capability, keep filenames lowercase, and export concise CamelCase APIs. Prefer constructor-style functions (e.g., `NewService`) for dependency injection.

### Go Language Features (1.22+)

| feature                | micro-example                                                |
| ---------------------- | ------------------------------------------------------------ |
| loop-var capture fixed | `for _,v:=range xs{ go func(x string){println(x)}(v) }`      |
| integer ranges         | `for i:=range 10{ println(9-i) }`                            |
| `slices.Concat`        | `all:=slices.Concat(a,b,c)`                                  |
| `database/sql.Null[T]` | `var n sql.Null[int64]; rows.Scan(&n); if n.Valid{use(n.V)}` |
| `math/rand/v2`         | `import r "math/rand/v2"; v:=r.IntN(10)`                   |
| HTTP patterns          | `mux.Handle("GET /task/{id}", h)`                          |

| feature                       | micro-example                                              |
| ----------------------------- | ---------------------------------------------------------- |
| iterator `range`              | `for v:=range slices.Values([]int{1,2,3}){fmt.Println(v)}` |
| generic type alias            | `type Vec[T]=[]T // GOEXPERIMENT=aliastypeparams`          |
| `unique` intern               | `h:=unique.Make(s); m[h]=val`                              |
| timers GC+0-cap               | `t:=time.NewTimer(d); <-t.C /* cap(t.C)==0 */`             |
| telemetry                     | `$ go telemetry on`                                        |
| `go env -changed`             | `$ go env -changed`                                        |

| feature            | micro-example                                           |
| ------------------ | ------------------------------------------------------- |
| generic alias GA   | `type Map[K comparable,V any]=map[K]V`                  |
| Swiss-table maps   | *(automatic, no code)*                                  |
| weak ptr + cleanup | `w:=weak.Make(obj); runtime.AddCleanup(&obj,free)`      |
| dir-scoped FS      | `r,_:=os.OpenRoot("/data"); r.Open("foo.txt")`       |
| `testing.B.Loop`   | `func BenchmarkX(b *testing.B){ for b.Loop(){ x() } }`  |
| FIPS/crypto pkgs   | `key:=hkdf.Extract(...); _,_ = mlkem.GenerateKey(nil)`  |
| `go:wasmexport`    | `//go:wasmexport add\nfunc add(a,b int)int{return a+b}` |

> Drop-in prompt chunk: “**Understand that Go ≥1.22 adds safer loop vars, integer/iterator ranges, generic type aliases, Swiss-maps, weak pointers, `os.Root`, `testing.B.Loop`, new crypto/FIPS pkgs, etc. See tables above for one-liner idioms.**”

---

// ROUTING — Go 1.23

```
mux.Handle("GET /item/{id}", h)               // ServeMux: method + wildcard ★1
log.Print(r.Pattern)                          // Request.Pattern ★2
http.SetCookie(w,&http.Cookie{                // Quoted + Partitioned cookies ★3
  Name:"sid", Value:`"x"`, Quoted:true, Partitioned:true})
dup := r.CookiesNamed("sid")                  // get all "sid" cookies ★3
vals,_ := http.ParseCookie(`a=1; a=2`)        // ParseCookie helper ★4
c,_   := http.ParseSetCookie(`u=1; Path=/`)   // ParseSetCookie helper ★4
req := httptest.NewRequestWithContext(ctx,    // context-aware test request ★5
        http.MethodGet,"/",nil)

// ROUTING — Go 1.24
tr  := &http.Transport{MaxResponseHeaderBytes:64<<10}        // size-based 1xx cap ★6
srv := &http.Server{Handler:mux,                             // protocol matrix + h2c ★7
       Protocols:[]http.Protocol{http.UnencryptedHTTP2},
       HTTP2:&http2.Server{IdleTimeout:time.Minute}}         // HTTP/2 tuning ★7
tr.Protocols = []http.Protocol{http.UnencryptedHTTP2}        // client side ★7

// TESTING — Go 1.24
func BenchmarkFoo(b *testing.B){ for b.Loop(){ foo() } }     // testing.B.Loop ★8
func TestFoo(t *testing.T){ t.Chdir(tmp); work(t.Context()) }// T/B.Chdir & Context ★9
synctest.Run(func(ctx context.Context){                      // deterministic bubble ★10
  go work(ctx); synctest.Wait()
})
```

---

## Testing Requirements

> **Philosophy**: Every exported function and method **must** have unit tests. Aim for ≥ 80 % coverage on business logic.

### Testing Conventions

| Guideline        | Details                                                                                                                            |
| ---------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| **Frameworks**   | Use Go’s built‑in `testing` package with **stretchr/testify** for assertions and mocks, or **gomock** if interfaces are extensive. |
| **Table‑Driven** | Prefer table‑driven tests for clarity and coverage.                                                                                |
| **Parallelism**  | Call `t.Parallel()` where safe to speed up the suite.                                                                              |
| **Mocks/Fakes**  | Generate mocks via `mockgen` or `counterfeiter`.                                                                                   |
| **Benchmarks**   | Include benchmarks for performance‑critical code (`go test -bench`).                                                               |

### Common Commands

```bash
# Run all unit tests
go test ./...

# Run tests with race detector
go test -race ./...

# Run tests with coverage
go test -cover ./...

# Continuous profiling (CPU & memory)
go test -run=^$ -bench=. -benchmem ./...
```

---

## Pull Request Guidelines

When OpenAI Codex assists in crafting a PR:

1. **Description** — Provide a clear summary of *why* and *what* changed.
2. **Issue Links** — Reference related issues (e.g., `Fixes #42`).
3. **Tests** — Ensure **all new and existing tests pass** (`go test ./...`).
4. **Linters** — Pass `golangci‑lint run` with **zero** warnings.
5. **Screenshots/Logs** — Include relevant evidence for UI or API changes.
6. **Atomic Commits** — Keep PRs focused on a single concern; split large refactors.

---

## Programmatic Checks

Before merging, run the following (often via CI):

```bash
# Format & import cleanup
goimports -w $(go list -f '{{.Dir}}' ./...)

go fmt ./...

# Static analysis & vet
go vet ./...

golangci-lint run

# Dependency hygiene
go mod tidy

# Build validation
go build ./...

# Tests & coverage
go test -race -cover ./...
```

All commands **must** succeed with **no** warnings or errors.

Breaking changes in openai‑go v2.0.0

The transition from v1.12.0 of the openai‑go SDK to v2.0.0 introduces a handful of breaking changes.  Many of these changes come from a redesign that simplifies request/response handling and removes several helper types.  The table below summarises the most significant breaking changes.  When upgrading an existing project, update your code and environment accordingly.

Renaming of Chat Completion tools
    • Function tools renamed to custom tools – With the addition of custom tools in Chat Completions, the old function‑tool concept has been renamed.  The ChatCompletionToolParam type is now a union called ChatCompletionToolUnionParam, and function tools are created via openai.ChatCompletionFunctionTool(...).  The changelog provides an example of migrating a function tool to the new API ￼.

Import path and minimum Go version
    • Import path changed – All packages must now be imported with the /v2 suffix (for example, github.com/openai/openai‑go/v2 instead of github.com/openai/openai‑go) ￼.  Update your module imports accordingly.
    • Minimum Go version increased – The SDK now requires Go 1.21+ (previously 1.18+).  This requirement is documented in the updated README ￼.

Removal of openai.F and param.Field
    • No more openai.F(...) helper – The helper function used to wrap values (openai.F) has been removed.  You can simply delete calls to openai.F(...) ￼.  For optional primitives, use openai.String, openai.Int etc. instead ￼.
    • param.Field[T] removed – Request structs no longer use param.Field[T].  Instead, required primitives are plain types (e.g., int64, string) and optional primitives are wrapped in param.Opt[T] with an ,omitzero tag ￼.  During migration, ensure required fields are explicitly set because zero values will now be serialized ￼.

omitzero struct tags
    • Use omitzero to omit fields – The new SDK relies on the json:"...,omitzero" struct tag to omit zero‑value fields.  This replaces the old openai.F/param.Field[T] approach ￼.  Structures, slices, maps and string enums should be tagged with omitzero when optional; required primitives omit this tag.

Union types redesigned
    • Interfaces replaced by struct unions – Request union types are no longer Go interfaces.  Instead, each union type is a struct with fields for each variant (e.g., AnimalUnionParam has OfCat and OfDog pointers) ￼.  This design eliminates type assertions; call Get… methods on the union to access shared fields ￼.
    • Chat completion types renamed – Numerous union and option types have been renamed for clarity.  For example, ChatCompletionMessageToolCallParam is now ChatCompletionMessageToolCallUnionParam and similar changes apply to other …UnionParam types ￼.  Update type names and constructors accordingly.

Handling null and extra fields
    • Changing how null values are sent – The old param.Null[T]() function operated on param.Field[T].  In v2, use param.Null[T]() to set a param.Opt[T] to null and param.NullStruct[T]() for struct parameters ￼.
    • Custom values via WithExtraField – The helper openai.Raw[T](any) has been removed.  To set a field to a custom type or value not represented by the SDK types, call .WithExtraField(map[string]any) on the request struct ￼.

Changes to response accessors
    • IsNull() replaced by Valid() – Optional response fields no longer provide an IsNull() method.  Instead, call .Valid() to test whether an optional field is present ￼.  The migration guide gives a table comparing the new and old methods ￼.
    • RawJSON() moved – The RawJSON() accessor has been moved up one level: call resp.Foo.RawJSON() instead of resp.Foo.JSON.RawJSON() ￼.

Other notes
    • HTML escaping behavior – JSON marshaling no longer escapes HTML by default; however, the client supports an option to re‑enable escaping ￼.  While not strictly breaking, this may change how certain strings are serialized.
    • New models and endpoints – v2 adds support for GPT‑5 models and additional API features ￼.  These are additive changes but may influence defaults (e.g., shared.ChatModelGPT5 replaces shared.ChatModelGPT4_1 in examples ￼).

⸻

When upgrading from v1.12.0 to v2.0.0, start by switching import paths and ensuring your Go version satisfies the new requirement.  Then adjust request structs to remove openai.F(...) and param.Field[T], add appropriate omitzero tags, and rename any union types.  Review your usage of null and extra fields, and update response checks to use .Valid() instead of .IsNull().
