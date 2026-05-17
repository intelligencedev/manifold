/**
 * Pure-TypeScript force-directed layout engine for the Fleet Map graph.
 *
 * No external dependencies. Uses Verlet integration with:
 *  - Coulomb repulsion between all node pairs
 *  - Hooke spring attraction along edges
 *  - Weak center gravity to keep the graph on screen
 *  - Velocity damping for stable convergence
 *
 * The orchestrator node is pinned at the center of the canvas.
 */

import type { GraphEdge, GraphNode } from "./types";

export interface LayoutNode extends GraphNode {
  x: number;
  y: number;
  vx: number;
  vy: number;
  /** Pinned nodes ignore forces and stay at their initial position. */
  fixed: boolean;
}

export class ForceLayout {
  nodes: LayoutNode[] = [];
  private edges: GraphEdge[] = [];

  width = 800;
  height = 500;

  // Tuning parameters
  private readonly REPULSION = 7000;
  private readonly SPRING_K = 0.05;
  private readonly REST_LEN = 150; // natural edge length
  private readonly DAMPING = 0.86;
  private readonly GRAVITY = 0.012;
  private readonly MAX_V = 18;

  /** Replace the graph data. Existing node positions are preserved by ID. */
  setGraph(nodes: GraphNode[], edges: GraphEdge[]): void {
    const prev = new Map(this.nodes.map((n) => [n.id, n]));

    this.nodes = nodes.map((n) => {
      const p = prev.get(n.id);
      const isOrchestrator = n.id === "orchestrator";

      if (p) {
        // Update label/status but keep physics state
        return { ...n, x: p.x, y: p.y, vx: p.vx, vy: p.vy, fixed: isOrchestrator };
      }

      // New node — place near center with jitter so they spread
      const angle = Math.random() * Math.PI * 2;
      const dist = isOrchestrator ? 0 : 80 + Math.random() * 140;
      return {
        ...n,
        x: this.width / 2 + Math.cos(angle) * dist,
        y: this.height / 2 + Math.sin(angle) * dist,
        vx: 0,
        vy: 0,
        fixed: isOrchestrator,
      };
    });

    this.edges = edges;
  }

  resize(w: number, h: number): void {
    this.width = w;
    this.height = h;
    // Re-pin orchestrator
    const orch = this.nodes.find((n) => n.id === "orchestrator");
    if (orch) {
      orch.x = w / 2;
      orch.y = h / 2;
    }
  }

  /** Run one integration step. Call once per animation frame. */
  step(): void {
    const cx = this.width / 2;
    const cy = this.height / 2;
    const nodeMap = new Map(this.nodes.map((n) => [n.id, n]));

    for (const node of this.nodes) {
      if (node.fixed) {
        node.x = cx;
        node.y = cy;
        node.vx = 0;
        node.vy = 0;
        continue;
      }

      let fx = 0;
      let fy = 0;

      // --- Coulomb repulsion ---
      for (const other of this.nodes) {
        if (other.id === node.id) continue;
        const dx = node.x - other.x;
        const dy = node.y - other.y;
        const d2 = dx * dx + dy * dy + 0.01;
        const d = Math.sqrt(d2);
        const f = this.REPULSION / d2;
        fx += (dx / d) * f;
        fy += (dy / d) * f;
      }

      // --- Hooke spring attraction ---
      for (const edge of this.edges) {
        let otherId: string | null = null;
        if (edge.source === node.id) otherId = edge.target;
        else if (edge.target === node.id) otherId = edge.source;
        if (!otherId) continue;
        const other = nodeMap.get(otherId);
        if (!other) continue;
        const dx = other.x - node.x;
        const dy = other.y - node.y;
        const d = Math.sqrt(dx * dx + dy * dy) || 1;
        const stretch = d - this.REST_LEN;
        const f = this.SPRING_K * stretch;
        fx += (dx / d) * f;
        fy += (dy / d) * f;
      }

      // --- Weak center gravity ---
      fx += (cx - node.x) * this.GRAVITY;
      fy += (cy - node.y) * this.GRAVITY;

      // --- Integrate ---
      node.vx = (node.vx + fx) * this.DAMPING;
      node.vy = (node.vy + fy) * this.DAMPING;

      // Clamp velocity
      const v = Math.sqrt(node.vx * node.vx + node.vy * node.vy);
      if (v > this.MAX_V) {
        node.vx = (node.vx / v) * this.MAX_V;
        node.vy = (node.vy / v) * this.MAX_V;
      }

      node.x += node.vx;
      node.y += node.vy;

      // Keep inside canvas with a margin
      const margin = 56;
      node.x = Math.max(margin, Math.min(this.width - margin, node.x));
      node.y = Math.max(margin, Math.min(this.height - margin, node.y));
    }
  }

  /** True if the layout has approximately converged. */
  get isSettled(): boolean {
    return this.nodes.every((n) => !n.fixed && Math.abs(n.vx) < 0.5 && Math.abs(n.vy) < 0.5);
  }
}
