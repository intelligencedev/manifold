import { describe, expect, it } from "vitest";
import {
  computeAutoLayout,
  type LayoutEdge,
  type LayoutNode,
} from "@/lib/warppLayout";

const NODES: LayoutNode[] = [
  { id: "a", width: 200, height: 80 },
  { id: "b", width: 200, height: 80 },
  { id: "c", width: 200, height: 80 },
];
const EDGES: LayoutEdge[] = [
  { source: "a", target: "b" },
  { source: "b", target: "c" },
];

describe("computeAutoLayout", () => {
  it("returns no positions for an empty graph", () => {
    expect(computeAutoLayout([], [], "TB").size).toBe(0);
  });

  it("places every node exactly once", () => {
    const pos = computeAutoLayout(NODES, EDGES, "TB");
    expect(pos.size).toBe(3);
    expect(pos.has("a")).toBe(true);
    expect(pos.has("b")).toBe(true);
    expect(pos.has("c")).toBe(true);
  });

  it("stacks a chain vertically for TB and horizontally for LR", () => {
    const tb = computeAutoLayout(NODES, EDGES, "TB");
    const a = tb.get("a")!;
    const b = tb.get("b")!;
    const c = tb.get("c")!;
    // Vertical: y increases down the chain, x stays aligned.
    expect(a.y).toBeLessThan(b.y);
    expect(b.y).toBeLessThan(c.y);
    expect(a.x).toBeCloseTo(b.x, 5);

    const lr = computeAutoLayout(NODES, EDGES, "LR");
    const la = lr.get("a")!;
    const lb = lr.get("b")!;
    const lc = lr.get("c")!;
    // Horizontal: x increases along the chain, y stays aligned.
    expect(la.x).toBeLessThan(lb.x);
    expect(lb.x).toBeLessThan(lc.x);
    expect(la.y).toBeCloseTo(lb.y, 5);
  });

  it("returns VueFlow top-left coordinates, not dagre centers", () => {
    // A single node with the graph margin (24) sits at its top-left origin,
    // i.e. center (24 + w/2, 24 + h/2) converted back to (24, 24).
    const pos = computeAutoLayout([{ id: "solo", width: 200, height: 80 }], [], "TB");
    const solo = pos.get("solo")!;
    expect(solo.x).toBeCloseTo(24, 5);
    expect(solo.y).toBeCloseTo(24, 5);
  });

  it("lays out each scope independently and keeps children relative", () => {
    const nodes: LayoutNode[] = [
      { id: "root1", width: 200, height: 80 },
      { id: "map", width: 400, height: 300 },
      { id: "map/child1", parentNode: "map", width: 200, height: 80 },
      { id: "map/child2", parentNode: "map", width: 200, height: 80 },
    ];
    const edges: LayoutEdge[] = [
      { source: "map/child1", target: "map/child2" },
    ];
    const pos = computeAutoLayout(nodes, edges, "TB");
    expect(pos.size).toBe(4);
    // Children are laid out in their own scope, so both start near the scope
    // origin (small relative coordinates), independent of the root nodes.
    const child1 = pos.get("map/child1")!;
    const child2 = pos.get("map/child2")!;
    expect(child1.x).toBeCloseTo(24, 5);
    expect(child1.y).toBeCloseTo(24, 5);
    expect(child2.y).toBeGreaterThan(child1.y);
  });

  it("ignores edges that cross scope boundaries", () => {
    const nodes: LayoutNode[] = [
      { id: "root1", width: 200, height: 80 },
      { id: "map", width: 400, height: 300 },
      { id: "map/child1", parentNode: "map", width: 200, height: 80 },
    ];
    // Edge from a root node into a child of the map must not throw or misplace.
    const edges: LayoutEdge[] = [{ source: "root1", target: "map/child1" }];
    const pos = computeAutoLayout(nodes, edges, "LR");
    expect(pos.size).toBe(3);
  });
});
