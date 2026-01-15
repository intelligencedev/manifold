import { describe, expect, it, vi, beforeEach } from "vitest";
import { flushPromises } from "@vue/test-utils";
import { mount } from "@vue/test-utils";
import { VueQueryPlugin, QueryClient } from "@tanstack/vue-query";

vi.mock("vue3-grid-layout-next/dist/style.css", () => ({}));
vi.mock("@/components/DashboardGrid.vue", () => ({
  default: {
    template:
      "<div>" +
      "<slot name='item-tokens' />" +
      "<slot name='item-traces' />" +
      "<slot name='item-memory' />" +
      "<slot name='item-agents' />" +
      "<slot name='item-runs' />" +
      "</div>",
  },
}));

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
  listSpecialists: () =>
    Promise.resolve([
      {
        name: "orchestrator",
        model: "gpt-5",
      },
    ]),
}));

describe("OverviewView", () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = new QueryClient();
  });

  it("renders stats once data resolves", async () => {
    const { default: OverviewView } = await import("@/views/OverviewView.vue");
    const wrapper = mount(OverviewView, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
      },
    });

    await flushPromises();

    expect(wrapper.text()).toContain("Active Agents");
    expect(wrapper.text()).toContain("Runs Today");
    expect(wrapper.text()).toContain("Specialists");
    expect(wrapper.text()).toContain("Primary Agent");
  });
});
