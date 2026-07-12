import { defineStore } from "pinia";
import { ref } from "vue";
import { startRun, streamRunEvents } from "@/api/warpp";
import type { WarppRunEvent } from "@/types/warpp";

export const useWarppRun = defineStore("warppRun", () => {
  const runId = ref<string | null>(null);
  const status = ref<string>("");
  const events = ref<WarppRunEvent[]>([]);
  const nodeStatus = ref<Record<string, string>>({});
  const nodeOutputs = ref<Record<string, Record<string, unknown>>>({});
  let cancel: (() => void) | null = null;

  function reset(): void {
    runId.value = null;
    status.value = "";
    events.value = [];
    nodeStatus.value = {};
    nodeOutputs.value = {};
  }

  // ingest folds one event into node/run state. Exported for testing.
  function ingest(ev: WarppRunEvent): void {
    events.value.push(ev);
    switch (ev.type) {
      case "node_started":
        if (ev.node_path) nodeStatus.value[ev.node_path] = "running";
        break;
      case "node_completed":
        if (ev.node_path) {
          nodeStatus.value[ev.node_path] = "completed";
          if (ev.outputs) nodeOutputs.value[ev.node_path] = ev.outputs;
        }
        break;
      case "node_failed":
        if (ev.node_path) nodeStatus.value[ev.node_path] = "failed";
        break;
      case "node_skipped":
        if (ev.node_path) nodeStatus.value[ev.node_path] = "skipped";
        break;
      case "node_retrying":
        if (ev.node_path) nodeStatus.value[ev.node_path] = "retrying";
        break;
      case "run_started":
        status.value = ev.status || "running";
        break;
      case "run_completed":
      case "run_failed":
      case "run_cancelled":
        status.value = ev.status || ev.type.replace("run_", "");
        break;
    }
  }

  async function start(
    workflowId: string,
    input: Record<string, unknown>,
  ): Promise<void> {
    reset();
    const resp = await startRun(workflowId, input);
    runId.value = resp.run_id;
    status.value = resp.status;
    cancel = streamRunEvents(
      resp.run_id,
      (ev) => ingest(ev),
      () => {
        cancel = null;
      },
    );
  }

  function stop(): void {
    if (cancel) {
      cancel();
      cancel = null;
    }
  }

  return {
    runId,
    status,
    events,
    nodeStatus,
    nodeOutputs,
    reset,
    ingest,
    start,
    stop,
  };
});
