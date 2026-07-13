import { describe, expect, it, vi } from "vitest";
import { ref } from "vue";
import { useChatModals } from "@/composables/chat/useChatModals";

function makeModals(sessionId = "session-1") {
  return useChatModals({
    activeSessionId: ref(sessionId),
    scrollMessagesToBottom: () => {},
    autoScrollEnabled: ref(false),
  });
}

describe("useChatModals allow-all-commands dialog", () => {
  it("opens the dialog only when a session is active", () => {
    const withSession = makeModals("session-1");
    withSession.openAllowAllDialog();
    expect(withSession.showAllowAllDialog.value).toBe(true);
    expect(withSession.canConfirmAllowAll.value).toBe(true);

    const noSession = makeModals("");
    noSession.openAllowAllDialog();
    expect(noSession.showAllowAllDialog.value).toBe(false);
    expect(noSession.canConfirmAllowAll.value).toBe(false);
  });

  it("closes without invoking the callback (cancel path)", async () => {
    const modals = makeModals("session-1");
    const enable = vi.fn(async () => {});
    modals.openAllowAllDialog();
    modals.closeAllowAllDialog();
    expect(modals.showAllowAllDialog.value).toBe(false);
    expect(enable).not.toHaveBeenCalled();
  });

  it("enables allow-all for the active session and closes on success", async () => {
    const modals = makeModals("session-1");
    const enable = vi.fn(async () => {});
    modals.openAllowAllDialog();
    await modals.confirmAllowAll(enable);
    expect(enable).toHaveBeenCalledTimes(1);
    expect(enable).toHaveBeenCalledWith("session-1");
    expect(modals.showAllowAllDialog.value).toBe(false);
    expect(modals.allowAllError.value).toBe("");
    expect(modals.allowAllPending.value).toBe(false);
  });

  it("keeps the dialog open and surfaces an error when enabling fails", async () => {
    const modals = makeModals("session-1");
    const enable = vi.fn(async () => {
      throw new Error("boom");
    });
    modals.openAllowAllDialog();
    await modals.confirmAllowAll(enable);
    expect(modals.showAllowAllDialog.value).toBe(true);
    expect(modals.allowAllError.value).toContain("Could not allow all commands");
    expect(modals.allowAllPending.value).toBe(false);
  });

  it("does not call the callback when there is no active session", async () => {
    const modals = makeModals("");
    const enable = vi.fn(async () => {});
    await modals.confirmAllowAll(enable);
    expect(enable).not.toHaveBeenCalled();
  });
});
