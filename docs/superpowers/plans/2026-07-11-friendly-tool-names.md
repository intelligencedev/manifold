# Descriptive Friendly Tool Names Implementation Plan

**Goal:** Replace Manifold’s terse internal-tool titles with engaging, descriptive activity labels that match the tools’ runtime names.

**Architecture:** Keep the existing centralized `friendlyToolTitles` map and `HumanizeToolName` fallback unchanged. Update the map entries only, correcting the two stale keys (`llm_parallel_completions` and `matrix_room_message`), and add table-driven regression coverage for the complete runtime mapping and tracer output.

**Tech Stack:** Go 1.26.3, standard `testing` package, `gofmt`.

## Global Constraints

- Keep the title behavior centralized in `internal/tools/titles.go`.
- Use sentence-case, action-oriented labels suitable for UI activity events.
- Preserve the dotted `multi_tool_use.parallel` alias.
- Do not modify unrelated existing worktree changes.
- Do not commit unless explicitly requested.

### Task 1: Add the failing title-mapping regression test

- [x] Add and run the failing table-driven regression test.

**Files:**
- Modify: `internal/tools/registry_test.go`

Add a table-driven test that calls `HumanizeToolName` for every supported runtime name, including the dotted parallel alias, and asserts the approved replacement label. Run the focused test before changing production metadata; it must fail against the old titles.

### Task 2: Replace the centralized friendly-name metadata

- [x] Replace the centralized map and correct runtime tool-name keys.

**Files:**
- Modify: `internal/tools/titles.go`

Replace the old labels with the approved action-oriented labels, use runtime keys `llm_parallel_completions` and `matrix_room_message`, and retain both parallel aliases.

### Task 3: Format and verify

- [x] Format changed Go files and run focused plus repository-wide verification.

**Files:**
- No additional files.
- Modify: `internal/tools/agents/delegator_test.go` to update its expected `run_cli` activity title.

Run `go fmt internal/tools/titles.go internal/tools/registry_test.go internal/tools/agents/delegator_test.go`, then run `go test ./internal/tools`, the focused tracer test, and `go test ./... -run '.*' -skip '^TestDelegatorRunUsesSharedEvolvingMemory$'`. The unskipped full suite still has the pre-existing memory-test failure because its 13-character prompt is rejected by the 20-character minimum in `storeExperience`.
