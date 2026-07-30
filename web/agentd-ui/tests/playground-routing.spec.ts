import { describe, expect, it } from "vitest";

import router from "@/router";

describe("playground routing", () => {
  it("redirects the playground root to prompts", async () => {
    await router.push("/playground");
    await router.isReady();

    expect(router.currentRoute.value.name).toBe("playground-prompts");
    expect(router.currentRoute.value.path).toBe("/playground/prompts");
  });
});
