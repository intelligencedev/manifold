/**
 * PixiJS v7 WebGL renderer for the Fleet Map graph.
 *
 * Architecture:
 *  - One PIXI.Graphics layer for edges (cleared and redrawn each frame).
 *  - One PIXI.Container per node (created once, position updated each frame).
 *  - ForceLayout runs inside the Ticker so the graph animates to equilibrium.
 *  - Pointer events on node containers emit hover/click callbacks.
 *
 * Usage:
 *   const renderer = new FleetRenderer(canvasElement, 800, 500);
 *   renderer.update(nodes, edges);
 *   renderer.onNodeHover = (node, screenX, screenY) => { ... };
 *   renderer.onNodeClick = (node) => { ... };
 *   // on cleanup:
 *   renderer.destroy();
 */

import {
  Application,
  Container,
  Graphics,
  Text,
  TextStyle,
  type ICanvas,
} from "pixi.js";
import { ForceLayout } from "./layoutEngine";
import type { GraphEdge, GraphNode, NodeKind, NodeStatus } from "./types";

// ─── Color palette (matches token.css design tokens) ─────────────────────────
const COLORS = {
  bg: 0x07090c,
  surface: 0x14181e,
  border: 0x303842,
  orchestrator: 0x5a59d3, // accent
  specialist: 0x1e86d4, // blue
  team: 0x22c9a6, // teal / success
  runRunning: 0x22c966, // green
  runFailed: 0xe35d4d, // danger
  runCompleted: 0x3c3c4a, // muted
  runIdle: 0x2a3040,
  edgeMembership: 0x303842, // subtle border
  edgeDelegation: 0x5a59d3, // accent
  edgeActive: 0x22c9a6,
  textMain: 0xf2f5f8,
  textSub: 0x9099a8,
};

const NODE_RADIUS: Record<NodeKind, number> = {
  orchestrator: 34,
  specialist: 24,
  team: 26,
  run: 18,
};

function nodeColor(kind: NodeKind, status: NodeStatus): number {
  if (kind === "orchestrator") return COLORS.orchestrator;
  if (kind === "team") return COLORS.team;
  if (kind === "specialist") return status === "paused" ? COLORS.border : COLORS.specialist;
  // run
  if (status === "running") return COLORS.runRunning;
  if (status === "failed") return COLORS.runFailed;
  if (status === "completed") return COLORS.runCompleted;
  return COLORS.runIdle;
}

function edgeColor(kind: GraphEdge["kind"]): number {
  if (kind === "delegation") return COLORS.edgeDelegation;
  if (kind === "active") return COLORS.edgeActive;
  return COLORS.edgeMembership;
}

function edgeAlpha(kind: GraphEdge["kind"]): number {
  if (kind === "membership") return 0.25;
  if (kind === "delegation") return 0.7;
  return 0.5;
}

function edgeWidth(kind: GraphEdge["kind"]): number {
  if (kind === "membership") return 1;
  return 2;
}

// ─── Node display object ─────────────────────────────────────────────────────

interface NodeDisplay {
  container: Container;
  circle: Graphics;
  glow: Graphics;
  label: Text;
  sublabel: Text | null;
  node: GraphNode;
  pulsePhase: number;
}

// ─── Main renderer ────────────────────────────────────────────────────────────

export class FleetRenderer {
  private app: Application;
  private edgeLayer: Graphics;
  private nodeLayer: Container;
  private layout: ForceLayout;
  private nodeDisplays = new Map<string, NodeDisplay>();
  private edges: GraphEdge[] = [];
  private dashOffset = 0;
  private frameCount = 0;

  // Callbacks for the Vue layer
  onNodeHover: ((node: GraphNode | null, sx: number, sy: number) => void) | null = null;
  onNodeClick: ((node: GraphNode) => void) | null = null;

  constructor(canvas: HTMLCanvasElement, width: number, height: number) {
    this.app = new Application({
      view: canvas as unknown as ICanvas,
      width,
      height,
      backgroundColor: COLORS.bg,
      antialias: true,
      resolution: Math.min(window.devicePixelRatio || 1, 2),
      autoDensity: true,
    });

    this.layout = new ForceLayout();
    this.layout.width = width;
    this.layout.height = height;

    // Edge layer (drawn behind nodes)
    this.edgeLayer = new Graphics();
    this.app.stage.addChild(this.edgeLayer);

    // Node layer
    this.nodeLayer = new Container();
    this.app.stage.addChild(this.nodeLayer);

    // Main ticker
    this.app.ticker.add(this._tick.bind(this));
  }

  static isWebGLAvailable(): boolean {
    try {
      const c = document.createElement("canvas");
      return !!(c.getContext("webgl2") || c.getContext("webgl"));
    } catch {
      return false;
    }
  }

