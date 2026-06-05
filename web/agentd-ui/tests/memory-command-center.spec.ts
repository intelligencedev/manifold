import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";

const apiMocks = vi.hoisted(() => ({
  approveMagmaEdge: vi.fn(),
  deleteMagmaNode: vi.fn(),
  drainMagmaConsolidation: vi.fn(),
  fetchMemoryObservabilityExplain: vi.fn(),
  fetchMemoryObservabilityGraph: vi.fn(),
  fetchMemoryObservabilityOverview: vi.fn(),
  fetchMemoryObservabilityReviewEdges: vi.fn(),
  fetchMemoryObservabilityTimeline: vi.fn(),
  pruneMagmaMemory: vi.fn(),
  rebuildEvolvingMemoryEmbeddings: vi.fn(),
  retractMagmaEdge: vi.fn(),
}));

vi.mock("@/api/memoryObservability", () => apiMocks);

vi.mock("@vue-flow/core", async () => {
  const { defineComponent, h } = await import("vue");
  return {
    MarkerType: { ArrowClosed: "arrowclosed" },
    VueFlow: defineComponent({
      name: "VueFlow",
      props: ["nodes", "edges"],
      emits: ["node-click", "edge-click", "pane-click"],
      setup(props, { emit, slots }) {
        return () =>
          h("div", { "data-test": "vue-flow" }, [
            ...(props.nodes ?? []).map((node: any) =>
              h(
                "button",
                {
                  "data-test": "flow-node",
                  "data-node-id": node.id,
                  type: "button",
                  onClick: () => emit("node-click", { node }),
                },
                node.label,
              ),
            ),
            slots.default?.(),
          ]);
      },
    }),
  };
});

vi.mock("@vue-flow/background", async () => {
  const { defineComponent, h } = await import("vue");
  return {
    Background: defineComponent({
      name: "Background",
      setup() {
        return () => h("div", { "data-test": "flow-background" });
      },
    }),
  };
});

vi.mock("@vue-flow/minimap", async () => {
  const { defineComponent, h } = await import("vue");
  return {
    MiniMap: defineComponent({
      name: "MiniMap",
      setup() {
        return () => h("div", { "data-test": "flow-minimap" });
      },
    }),
  };
});

enableAutoUnmount(afterEach);

function overviewResponse() {
  return {
    timestamp: 1,
    source: "test",
    config: {
      memoryEnabled: true,
      evolvingEnabled: true,
      beliefEnabled: true,
      magmaEnabled: true,
    },
    totals: {
      searches: 0,
      hits: 0,
      avgHitsPerSearch: 0,
      evolves: 0,
      evolveErrors: 0,
      smartMerges: 0,
      pruned: 0,
    },
    latency: { avgMs: 0 },
    graph: { nodes: 1, edges: 0, events: 1, entities: 0, reviewEdges: 0 },
    magma: {
      enabled: true,
      maintenanceEnabled: true,
      queueDepth: 0,
      processedTotal: 0,
      failedTotal: 0,
      droppedTotal: 0,
    },
    lanes: [],
  };
}

function graphResponse(nodes: any[]) {
  return {
    timestamp: 1,
    graph: {
      nodes: nodes.length,
      edges: 0,
      events: nodes.filter((node) => node.type === "event").length,
      entities: nodes.filter((node) => node.type === "entity").length,
      reviewEdges: 0,
    },
    nodes,
    edges: [],
  };
}

describe("MemoryCommandCenter", () => {
  beforeEach(() => {
    for (const mock of Object.values(apiMocks)) {
      mock.mockReset();
    }
    vi.stubGlobal(
      "confirm",
      vi.fn(() => true),
    );
    apiMocks.fetchMemoryObservabilityOverview.mockResolvedValue(
      overviewResponse(),
    );
    apiMocks.fetchMemoryObservabilityTimeline.mockResolvedValue({
      timestamp: 1,
      items: [],
    });
    apiMocks.fetchMemoryObservabilityReviewEdges.mockResolvedValue({
      timestamp: 1,
      edges: [],
    });
    apiMocks.deleteMagmaNode.mockResolvedValue({
      ok: true,
      message: "Graph memory node deleted.",
    });
  });

  it("deletes a selected event graph node from the inspector", async () => {
    const eventNode = {
      id: "event:user:0:memory-1",
      type: "event",
      label: "Memory node",
      tenant: "user:0",
      session: "sess-1",
      text: "Delete this graph memory node.",
      createdAt: new Date().toISOString(),
    };
    apiMocks.fetchMemoryObservabilityGraph
      .mockResolvedValueOnce(graphResponse([eventNode]))
      .mockResolvedValue(graphResponse([]));

    const { default: MemoryCommandCenter } =
      await import("@/components/observability/MemoryCommandCenter.vue");
    const wrapper = mount(MemoryCommandCenter, {
      props: { timeRange: "1h" },
    });

    await flushPromises();
    await flushPromises();

    await wrapper
      .get('[data-node-id="event:user:0:memory-1"]')
      .trigger("click");
    await flushPromises();

    const deleteButton = wrapper
      .findAll("button")
      .find((button) => button.text().trim() === "Delete");
    expect(deleteButton).toBeTruthy();

    await deleteButton!.trigger("click");
    await flushPromises();
    await flushPromises();

    expect(window.confirm).toHaveBeenCalledWith(
      "Delete graph memory node Memory node?",
    );
    expect(apiMocks.deleteMagmaNode).toHaveBeenCalledWith({
      nodeId: "event:user:0:memory-1",
    });
    expect(wrapper.text()).toContain("Graph memory node deleted.");
  });
});
