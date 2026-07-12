import { beforeEach, describe, expect, it, vi } from "vitest";
import { createPinia, setActivePinia } from "pinia";

vi.mock("@/api/warpp", () => ({
  fetchCatalog: vi.fn(async () => ({
    manifests: [
      {
        type: "data.stringify",
        title: "Stringify",
        category: "data",
        inputs: [{ name: "value", type: "T", required: true }],
        outputs: [{ name: "text", type: "text" }],
      },
      {
        type: "logic.coalesce",
        title: "Coalesce",
        category: "logic",
        inputs: [
          { name: "values", type: "T", required: true, variadic: "list" },
        ],
        outputs: [{ name: "value", type: "T" }],
      },
      {
        type: "control.map",
        title: "Map",
        category: "control",
        inputs: [{ name: "items", type: "list<T>", required: true }],
        outputs: [{ name: "results", type: "dynamic:body" }],
      },
    ],
    coercions: [
      ["number", "text"],
      ["boolean", "text"],
    ],
    workflows: [],
  })),
  listWorkflows: vi.fn(async () => ({ workflows: [] })),
  saveWorkflow: vi.fn(async (_id: string, p: unknown) => p),
  validateWorkflow: vi.fn(async () => ({ valid: true })),
  WarppValidationError: class extends Error {},
}));

import { useWarppEditor } from "@/stores/warppEditor";

describe("warppEditor", () => {
  beforeEach(async () => {
    setActivePinia(createPinia());
    const store = useWarppEditor();
    await store.loadCatalog();
    store.create("wf", "WF");
  });

  it("adds nodes with unique scope-local ids and derives edges from wires", () => {
    const store = useWarppEditor();
    const a = store.addNode("data.stringify", { x: 0, y: 0 });
    const b = store.addNode("data.stringify", { x: 200, y: 0 });
    expect(a).not.toBe(b);
    expect(store.wire(a, "text", b, "value")).toBe(true);
    const node = store.nodeAtPath(b)!;
    expect(node.inputs!.value).toEqual({ from: `${a}.text` });
    const edge = store.flowEdges.find((e) => e.target === b);
    expect(edge).toBeTruthy();
    expect(edge!.sourceHandle).toBe("text");
  });

  it("appends to list-variadic ports", () => {
    const store = useWarppEditor();
    const a = store.addNode("data.stringify", { x: 0, y: 0 });
    const b = store.addNode("data.stringify", { x: 0, y: 100 });
    const c = store.addNode("logic.coalesce", { x: 200, y: 50 });
    store.wire(a, "text", c, "values");
    store.wire(b, "text", c, "values");
    const bindings = store.nodeAtPath(c)!.inputs!.values as { from: string }[];
    expect(bindings).toHaveLength(2);
  });

  it("map children live in the body with scope-local refs", () => {
    const store = useWarppEditor();
    const m = store.addNode("control.map", { x: 0, y: 0 });
    const child = store.addNode("data.stringify", { x: 10, y: 10 }, m);
    expect(child).toBe(`${m}::${child.split("::")[1]}`);
    const mapNode = store.nodeAtPath(m)!;
    expect(mapNode.body!.nodes).toHaveLength(1);
    const vf = store.flowNodes.find((n) => n.id === child)!;
    expect(vf.parentNode).toBe(m);
  });

  it("removeNode strips dangling bindings", () => {
    const store = useWarppEditor();
    const a = store.addNode("data.stringify", { x: 0, y: 0 });
    const b = store.addNode("data.stringify", { x: 200, y: 0 });
    store.wire(a, "text", b, "value");
    store.removeNode(a);
    expect(store.nodeAtPath(b)!.inputs!.value).toBeUndefined();
    expect(store.flowEdges).toHaveLength(0);
  });

  it("setLiteral marks dirty and survives save payload", async () => {
    const store = useWarppEditor();
    const a = store.addNode("data.stringify", { x: 0, y: 0 });
    store.setLiteral(a, "value", "hello");
    expect(store.dirty).toBe(true);
    expect(store.nodeAtPath(a)!.inputs!.value).toEqual({ value: "hello" });
    await store.save();
    expect(store.dirty).toBe(false);
  });
});