  resize(width: number, height: number): void {
    this.app.renderer.resize(width, height);
    this.layout.resize(width, height);
  }

  /** Replace the graph data. Reconciles existing display objects by node ID. */
  update(nodes: GraphNode[], edges: GraphEdge[]): void {
    this.edges = edges;
    this.layout.setGraph(nodes, edges);

    const newIds = new Set(nodes.map((n) => n.id));

    // Remove stale nodes
    for (const [id, display] of this.nodeDisplays) {
      if (!newIds.has(id)) {
        this.nodeLayer.removeChild(display.container);
        display.container.destroy({ children: true });
        this.nodeDisplays.delete(id);
      }
    }

    // Add new nodes or update existing
    for (const node of nodes) {
      if (this.nodeDisplays.has(node.id)) {
        // Update mutable fields
        const d = this.nodeDisplays.get(node.id)!;
        d.node = node;
        this._updateNodeVisuals(d);
      } else {
        this._createNodeDisplay(node);
      }
    }
  }

  destroy(): void {
    this.app.destroy(false, { children: true });
  }

  // ─── Private ───────────────────────────────────────────────────────────────

  private _tick(): void {
    this.frameCount++;
    this.dashOffset = (this.dashOffset + 0.8) % 20;

    // Run layout step (skip when settled to save CPU)
    if (!this.layout.isSettled || this.frameCount % 30 === 0) {
      this.layout.step();
    }

    // Sync node container positions
    for (const [id, display] of this.nodeDisplays) {
      const ln = this.layout.nodes.find((n) => n.id === id);
      if (!ln) continue;
      display.container.position.set(ln.x, ln.y);

      // Pulse animation for running nodes
      if (display.node.status === "running") {
        display.pulsePhase += 0.04;
        const pulse = 0.15 + Math.sin(display.pulsePhase) * 0.15;
        display.glow.alpha = pulse;
        display.glow.scale.set(1 + Math.sin(display.pulsePhase) * 0.2);
      }
    }

    // Redraw all edges from layout positions
    this._drawEdges();
  }

  private _drawEdges(): void {
    const g = this.edgeLayer;
    g.clear();

    const nodeMap = new Map(this.layout.nodes.map((n) => [n.id, n]));

    for (const edge of this.edges) {
      const src = nodeMap.get(edge.source);
      const tgt = nodeMap.get(edge.target);
      if (!src || !tgt) continue;

      const color = edgeColor(edge.kind);
      const alpha = edgeAlpha(edge.kind);
      const lineWidth = edgeWidth(edge.kind);

      // Compute start/end points (from the node circle circumference)
      const dx = tgt.x - src.x;
      const dy = tgt.y - src.y;
      const dist = Math.sqrt(dx * dx + dy * dy) || 1;
      const ux = dx / dist;
      const uy = dy / dist;

      const r1 = NODE_RADIUS[src.kind] + 2;
      const r2 = NODE_RADIUS[tgt.kind] + 2;
      const x1 = src.x + ux * r1;
      const y1 = src.y + uy * r1;
      const x2 = tgt.x - ux * r2;
      const y2 = tgt.y - uy * r2;

      g.lineStyle(lineWidth, color, alpha);

      if (edge.animated) {
        this._drawDashedLine(g, x1, y1, x2, y2, 8, 6, this.dashOffset);
      } else if (edge.kind === "membership") {
        this._drawDashedLine(g, x1, y1, x2, y2, 4, 8, 0);
      } else {
        g.moveTo(x1, y1);
        g.lineTo(x2, y2);
      }

      // Arrow head for delegation edges
      if (edge.kind === "delegation" || edge.kind === "active") {
        this._drawArrow(g, x1, y1, x2, y2, 10, color, alpha);
      }
    }
  }

  private _drawDashedLine(
    g: Graphics,
    x1: number, y1: number, x2: number, y2: number,
    dashLen: number, gapLen: number, offset: number
  ): void {
    const dx = x2 - x1;
    const dy = y2 - y1;
    const total = Math.sqrt(dx * dx + dy * dy);
    if (total < 1) return;
    const ux = dx / total;
    const uy = dy / total;

    // Start offset so the dashes animate
    let pos = -(offset % (dashLen + gapLen));
    let drawing = pos >= 0;
    if (pos < 0) {
      pos += dashLen + gapLen;
      drawing = false;
    }

    while (pos < total) {
      const segLen = Math.min(drawing ? dashLen : gapLen, total - pos);
      const px = x1 + ux * pos;
      const py = y1 + uy * pos;
      if (drawing) {
        g.moveTo(px, py);
        g.lineTo(px + ux * segLen, py + uy * segLen);
      }
      pos += segLen;
      drawing = !drawing;
    }
  }

