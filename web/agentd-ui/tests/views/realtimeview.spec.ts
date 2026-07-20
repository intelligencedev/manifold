import { flushPromises, mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";

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
      },
    ]),
    fetchChatMessages: vi.fn(async () => []),
    fetchChatActivities: vi.fn(async () => []),
    listActiveChatRuns: vi.fn(async () => []),
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
    expect(wrapper.text()).toContain("Start conversation");
    expect(wrapper.text()).toContain("Moonshine STT");
    expect(wrapper.text()).toContain("Supertonic TTS");
  });
});
