import { describe, expect, it } from "vitest";
import { useReplayCursor } from "@/composables/useReplayCursor";

describe("useReplayCursor", () => {
  it("clamps movement within range", () => {
    const cursor = useReplayCursor(() => 3);
    expect(cursor.index.value).toBe(0);
    cursor.next();
    expect(cursor.index.value).toBe(1);
    cursor.set(10);
    expect(cursor.index.value).toBe(2);
    cursor.prev();
    expect(cursor.index.value).toBe(1);
  });
});
