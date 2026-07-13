import { defineStore } from "pinia";
import { computed, ref } from "vue";
import {
  deleteWorkflow as apiDeleteWorkflow,
  fetchCatalog,
  getWorkflow,
  listWorkflows,
  saveWorkflow,
  validateWorkflow,
  WarppValidationError,
} from "@/api/warpp";
import type {
  WarppBinding,
  WarppCanvas,
  WarppCatalog,
  WarppDiagnostic,
  WarppDocument,
  WarppManifest,
  WarppNode,
  WarppWorkflowSummary,
} from "@/types/warpp";

interface VfNode {
  id: string;
  type: string;
  position: { x: number; y: number };
  data: { node: WarppNode; scopePath: string };
  parentNode?: string;
  extent?: "parent";
}

interface VfEdge {
  id: string;
  source: string;
  target: string;
  sourceHandle: string;
  targetHandle: string;
}

const SEP = "::";

function localId(path: string): string {
  const parts = path.split(SEP);
  return parts[parts.length - 1];
}

function parentPrefix(path: string): string {
  const parts = path.split(SEP);
  parts.pop();
  return parts.join(SEP);
}

function refString(prefix: string, id: string): string {
  return prefix ? `${prefix}${SEP}${id}` : id;
}

export const useWarppEditor = defineStore("warppEditor", () => {
  const doc = ref<WarppDocument | null>(null);
  const canvas = ref<WarppCanvas>({ nodes: {} });
  const catalog = ref<WarppCatalog | null>(null);
  const selectedPath = ref<string | null>(null);
  const diagnostics = ref<WarppDiagnostic[]>([]);
  const dirty = ref(false);
  const workflows = ref<WarppWorkflowSummary[]>([]);

  function manifestByType(type: string): WarppManifest | undefined {
    return catalog.value?.manifests.find((m) => m.type === type);
  }

  // containerFor returns the sibling nodes array that holds the node at path.
  function containerFor(path: string): WarppNode[] | null {
    if (!doc.value) return null;
    const parts = path.split(SEP);
    let nodes = doc.value.nodes;
    for (let i = 0; i < parts.length - 1; i++) {
      const parent = nodes.find((n) => n.id === parts[i]);
      if (!parent || !parent.body) return null;
      nodes = parent.body.nodes;
    }
    return nodes;
  }

  function nodeAtPath(path: string): WarppNode | undefined {
    const nodes = containerFor(path);
    if (!nodes) return undefined;
    return nodes.find((n) => n.id === localId(path));
  }

  function uniqueId(nodes: WarppNode[], type: string): string {
    const tail = type.split(".").pop() || "node";
    let i = 1;
    while (nodes.some((n) => n.id === `${tail}${i}`)) i++;
    return `${tail}${i}`;
  }

  function addNode(
    type: string,
    pos: { x: number; y: number },
    parentPath?: string,
  ): string {
    if (!doc.value) return "";
    const nodes = parentPath
      ? nodeAtPath(parentPath)?.body?.nodes
      : doc.value.nodes;
    if (!nodes) return "";
    const id = uniqueId(nodes, type);
    const node: WarppNode = { id, type, inputs: {} };
    if (type === "control.map") {
      node.body = { nodes: [], outputs: {} };
    }
    nodes.push(node);
    const path = parentPath ? `${parentPath}${SEP}${id}` : id;
    setPosition(path, pos.x, pos.y);
    dirty.value = true;
    return path;
  }

  function removeNode(path: string): void {
    const nodes = containerFor(path);
    if (!nodes) return;
    const id = localId(path);
    const idx = nodes.findIndex((n) => n.id === id);
    if (idx === -1) return;
    nodes.splice(idx, 1);
    // Strip any binding in this scope that references the removed node.
    for (const n of nodes) {
      stripRefs(n, id);
    }
    if (doc.value) {
      for (const [k, b] of Object.entries(doc.value.outputs ?? {})) {
        if (bindingRefsNode(b, id)) delete doc.value.outputs![k];
      }
    }
    if (canvas.value.nodes) delete canvas.value.nodes[path];
    dirty.value = true;
  }

  function stripRefs(node: WarppNode, removedId: string): void {
    if (!node.inputs) return;
    for (const [port, input] of Object.entries(node.inputs)) {
      if (Array.isArray(input)) {
        const kept = input.filter((b) => !bindingRefsNode(b, removedId));
        if (kept.length) node.inputs[port] = kept;
        else delete node.inputs[port];
      } else if (isBinding(input)) {
        if (bindingRefsNode(input, removedId)) delete node.inputs[port];
      } else {
        for (const [key, b] of Object.entries(input)) {
          if (bindingRefsNode(b, removedId)) delete input[key];
        }
      }
    }
  }

  function bindingRefsNode(b: WarppBinding, id: string): boolean {
    return typeof b.from === "string" && b.from.split(".")[0] === id;
  }

  function isBinding(v: unknown): v is WarppBinding {
    return (
      typeof v === "object" &&
      v !== null &&
      ("from" in (v as object) || "value" in (v as object))
    );
  }

  function setLiteral(path: string, port: string, value: unknown): void {
    const node = nodeAtPath(path);
    if (!node) return;
    node.inputs = node.inputs ?? {};
    node.inputs[port] = { value };
    dirty.value = true;
  }

  function clearInput(path: string, port: string): void {
    const node = nodeAtPath(path);
    if (!node?.inputs) return;
    delete node.inputs[port];
    dirty.value = true;
  }

  function wire(
    fromPath: string,
    fromPort: string,
    toPath: string,
    toPort: string,
  ): boolean {
    const target = nodeAtPath(toPath);
    if (!target) return false;
    const manifest = manifestByType(target.type);
    if (!manifest) return false;
    const spec = manifest.inputs.find((p) => p.name === toPort);
    if (!spec) return false;
    target.inputs = target.inputs ?? {};
    const ref: WarppBinding = { from: `${localId(fromPath)}.${fromPort}` };
    if (spec.variadic === "list") {
      const existing = target.inputs[toPort];
      const list = Array.isArray(existing) ? existing : [];
      list.push(ref);
      target.inputs[toPort] = list;
    } else {
      target.inputs[toPort] = ref;
    }
    dirty.value = true;
    return true;
  }

  function unwire(toPath: string, toPort: string, index?: number): void {
    const node = nodeAtPath(toPath);
    if (!node?.inputs) return;
    const input = node.inputs[toPort];
    if (Array.isArray(input) && typeof index === "number") {
      input.splice(index, 1);
      if (!input.length) delete node.inputs[toPort];
    } else {
      delete node.inputs[toPort];
    }
    dirty.value = true;
  }

  function setNamedVar(
    path: string,
    port: string,
    key: string,
    binding: WarppBinding,
  ): void {
    const node = nodeAtPath(path);
    if (!node) return;
    node.inputs = node.inputs ?? {};
    const cur = node.inputs[port];
    const map: Record<string, WarppBinding> =
      cur && !Array.isArray(cur) && !isBinding(cur)
        ? (cur as Record<string, WarppBinding>)
        : {};
    map[key] = binding;
    node.inputs[port] = map;
    dirty.value = true;
  }

  function setPosition(path: string, x: number, y: number): void {
    canvas.value.nodes = canvas.value.nodes ?? {};
    canvas.value.nodes[path] = { x, y };
  }

  function positionFor(path: string, index: number): { x: number; y: number } {
    const stored = canvas.value.nodes?.[path];
    if (stored) return { x: stored.x, y: stored.y };
    return { x: 80 + 240 * (index % 4), y: 80 + 160 * Math.floor(index / 4) };
  }

  const flowNodes = computed<VfNode[]>(() => {
    if (!doc.value) return [];
    const out: VfNode[] = [];
    let counter = 0;
    const walk = (nodes: WarppNode[], prefix: string, parent?: string) => {
      for (const n of nodes) {
        const path = refString(prefix, n.id);
        out.push({
          id: path,
          type: "warpp",
          position: positionFor(path, counter++),
          data: { node: n, scopePath: prefix },
          ...(parent ? { parentNode: parent, extent: "parent" } : {}),
        });
        if (n.body) walk(n.body.nodes, path, path);
      }
    };
    walk(doc.value.nodes, "");
    return out;
  });

  const flowEdges = computed<VfEdge[]>(() => {
    if (!doc.value) return [];
    const out: VfEdge[] = [];
    const emit = (target: string, port: string, b: WarppBinding, i = 0) => {
      if (typeof b.from !== "string") return;
      const [srcId, ...rest] = b.from.split(".");
      if (srcId === "in" || srcId === "item") return;
      const prefix = parentPrefix(target);
      const source = refString(prefix, srcId);
      out.push({
        id: `${source}->${target}:${port}:${i}`,
        source,
        target,
        sourceHandle: rest.join("."),
        targetHandle: port,
      });
    };
    const walk = (nodes: WarppNode[], prefix: string) => {
      for (const n of nodes) {
        const path = refString(prefix, n.id);
        for (const [port, input] of Object.entries(n.inputs ?? {})) {
          if (Array.isArray(input)) {
            input.forEach((b, i) => emit(path, port, b, i));
          } else if (isBinding(input)) {
            emit(path, port, input);
          } else {
            let i = 0;
            for (const b of Object.values(input)) emit(path, port, b, i++);
          }
        }
        if (n.body) walk(n.body.nodes, path);
      }
    };
    walk(doc.value.nodes, "");
    return out;
  });

  async function loadCatalog(): Promise<void> {
    catalog.value = await fetchCatalog();
  }

  async function loadList(): Promise<void> {
    workflows.value = (await listWorkflows()).workflows;
  }

  async function load(id: string): Promise<void> {
    const resp = await getWorkflow(id);
    doc.value = resp.document;
    canvas.value = resp.canvas ?? { nodes: {} };
    diagnostics.value = [];
    dirty.value = false;
    selectedPath.value = null;
  }

  function create(id: string, name: string): void {
    doc.value = { id, name, nodes: [], outputs: {} };
    canvas.value = { nodes: {} };
    diagnostics.value = [];
    dirty.value = false;
    selectedPath.value = null;
  }

  // Delete a workflow by id. If it is the one currently open, the editor is
  // reset to the empty state. The workflow list is refreshed either way.
  async function remove(id: string): Promise<void> {
    await apiDeleteWorkflow(id);
    if (doc.value?.id === id) {
      doc.value = null;
      canvas.value = { nodes: {} };
      diagnostics.value = [];
      dirty.value = false;
      selectedPath.value = null;
    }
    await loadList();
  }

  async function save(): Promise<boolean> {
    if (!doc.value) return false;
    try {
      await saveWorkflow(doc.value.id, {
        document: doc.value,
        canvas: canvas.value,
      });
      diagnostics.value = [];
      dirty.value = false;
      return true;
    } catch (err) {
      if (err instanceof WarppValidationError) {
        diagnostics.value = err.diagnostics;
        return false;
      }
      throw err;
    }
  }

  async function runValidate(): Promise<void> {
    if (!doc.value) return;
    const resp = await validateWorkflow(doc.value);
    diagnostics.value = resp.diagnostics ?? [];
  }

  return {
    doc,
    canvas,
    catalog,
    selectedPath,
    diagnostics,
    dirty,
    workflows,
    manifestByType,
    nodeAtPath,
    flowNodes,
    flowEdges,
    addNode,
    removeNode,
    setLiteral,
    clearInput,
    wire,
    unwire,
    setNamedVar,
    setPosition,
    loadCatalog,
    loadList,
    load,
    create,
    remove,
    save,
    runValidate,
  };
});
