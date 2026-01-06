import { describe, expect, it, vi, beforeEach } from "vitest";
import { flushPromises } from "@vue/test-utils";
import { mount } from "@vue/test-utils";
import { VueQueryPlugin, QueryClient } from "@tanstack/vue-query";
import OverviewView from "@/views/OverviewView.vue";

vi.mock("@/api/client", () => ({
  fetchAgentStatus: () =>
    Promise.resolve([
      {
        id: "agent-1",
        name: "Primary Agent",
        state: "online",
        model: "gpt-5",
        updatedAt: new Date().toISOString(),
      },
    ]),
  fetchAgentRuns: () =>
    Promise.resolve([
      {
        id: "run-123",
        prompt: "status",
        createdAt: new Date().toISOString(),
        status: "completed",
        tokens: 120,
      },
    ]),
}));

describe("OverviewView", () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = new QueryClient();
  });

  it("renders stats once data resolves", async () => {
    const wrapper = mount(OverviewView, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
      },
    });

    await flushPromises();

    expect(wrapper.text()).toContain("Active Agents");
    expect(wrapper.text()).toContain("Runs Today");
    expect(wrapper.text()).toContain("Avg. Prompt Tokens");
    expect(wrapper.text()).toContain("Primary Agent");
  });
});
