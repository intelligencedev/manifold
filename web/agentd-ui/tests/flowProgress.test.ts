import { describe, expect, it } from "vitest";

import { activeNodeIdsFromEvents } from "@/api/flow";
import type { FlowV2RunEvent } from "@/types/flowV2";

describe("flow run progress", () => {
  it("keeps parallel diamond branches active until each branch completes", () => {
    const events: FlowV2RunEvent[] = [
      { type: "run_started", status: "running" },
      { type: "node_started", node_id: "root", status: "running" },
      { type: "node_completed", node_id: "root", status: "completed" },
      { type: "node_started", node_id: "branch_b", status: "running" },
      { type: "node_started", node_id: "branch_c", status: "running" },
    ];

    expect(activeNodeIdsFromEvents(events).sort()).toEqual([
      "branch_b",
      "branch_c",
    ]);

    expect(
      activeNodeIdsFromEvents([
        ...events,
        { type: "node_completed", node_id: "branch_b", status: "completed" },
      ]),
    ).toEqual(["branch_c"]);

    expect(
      activeNodeIdsFromEvents([
        ...events,
        { type: "node_completed", node_id: "branch_b", status: "completed" },
        { type: "node_completed", node_id: "branch_c", status: "completed" },
        { type: "node_started", node_id: "join", status: "running" },
      ]),
    ).toEqual(["join"]);

    expect(
      activeNodeIdsFromEvents([
        ...events,
        { type: "node_completed", node_id: "branch_b", status: "completed" },
        { type: "node_completed", node_id: "branch_c", status: "completed" },
        { type: "node_started", node_id: "join", status: "running" },
        { type: "node_completed", node_id: "join", status: "completed" },
        { type: "run_completed", status: "completed" },
      ]),
    ).toEqual([]);
  });
});
