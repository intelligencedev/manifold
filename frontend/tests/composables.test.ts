import { describe, it, expect } from "vitest";
import { useReplayCursor } from "@/composables/useReplayCursor";
import { useTrustBudget } from "@/composables/useTrustBudget";

describe("useReplayCursor", () => {
  it("starts at 0", () => {
    const c = useReplayCursor(() => 5);
    expect(c.index.value).toBe(0);
  });

  it("advances and clamps at max", () => {
    const c = useReplayCursor(() => 3);
    c.next(); c.next(); c.next(); c.next();
    expect(c.index.value).toBe(2);
  });

  it("decrements and clamps at 0", () => {
    const c = useReplayCursor(() => 3);
    c.set(2); c.prev(); c.prev(); c.prev();
    expect(c.index.value).toBe(0);
  });

  it("set() clamps within range", () => {
    const c = useReplayCursor(() => 4);
    c.set(100);
    expect(c.index.value).toBe(3);
    c.set(-5);
    expect(c.index.value).toBe(0);
  });
});

describe("useTrustBudget", () => {
  it("computes remaining and ratio", () => {
    const { remaining, ratio } = useTrustBudget(10, 3);
    expect(remaining.value).toBe(7);
    expect(ratio.value).toBeCloseTo(0.3);
  });

  it("clamps remaining at 0 when overspent", () => {
    const { remaining } = useTrustBudget(5, 10);
    expect(remaining.value).toBe(0);
  });

  it("ratio is 0 when quota is 0", () => {
    const { ratio } = useTrustBudget(0, 0);
    expect(ratio.value).toBe(0);
  });
});
