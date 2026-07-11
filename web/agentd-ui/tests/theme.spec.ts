import { describe, expect, it } from "vitest";

import { defaultDarkTheme } from "@/theme/themes";

describe("theme defaults", () => {
  it("uses the dark sodium theme by default", () => {
    expect(defaultDarkTheme).toBe("halo-sodium");
  });
});
