import { fireEvent, render, screen, waitFor } from "@testing-library/vue";
import { describe, expect, it, vi } from "vitest";

import CodeQaView from "@/views/CodeQaView.vue";

const routeState = vi.hoisted(() => ({ params: { runId: "run-1" } }));
const routerMocks = vi.hoisted(() => ({
  push: vi.fn(async (to: { params?: { runId?: string } }) => {
    routeState.params.runId = to.params?.runId ?? "";
  }),
}));

const apiMocks = vi.hoisted(() => ({
  listCodeQARuns: vi.fn(async () => [
    {
      run_id: "run-1",
      mode: "judge",
      status: "running",
      repository: "/Users/art/Documents/manifold",
      diff: {
        base_ref: "HEAD~1",
        head_ref: "HEAD",
        files: [{ path: "foo/foo.go", status: "M", related_tests: ["foo/foo_test.go"] }],
        unified_diff: "diff --git a/foo/foo.go b/foo/foo.go",
        truncated: false,
      },
      gates: [],
      judges: [],
      aggregate: { quality_delta: 0.12, confidence: 0.84, action: "accept", rationale: "looks good" },
      started_at: "2026-04-24T12:00:00Z",
    },
  ]),
  fetchCodeQARun: vi.fn(async (runId: string) => ({
    run_id: runId,
    mode: "judge",
    status: "running",
    repository: "/Users/art/Documents/manifold",
    diff: {
      base_ref: "HEAD~1",
      head_ref: "HEAD",
      files: [{ path: "foo/foo.go", status: "M", related_tests: ["foo/foo_test.go"] }],
      unified_diff: "diff --git a/foo/foo.go b/foo/foo.go",
      truncated: false,
    },
    gates: [{ name: "go_test", ref: "HEAD", ok: true, hard_fail: false, duration_ms: 120 }],
    judges: [{ judge_id: "judge-tests", verdict: "after_better", confidence: 0.9, scores: { test_quality: 0.2 }, swap_applied: false }],
    aggregate: { quality_delta: 0.12, confidence: 0.84, action: "accept", rationale: "looks good" },
    started_at: "2026-04-24T12:00:00Z",
  })),
  fetchCodeQARunEvents: vi.fn(async (runId: string) => ({
    run_id: runId,
    status: "running",
    events: [
      { run_id: runId, sequence: 1, type: "run_started", occurred_at: "2026-04-24T12:00:00Z", payload: { repository: "/Users/art/Documents/manifold" } },
    ],
  })),
  startCodeQARun: vi.fn(async () => ({ run_id: "run-2", status: "running" })),
  streamCodeQARunEvents: vi.fn((runId: string, onEvent: (event: any) => void) => {
    onEvent({ run_id: runId, sequence: 2, type: "gates_completed", occurred_at: "2026-04-24T12:00:01Z", payload: { gate_count: 4 } });
    return () => {};
  }),
}));

vi.mock("vue-router", async () => {
  const actual = await vi.importActual<typeof import("vue-router")>("vue-router");
  return {
    ...actual,
    useRoute: () => routeState,
    useRouter: () => routerMocks,
  };
});

vi.mock("@/api/codeqa", () => apiMocks);

describe("CodeQaView", () => {
  it("renders run detail and starts a new run", async () => {
    routeState.params.runId = "run-1";
    render(CodeQaView);

    expect(await screen.findByText(/Code Quality Control Room/i)).toBeTruthy();
    expect(await screen.findByText(/Stored Runs/i)).toBeTruthy();
    expect(await screen.findByText(/judge-tests/i)).toBeTruthy();
    expect(await screen.findByText(/run started/i)).toBeTruthy();

    const baseRef = screen.getByPlaceholderText("HEAD~1");
    await fireEvent.update(baseRef, "master");
    await fireEvent.click(screen.getByRole("button", { name: /Start Code QA/i }));

    await waitFor(() => {
      expect(apiMocks.startCodeQARun).toHaveBeenCalledTimes(1);
      expect(routerMocks.push).toHaveBeenCalledWith({ name: "codeqa", params: { runId: "run-2" } });
    });
  });
});