<template>
  <div class="warpp-inspector">
    <template v-if="node && manifest">
      <div class="warpp-inspector__header">
        <div>
          <div class="warpp-inspector__title">{{ manifest.title }}</div>
          <div class="warpp-inspector__id">{{ node.id }}</div>
        </div>
        <button class="warpp-inspector__delete" @click="onDelete">Delete</button>
      </div>

      <div
        v-for="port in scalarInputs"
        :key="port.name"
        class="warpp-inspector__field"
      >
        <label>{{ port.name }} <span class="warpp-inspector__type">{{ port.type }}</span></label>
        <div v-if="isWired(port.name)" class="warpp-inspector__wired">
          wired from {{ wiredFrom(port.name) }}
          <button @click="editor.clearInput(selectedPath!, port.name)">unwire</button>
        </div>
        <template v-else>
          <input
            v-if="widget(port.type) === 'text'"
            :value="literalString(port.name)"
            @input="onLiteral(port.name, ($event.target as HTMLInputElement).value)"
          />
          <input
            v-else-if="widget(port.type) === 'number'"
            type="number"
            :value="literalString(port.name)"
            @input="onLiteral(port.name, Number(($event.target as HTMLInputElement).value))"
          />
          <input
            v-else-if="widget(port.type) === 'boolean'"
            type="checkbox"
            :checked="literalBool(port.name)"
            @change="onLiteral(port.name, ($event.target as HTMLInputElement).checked)"
          />
          <textarea
            v-else
            :value="literalString(port.name)"
            @blur="onJSON(port.name, ($event.target as HTMLTextAreaElement).value)"
          />
        </template>
      </div>
    </template>

    <template v-else-if="doc">
      <div class="warpp-inspector__title">Workflow</div>
      <label>Name</label>
      <input v-model="doc.name" @input="markDirty" />
      <label class="warpp-inspector__check">
        <input type="checkbox" :checked="publishTool" @change="onPublish($event)" />
        Publish as agent tool
      </label>
    </template>

    <div v-else class="warpp-inspector__empty">Select a node to edit its inputs.</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { parseType } from "@/lib/warppTypes";
import { useWarppEditor } from "@/stores/warppEditor";
import type { WarppBinding } from "@/types/warpp";

const editor = useWarppEditor();

const selectedPath = computed(() => editor.selectedPath);
const node = computed(() =>
  selectedPath.value ? editor.nodeAtPath(selectedPath.value) : undefined,
);
const manifest = computed(() =>
  node.value ? editor.manifestByType(node.value.type) : undefined,
);
const doc = computed(() => editor.doc);
const publishTool = computed(() => Boolean(editor.doc?.publish?.tool));

const scalarInputs = computed(() =>
  (manifest.value?.inputs ?? []).filter((p) => !p.variadic),
);

function widget(type: string): string {
  const t = parseType(type);
  if (t.kind === "number") return "number";
  if (t.kind === "boolean") return "boolean";
  if (t.kind === "text" || t.kind === "file") return "text";
  return "json";
}

function binding(port: string): WarppBinding | undefined {
  const input = node.value?.inputs?.[port];
  if (input && !Array.isArray(input) && "from" in input) return input;
  if (input && !Array.isArray(input) && "value" in input) return input;
  return undefined;
}

function isWired(port: string): boolean {
  return typeof binding(port)?.from === "string";
}

function wiredFrom(port: string): string {
  return binding(port)?.from ?? "";
}

function literalString(port: string): string {
  const v = binding(port)?.value;
  if (v === undefined || v === null) return "";
  return typeof v === "string" ? v : JSON.stringify(v);
}

function literalBool(port: string): boolean {
  return binding(port)?.value === true;
}

function onLiteral(port: string, value: unknown): void {
  if (selectedPath.value) editor.setLiteral(selectedPath.value, port, value);
}

function onJSON(port: string, raw: string): void {
  if (!selectedPath.value) return;
  try {
    editor.setLiteral(selectedPath.value, port, JSON.parse(raw));
  } catch {
    // leave prior value on invalid JSON
  }
}

function onDelete(): void {
  if (selectedPath.value) {
    editor.removeNode(selectedPath.value);
    editor.selectedPath = null;
  }
}

function onPublish(e: Event): void {
  if (!editor.doc) return;
  editor.doc.publish = { tool: (e.target as HTMLInputElement).checked };
  markDirty();
}

function markDirty(): void {
  editor.dirty = true;
}
</script>

<style scoped>
.warpp-inspector {
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  overflow-y: auto;
  font-size: 12px;
}
.warpp-inspector__header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}
.warpp-inspector__title {
  font-weight: 600;
}
.warpp-inspector__id {
  opacity: 0.6;
}
.warpp-inspector__type {
  opacity: 0.5;
}
.warpp-inspector__field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.warpp-inspector__field input,
.warpp-inspector__field textarea {
  width: 100%;
  padding: 5px 7px;
  border-radius: 6px;
  border: 1px solid var(--halo-border, #2b3242);
  background: var(--halo-surface, #161a22);
  color: inherit;
}
.warpp-inspector__wired {
  display: flex;
  gap: 6px;
  align-items: center;
  opacity: 0.85;
}
.warpp-inspector__delete {
  color: #ff8f8f;
  background: transparent;
  border: 1px solid var(--halo-border, #2b3242);
  border-radius: 6px;
  padding: 3px 8px;
  cursor: pointer;
}
.warpp-inspector__empty {
  opacity: 0.6;
}
.warpp-inspector__check {
  display: flex;
  gap: 6px;
  align-items: center;
}
</style>
