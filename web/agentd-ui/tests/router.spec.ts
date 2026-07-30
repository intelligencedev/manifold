import { describe, expect, it, vi } from "vitest";

import router from "@/router";

describe("router", () => {
  it("routes the server root to chat", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          new Response(JSON.stringify({ ready: true }), { status: 200 }),
        ),
    );

    await router.push("/");

    expect(router.currentRoute.value.name).toBe("chat");

    vi.unstubAllGlobals();
  });

  it("exposes the dedicated realtime voice route", async () => {
    await router.push("/realtime");

    expect(router.currentRoute.value.name).toBe("realtime");
    expect(router.currentRoute.value.meta.title).toBe("Realtime");
  });
});
