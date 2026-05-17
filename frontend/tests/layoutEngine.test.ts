import { describe, it, expect } from "vitest";
import { ForceLayout } from "@/components/fleet/layoutEngine";

const makeNodes = (ids: string[]) =>
  ids.map((id) => ({
    id,
    label: id,
    kind: (id === "orchestrator" ? "orchestrator" : "specialist") as any,
    status: "online" as any,
  }));

describe("ForceLayout", () => {
  it("pins orchestrator at canvas center", () => {
    const layout = new ForceLayout();
    layout.width = 600;
    layout.height = 400;
    layout.setGraph(makeNodes(["orchestrator", "sp:alice"]), []);

    for (let i = 0; i < 30; i++) layout.step();

    const orch = layout.nodes.find((n) => n.id === "orchestrator")!;
    expect(orch.x).toBeCloseTo(300, 0);
    expect(orch.y).toBeCloseTo(200, 0);
  });

  it("repels disconnected nodes apart from orchestrator", () => {
    const layout = new ForceLayout();
    layout.width = 800;
    layout.height = 600;
    layout.setGraph(makeNodes(["orchestrator", "sp:a", "sp:b"]), []);

    for (let i = 0; i < 100; i++) layout.step();

    const orch = layout.nodes.find((n) => n.id === "orchestrator")!;
    const a = layout.nodes.find((n) => n.id === "sp:a")!;
    const b = layout.nodes.find((n) => n.id === "sp:b")!;

    const dA = Math.hypot(a.x - orch.x, a.y - orch.y);
    const dB = Math.hypot(b.x - orch.x, b.y - orch.y);

    // Nodes should have moved away from the center
    expect(dA).toBeGreaterThan(20);
    expect(dB).toBeGreaterThan(20);
  });

  it("preserves positions of existing nodes on setGraph", () => {
    const layout = new ForceLayout();
    layout.width = 800;
    layout.height = 600;
    layout.setGraph(makeNodes(["orchestrator", "sp:a"]), []);

    for (let i = 0; i < 20; i++) layout.step();

    const posBefore = { x: layout.nodes.find((n) => n.id === "sp:a")!.x };

    // Re-apply same graph — positions should be preserved
    layout.setGraph(makeNodes(["orchestrator", "sp:a", "sp:b"]), []);
    const posAfter = layout.nodes.find((n) => n.id === "sp:a")!.x;

    expect(posAfter).toBeCloseTo(posBefore.x, 0);
  });

  it("keeps nodes within canvas bounds after many steps", () => {
    const layout = new ForceLayout();
    layout.width = 600;
    layout.height = 400;
    const ids = ["orchestrator", ...Array.from({ length: 8 }, (_, i) => `sp:${i}`)];
    const edges = ids.slice(1).map((id) => ({ source: "orchestrator", target: id, kind: "membership" as any }));
    layout.setGraph(makeNodes(ids), edges);

    for (let i = 0; i < 200; i++) layout.step();

    for (const node of layout.nodes) {
      expect(node.x).toBeGreaterThanOrEqual(0);
      expect(node.x).toBeLessThanOrEqual(layout.width);
      expect(node.y).toBeGreaterThanOrEqual(0);
      expect(node.y).toBeLessThanOrEqual(layout.height);
    }
  });

  it("isSettled becomes true after convergence with no forces", () => {
    const layout = new ForceLayout();
    layout.width = 800;
    layout.height = 600;
    // Single pinned node — nothing moves, should settle immediately
    layout.setGraph(makeNodes(["orchestrator"]), []);

    for (let i = 0; i < 10; i++) layout.step();

    expect(layout.isSettled).toBe(true);
  });
});
