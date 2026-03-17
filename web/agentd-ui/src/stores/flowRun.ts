import { defineStore } from "pinia";
import { ref } from "vue";
import type { FlowRunResult, FlowStepTrace } from "@/types/flowV2";
import { runFlowWorkflow } from "@/api/flow";

export const useFlowRunStore = defineStore("flow-run", () => {
  const running = ref(false);
  const error = ref("");
  const runOutput = ref("");
  const runLogs = ref<string[]>([]);
  const runTrace = ref<Record<string, FlowStepTrace>>({});
  let runAbort: AbortController | null = null;

  function reset() {
    error.value = "";
    runOutput.value = "";
    runLogs.value = [];
    runTrace.value = {};
  }

  async function startRun(
    workflowId: string,
    prompt?: string,
    projectId?: string,
  ): Promise<FlowRunResult> {
    if (running.value) throw new Error("A run is already in progress");
    running.value = true;
    reset();
    runLogs.value.push(`▶ Starting run for workflow "${workflowId}"`);
    runAbort?.abort();
    runAbort = new AbortController();
    try {
      runLogs.value.push("→ POST /api/flows/v2/run");
      const res = await runFlowWorkflow(
        workflowId,
        prompt ?? `Run workflow: ${workflowId}`,
        runAbort.signal,
        projectId,
      );
      runOutput.value = res.result || "";
      const rec: Record<string, FlowStepTrace> = {};
      for (const trace of res.trace ?? []) {
        rec[trace.stepId] = trace;
      }
      runTrace.value = rec;
      runLogs.value.push("✓ Run finished");
      if (runOutput.value) {
        const snippet =
          runOutput.value.slice(0, 160) +
          (runOutput.value.length > 160 ? "…" : "");
        runLogs.value.push("Result snippet: " + snippet);
      }
      return res;
    } catch (err: any) {
      if (err?.name === "AbortError") {
        error.value = "Run cancelled";
        runLogs.value.push("⚠ Run cancelled by user");
        return { result: "", trace: [] };
      }
      const msg = err?.message ?? "Failed to run workflow";
      error.value = msg;
      runLogs.value.push("✗ Error: " + msg);
      throw err;
    } finally {
      running.value = false;
    }
  }

  function cancelRun() {
    if (running.value && runAbort) runAbort.abort();
  }

  return { running, error, runOutput, runLogs, runTrace, startRun, cancelRun };
});