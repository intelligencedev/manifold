import { flushPromises, mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";

vi.hoisted(() => {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: () => ({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
    }),
  });
});

vi.mock("@/api/client", () => ({
  fetchAgentdSettings: vi.fn().mockResolvedValue({
    serverConfig: { memory: { enabled: true, retrieval: { timeoutMs: 50 } } },
    configSource: "memory:\n  enabled: true\n",
  }),
  updateAgentdSettings: vi.fn(),
}));

vi.mock("@/api/mcp", () => ({
  deleteMCPServer: vi.fn(),
  listMCPServers: vi.fn().mockResolvedValue([]),
  startMCPOAuth: vi.fn(),
}));

import SettingsView from "@/views/SettingsView.vue";

function settingsNavButton(wrapper: ReturnType<typeof mount>, label: string) {
  const button = wrapper
    .findAll("aside nav button")
    .find((candidate) => candidate.text() === label);
  if (!button) throw new Error(`Missing settings navigation item: ${label}`);
  return button;
}

describe("SettingsView", () => {
  it("keeps runtime execution controls together in the safety section", async () => {
    const wrapper = mount(SettingsView);
    await flushPromises();

    await settingsNavButton(wrapper, "Timeouts & Safety").trigger("click");

    expect(wrapper.text()).toContain("Sandbox");
    expect(wrapper.text()).toContain("Terminal Sessions");
    expect(wrapper.text()).toContain("Network Domains");
  });

  it("shows the complete memory stack even when a subsystem is disabled", async () => {
    const wrapper = mount(SettingsView);
    await flushPromises();

    await settingsNavButton(wrapper, "Memory").trigger("click");

    expect(wrapper.text()).toContain("Unified memory");
    expect(wrapper.text()).toContain("Evolving memory");
    expect(wrapper.text()).toContain("Belief memory");
    expect(wrapper.text()).toContain("MAGMA memory");
    expect(wrapper.text()).toContain("Transit memory");
  });

  it("uses one canonical model and endpoint field for summarization", async () => {
    const wrapper = mount(SettingsView);
    await flushPromises();

    await settingsNavButton(wrapper, "Summarization").trigger("click");

    expect(wrapper.findAll("#summary-model")).toHaveLength(1);
    expect(wrapper.findAll("#summary-url")).toHaveLength(1);
  });

  it("shows archaeology controls in its dedicated settings view", async () => {
    const wrapper = mount(SettingsView);
    await flushPromises();

    await settingsNavButton(wrapper, "Archaeology").trigger("click");

    expect(wrapper.text()).toContain("Decision archaeology");
    expect(wrapper.text()).toContain("Context archaeology");
    expect(wrapper.find("pre").exists()).toBe(false);
  });
});
