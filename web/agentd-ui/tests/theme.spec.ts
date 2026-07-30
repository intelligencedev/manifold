import { describe, expect, it } from "vitest";

import { defaultDarkTheme, getTheme } from "@/theme/themes";

describe("theme defaults", () => {
  it("uses Desert Night by default", () => {
    expect(defaultDarkTheme).toBe("desert-night");
  });

  it("defines the complete Desert Night scene", () => {
    expect(getTheme("desert-night").tokens).toEqual({
      background: "11 12 14",
      surface: "20 22 26",
      "surface-muted": "26 29 34",
      border: "58 52 40",
      input: "26 29 34",
      muted: "16 18 20",
      foreground: "242 236 226",
      "muted-foreground": "184 175 160",
      "subtle-foreground": "140 131 114",
      "faint-foreground": "104 98 86",
      accent: "230 160 32",
      "accent-foreground": "11 12 14",
      ring: "240 190 90",
      live: "62 207 207",
      success: "108 188 120",
      "success-foreground": "11 12 14",
      info: "106 158 196",
      "info-foreground": "11 12 14",
      warning: "212 162 74",
      "warning-foreground": "11 12 14",
      destructive: "208 112 112",
      "destructive-foreground": "11 12 14",
      danger: "208 112 112",
      "danger-foreground": "11 12 14",
    });
  });
});
