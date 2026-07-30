import { flushPromises, mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";

const responderMocks = vi.hoisted(() => ({
  updateTarget: vi.fn(
    async (
      id: string,
      target: { activeSpecialist: string; activeTeam: string },
    ) => ({
      id,
      name: "Realtime call",
      createdAt: "2026-07-20T00:00:00Z",
      updatedAt: "2026-07-20T00:00:00Z",
      ...target,
    }),
  ),
}));

vi.mock("@/api/chat", async () => {
  const actual =
    await vi.importActual<typeof import("@/api/chat")>("@/api/chat");
  return {
    ...actual,
    listChatSessions: vi.fn(async () => [
      {
        id: "voice-session",
        name: "Realtime call",
        createdAt: "2026-07-20T00:00:00Z",
        updatedAt: "2026-07-20T00:00:00Z",
        activeSpecialist: "orchestrator",
        activeTeam: "",
      },
    ]),
    fetchChatMessages: vi.fn(async () => []),
    fetchChatActivities: vi.fn(async () => []),
    listActiveChatRuns: vi.fn(async () => []),
    updateChatSessionActiveTarget: responderMocks.updateTarget,
  };
});

vi.mock("@/api/client", async () => {
  const actual =
    await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return {
    ...actual,
    listSpecialists: vi.fn(async () => [
      {
        name: "research",
        model: "research-model",
        baseURL: "",
        enableTools: true,
        paused: false,
      },
      {
        name: "paused-specialist",
        model: "paused-model",
        baseURL: "",
        enableTools: true,
        paused: true,
      },
    ]),
    listTeams: vi.fn(async () => [
      {
        name: "operations",
        orchestratorName: "orchestrator",
        members: ["research"],
      },
    ]),
  };
});

import RealtimeView from "@/views/RealtimeView.vue";

describe("RealtimeView", () => {
  it("renders a dedicated call surface and conversation selector", async () => {
    const wrapper = mount(RealtimeView, {
      global: { stubs: { RouterLink: true } },
    });
    await flushPromises();

    expect(wrapper.get("h1").text()).toBe("Realtime voice");
    expect(wrapper.get("#realtime-conversation").text()).toContain(
      "Realtime call",
    );
    expect(wrapper.get("#realtime-responder").text()).toContain(
      "Main orchestrator",
    );
    expect(wrapper.get("#realtime-responder").text()).toContain("research");
    expect(wrapper.get("#realtime-responder").text()).toContain("operations");
    expect(wrapper.get("#realtime-responder").text()).not.toContain(
      "paused-specialist",
    );
    expect(wrapper.text()).toContain("Start conversation");
    expect(wrapper.text()).toContain("Moonshine STT");
    expect(wrapper.text()).toContain("Supertonic TTS");
    expect(wrapper.get("#realtime-microphone").exists()).toBe(true);
    expect(wrapper.get("#realtime-noise-mode").text()).toContain("Automatic");
    expect(wrapper.text()).toContain("native voice isolation");
    expect(wrapper.text()).toContain("RNNoise");

    await wrapper.get("#realtime-responder").setValue("specialist:research");
    await flushPromises();
    expect(responderMocks.updateTarget).toHaveBeenLastCalledWith(
      "voice-session",
      { activeSpecialist: "research", activeTeam: "" },
    );
    expect(wrapper.get("#realtime-responder").element).toHaveProperty(
      "value",
      "specialist:research",
    );

    await wrapper.get("#realtime-responder").setValue("team:operations");
    await flushPromises();
    expect(responderMocks.updateTarget).toHaveBeenLastCalledWith(
      "voice-session",
      { activeSpecialist: "orchestrator", activeTeam: "operations" },
    );
    expect(wrapper.text()).toContain("operations (team)");
  });
});