  private _drawArrow(
    g: Graphics,
    x1: number, y1: number, x2: number, y2: number,
    size: number, color: number, alpha: number
  ): void {
    const angle = Math.atan2(y2 - y1, x2 - x1);
    g.lineStyle(0);
    g.beginFill(color, alpha * 0.9);
    g.moveTo(x2, y2);
    g.lineTo(
      x2 - size * Math.cos(angle - Math.PI / 6),
      y2 - size * Math.sin(angle - Math.PI / 6)
    );
    g.lineTo(
      x2 - size * Math.cos(angle + Math.PI / 6),
      y2 - size * Math.sin(angle + Math.PI / 6)
    );
    g.closePath();
    g.endFill();
  }

  private _createNodeDisplay(node: GraphNode): void {
    const container = new Container();
    (container as any).interactive = true;
    (container as any).cursor = "pointer";

    // Glow ring (shown during pulse or hover)
    const glow = new Graphics();
    glow.alpha = 0;
    container.addChild(glow);

    // Main circle
    const circle = new Graphics();
    container.addChild(circle);

    // Label
    const labelStyle = new TextStyle({
      fontFamily: "Inter, ui-sans-serif, sans-serif",
      fontSize: node.kind === "orchestrator" ? 12 : 10,
      fontWeight: node.kind === "orchestrator" ? "700" : "500",
      fill: COLORS.textMain,
      align: "center",
    });
    const label = new Text(node.label, labelStyle);
    label.anchor.set(0.5, 0);
    container.addChild(label);

    // Sub-label
    let sublabel: Text | null = null;
    if (node.sublabel) {
      const subStyle = new TextStyle({
        fontFamily: "Inter, ui-sans-serif, sans-serif",
        fontSize: 9,
        fill: COLORS.textSub,
        align: "center",
      });
      sublabel = new Text(node.sublabel, subStyle);
      sublabel.anchor.set(0.5, 0);
      container.addChild(sublabel);
    }

    const display: NodeDisplay = { container, circle, glow, label, sublabel, node, pulsePhase: Math.random() * Math.PI * 2 };
    this._updateNodeVisuals(display);

    // Hover and click
    (container as any).on("pointerover", (e: any) => {
      display.glow.alpha = 0.35;
      display.circle.scale.set(1.08);
      if (this.onNodeHover) {
        const bounds = (this.app.view as HTMLCanvasElement).getBoundingClientRect();
        this.onNodeHover(node, bounds.left + e.global.x, bounds.top + e.global.y);
      }
    });
    (container as any).on("pointerout", () => {
      if (node.status !== "running") display.glow.alpha = 0;
      display.circle.scale.set(1);
      if (this.onNodeHover) this.onNodeHover(null, 0, 0);
    });
    (container as any).on("pointertap", () => {
      if (this.onNodeClick) this.onNodeClick(node);
    });

    // Place at layout position (or center fallback)
    const ln = this.layout.nodes.find((n) => n.id === node.id);
    if (ln) container.position.set(ln.x, ln.y);

    this.nodeLayer.addChild(container);
    this.nodeDisplays.set(node.id, display);
  }

  private _updateNodeVisuals(d: NodeDisplay): void {
    const { node, circle, glow, label, sublabel } = d;
    const r = NODE_RADIUS[node.kind];
    const color = nodeColor(node.kind, node.status);

    // Redraw glow
    glow.clear();
    glow.beginFill(color, 0.25);
    glow.drawCircle(0, 0, r + 14);
    glow.endFill();

    // Redraw circle
    circle.clear();

    // Outer ring
    circle.lineStyle(1.5, color, 0.5);
    circle.drawCircle(0, 0, r + 3);

    // Fill
    circle.lineStyle(0);
    circle.beginFill(COLORS.surface);
    circle.drawCircle(0, 0, r);
    circle.endFill();

    // Inner accent fill
    circle.beginFill(color, 0.2);
    circle.drawCircle(0, 0, r);
    circle.endFill();

    // Kind letter badge
    const letterRadius = r * 0.45;
    circle.beginFill(color, 0.6);
    circle.drawCircle(0, -letterRadius * 0.3, letterRadius);
    circle.endFill();

    // Label below circle
    label.text = node.label.length > 14 ? node.label.slice(0, 13) + "…" : node.label;
    label.position.set(0, r + 5);

    if (sublabel) {
      sublabel.text = node.sublabel
        ? node.sublabel.length > 16
          ? node.sublabel.slice(0, 15) + "…"
          : node.sublabel
        : "";
      sublabel.position.set(0, r + 16);
    }

    // Running nodes start with subtle glow
    if (node.status === "running") {
      glow.alpha = 0.2;
    }
  }
}
