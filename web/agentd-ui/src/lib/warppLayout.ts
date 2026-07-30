import dagre from "dagre";

export type LayoutDirection = "TB" | "LR";

export interface LayoutNode {
  id: string;
  /** Parent node id when nested inside a Map body; undefined at the root. */
  parentNode?: string;
  width: number;
  height: number;
}

export interface LayoutEdge {
  source: string;
  target: string;
}

/** Fallback footprint for nodes VueFlow has not measured yet. */
export const DEFAULT_NODE_WIDTH = 200;
export const DEFAULT_NODE_HEIGHT = 80;

/**
 * Compute auto-layout positions for a set of flow nodes using dagre.
 *
 * Each scope (the root canvas and every Map body) is laid out independently so
 * that nested children stay positioned relative to their parent. Positions are
 * returned as VueFlow-style top-left coordinates keyed by node id. Nodes that
 * dagre could not place are omitted from the result.
 */
export function computeAutoLayout(
  nodes: LayoutNode[],
  edges: LayoutEdge[],
  direction: LayoutDirection,
): Map<string, { x: number; y: number }> {
  const positions = new Map<string, { x: number; y: number }>();
  if (!nodes.length) return positions;

  const byScope = new Map<string, LayoutNode[]>();
  for (const n of nodes) {
    const scope = n.parentNode ?? "";
    const bucket = byScope.get(scope);
    if (bucket) bucket.push(n);
    else byScope.set(scope, [n]);
  }

  for (const scopeNodes of byScope.values()) {
    const ids = new Set(scopeNodes.map((n) => n.id));
    const g = new dagre.graphlib.Graph();
    g.setGraph({
      rankdir: direction,
      nodesep: 60,
      ranksep: 80,
      marginx: 24,
      marginy: 24,
    });
    g.setDefaultEdgeLabel(() => ({}));

    for (const n of scopeNodes) {
      g.setNode(n.id, {
        width: n.width || DEFAULT_NODE_WIDTH,
        height: n.height || DEFAULT_NODE_HEIGHT,
      });
    }
    for (const e of edges) {
      if (ids.has(e.source) && ids.has(e.target)) g.setEdge(e.source, e.target);
    }

    try {
      dagre.layout(g);
    } catch (err) {
      console.warn("dagre layout failed", err);
      continue;
    }

    for (const n of scopeNodes) {
      const pos = g.node(n.id) as { x: number; y: number } | undefined;
      if (!pos) continue;
      const w = n.width || DEFAULT_NODE_WIDTH;
      const h = n.height || DEFAULT_NODE_HEIGHT;
      // dagre reports node centers; VueFlow positions are top-left.
      positions.set(n.id, { x: pos.x - w / 2, y: pos.y - h / 2 });
    }
  }

  return positions;
}
