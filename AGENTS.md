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

Understand that Go ≥1.22 adds safer loop vars, integer/iterator ranges, generic type aliases, Swiss-maps, weak pointers, `os.Root`, `testing.B.Loop`, new crypto/FIPS pkgs, etc. See tables above for one-liner idioms.

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

## Testing Requirements

Every exported function and method **must** have unit tests. Aim for ≥ 80 % coverage on business logic.

### Testing Conventions

- Use Go’s built‑in `testing` package with **stretchr/testify** for assertions and mocks, or **gomock** if interfaces are extensive. |
- Prefer table‑driven tests for clarity and coverage.                                                                                |
- Call `t.Parallel()` where safe to speed up the suite.                                                                              |
- Generate mocks via `mockgen` or `counterfeiter`.                                                                                   |
- Include benchmarks for performance‑critical code (`go test -bench`).                                                               |

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
