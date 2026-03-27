import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { QueryClient, VueQueryPlugin } from "@tanstack/vue-query";

const fetchMemorySessions = vi.fn();
const fetchMemorySessionDebug = vi.fn();
const fetchEvolvingMemory = vi.fn();

vi.mock("@/api/memory", () => ({
  fetchMemorySessions,
  fetchMemorySessionDebug,
  fetchEvolvingMemory,
}));

vi.mock("@/components/DropdownSelect.vue", () => ({
  default: {
    props: ["modelValue", "options"],
    emits: ["update:modelValue"],
    template: "<div data-test='dropdown-select' />",
  },
}));

describe("MemoryPanel", () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = new QueryClient();
    fetchMemorySessions.mockReset();
    fetchMemorySessionDebug.mockReset();
    fetchEvolvingMemory.mockReset();
  });

  it("auto-selects the first session and renders evolving memories", async () => {
    fetchMemorySessions.mockResolvedValue([
      {
        id: "sess-1",
        name: "Session One",
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      },
    ]);
    fetchMemorySessionDebug.mockResolvedValue({
      session: { id: "sess-1" },
      summary: "Existing summary",
      summarizedCount: 1,
      messages: [],
      plan: {
        mode: "summary",
        contextWindowTokens: 128000,
        targetUtilizationPct: 0.75,
        tailTokenBudget: 2000,
        minKeepLastMessages: 8,
        maxSummaryChunkTokens: 4000,
        estimatedHistoryTokens: 100,
        estimatedTailTokens: 25,
        tailStartIndex: 0,
        totalMessages: 1,
      },
    });
    fetchEvolvingMemory.mockResolvedValue({
      enabled: true,
      totalEntries: 1,
      topK: 4,
      maxSize: 100,
      windowSize: 20,
      recentWindow: [
        {
          id: "mem-1",
          input: "remember this",
          output: "stored output",
          feedback: "success",
          summary: "reusable lesson",
          created_at: new Date().toISOString(),
        },
      ],
    });

    const { default: MemoryPanel } = await import(
      "@/components/observability/MemoryPanel.vue"
    );
    const wrapper = mount(MemoryPanel, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
      },
    });

    await flushPromises();
    await flushPromises();

    expect(fetchMemorySessionDebug).toHaveBeenCalledWith("sess-1");
    expect(fetchEvolvingMemory).toHaveBeenCalledWith(undefined, "sess-1");
    expect(wrapper.text()).toContain("1 entries");
    expect(wrapper.text()).toContain("remember this");
  });

  it("shows evolving memory even when the chat summary endpoint returns 404", async () => {
    fetchMemorySessions.mockResolvedValue([
      {
        id: "memory-only",
        name: "",
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      },
    ]);
    fetchMemorySessionDebug.mockRejectedValue({
      response: { status: 404 },
    });
    fetchEvolvingMemory.mockResolvedValue({
      enabled: true,
      totalEntries: 1,
      topK: 4,
      maxSize: 100,
      windowSize: 20,
      recentWindow: [
        {
          id: "mem-404",
          input: "memory-only prompt",
          output: "memory-only output",
          feedback: "success",
          summary: "memory-only summary",
          created_at: new Date().toISOString(),
        },
      ],
    });

    const { default: MemoryPanel } = await import(
      "@/components/observability/MemoryPanel.vue"
    );
    const wrapper = mount(MemoryPanel, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
      },
    });

    await flushPromises();
    await flushPromises();

    expect(wrapper.text()).toContain("No chat summary is available for this session yet.");
    expect(wrapper.text()).toContain("memory-only prompt");
    expect(wrapper.text()).not.toContain("Failed to load session memory");
  });
});