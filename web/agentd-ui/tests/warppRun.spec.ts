import { beforeEach, describe, expect, it, vi } from "vitest";
import { createPinia, setActivePinia } from "pinia";

vi.mock("@/api/warpp", () => ({
  startRun: vi.fn(async () => ({ run_id: "r1", status: "running" })),
  streamRunEvents: vi.fn(() => () => {}),
}));

import { useWarppRun } from "@/stores/warppRun";
import type { WarppRunEvent } from "@/types/warpp";

function ev(partial: Partial<WarppRunEvent>): WarppRunEvent {
  return {
    run_id: "r1",
    sequence: 0,
    type: "",
    occurred_at: "",
    ...partial,
  };
}

describe("warppRun ingest", () => {
  beforeEach(() => setActivePinia(createPinia()));

  it("folds a scripted event sequence into node and run state", () => {
    const store = useWarppRun();
    store.ingest(ev({ type: "run_started", status: "running" }));
    store.ingest(ev({ type: "node_started", node_path: "tpl" }));
    store.ingest(
      ev({
        type: "node_completed",
        node_path: "tpl",
        outputs: { text: "about go" },
      }),
    );
    store.ingest(ev({ type: "node_skipped", node_path: "down" }));
    store.ingest(
      ev({ type: "run_completed", status: "completed_with_skips" }),
    );

    expect(store.nodeStatus.tpl).toBe("completed");
    expect(store.nodeStatus.down).toBe("skipped");
    expect(store.nodeOutputs.tpl.text).toBe("about go");
    expect(store.status).toBe("completed_with_skips");
    expect(store.events).toHaveLength(5);
  });
});
