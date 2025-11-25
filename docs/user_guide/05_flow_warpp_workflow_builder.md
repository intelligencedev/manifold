---
title: Flow — WARPP Workflow Builder
description: Drag-and-drop workflows that connect tools and utilities, save/export, and run with live traces.
---

# Flow (WARPP workflow builder)

Flow is a visual editor for WARPP workflows. You drag tools from a palette, connect nodes, group them, save/export, and run. A run view shows step traces and a timer.

Placeholder for screenshots: [Flow canvas with tool palette and nodes]

## Key concepts

- Workflow: identified by intent (a short name). Saved server-side as steps plus UI metadata.
- Step: a unit that calls a tool by name with arguments; edges define depends_on.
- Utility nodes: editor-only helpers like Group Container, Sticky Note, and special Agent Response renderer. Utilities don’t execute remote APIs unless the underlying tool does.
- Groups: visual containers for organizing steps; membership is tracked in workflow UI metadata.

## Getting started

- Open Flow from the nav.
- Select an existing workflow from the Workflow dropdown, or click New to create one. New prompts for an intent name.

## Tool palette

- Utility nodes are listed first: Group Container, Sticky Note, and any backend-provided utility_* tools. Drag onto the canvas to add.
- Workflow tools (real steps) come from the server’s WARPP tools registry and appear below.

Drag and drop
- Drag from the palette; the canvas border highlights during drag. Drop to create a node at that position.
- Drop inside a Group to assign membership automatically.
- Connect nodes by dragging from a handle to another node. Duplicate edges and self-loops are ignored.

## Canvas controls

- Collapse/Expand all nodes.
- Auto layout: vertical (TB) or horizontal (LR) via dagre; grouped children move with their parent group.
- Zoom in/out, Fit view.
- Edge style cycle: Default, Smoothstep, Step, Straight, Simple Bezier.
- Lock positions: disables dragging nodes.
- MiniMap: toggle show/hide; pannable/zoomable.

## Node configuration

- Select a node to open Node Configuration in the left sidebar.
- For step nodes, set the tool name, arguments, labels, and publish options in the inspector (via NodeInspectorStep component). Utilities have their own inspectors.

## Save, export, import, delete

- Save: opens a Workflow Metadata modal. Both Description and Keywords are required to enable Save. The editor persists steps, edges (as depends_on), and UI metadata (layout, parents, groups, notes). The client also caches UI to preserve layout even if the server doesn’t persist all UI fields.
- Export: downloads the current workflow JSON (merged with latest UI snapshot) as a file named <intent>-<timestamp>.json.
- Import: choose a JSON file; you’ll be prompted to set or adjust the intent if there’s a conflict. Imported workflows appear as local drafts until saved.
- Delete: removes the workflow from the server.

## Run mode and traces

- Run: saves (if needed) and starts a run for the selected intent.
- While running: the header shows a spinner and a live timer. A Cancel button appears to stop the run. The run view streams step traces (with small delays for readability) and logs messages.
- Result modal: click a step with trace to open a modal showing Rendered Arguments, Delta, Payload, and any error. Each section is collapsible.
- Switch modes: Design or Run using the Mode toggle in the header. Entering Run freezes node sizes for stability.

Hotkeys and interactions
- Shift+drag to box-select multiple nodes; Delete/Backspace removes selection.
- Cmd/Ctrl+click to multi-select. Drag on empty canvas to pan; wheel/trackpad to zoom.

Notes on UI persistence
- The editor tracks positions, group membership, and sticky notes locally and merges with server UI metadata on save/load. This protects layout even if server UI persistence is partial.

Additional UI details
- MiniMap is hidden by default; click the show button in the bottom-right overlay to reveal it.
- Run logs appear in a compact scrollable bar under the header while saving/running.
- The Mode button for Run is disabled until trace data exists or a run is in progress.
- Node Configuration sidebar shows a back arrow to return to the Tool Palette; the palette auto-appears when 0 or 2+ nodes are selected.
- Edge styles cycle through: Default, Smoothstep, Step, Straight, Simple Bezier.
- Auto layout preserves relative positions of nodes inside groups while moving the group as a unit.

Placeholder for screenshots: [Controls toolbar; Metadata modal; Result modal]

