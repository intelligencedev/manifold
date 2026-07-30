<template>
  <div class="warpp-view">
    <header class="warpp-view__toolbar">
      <select :value="editor.doc?.id ?? ''" @change="onSelectWorkflow($event)">
        <option value="">— workflow —</option>
        <option v-for="w in editor.workflows" :key="w.id" :value="w.id">
          {{ w.name || w.id }}
        </option>
      </select>
      <input
        v-model="newId"
        class="warpp-view__newid"
        placeholder="new-workflow-id"
      />
      <button @click="onNew">New</button>
      <button :disabled="!editor.doc" @click="onSave">Save</button>
      <button :disabled="!editor.doc" @click="editor.runValidate()">Validate</button>
      <button :disabled="!editor.doc" @click="showRun = true">Run</button>
      <button @click="showTimeline = !showTimeline">Timeline</button>
      <button
        class="warpp-view__danger"
        :disabled="!editor.doc"
        title="Delete this workflow"
        @click="openDeleteConfirm"
      >
        Delete
      </button>
      <span v-if="editor.dirty" class="warpp-view__dirty">● unsaved</span>
    </header>

    <div v-if="editor.diagnostics.length" class="warpp-view__diags">
      <div
        v-for="(d, i) in editor.diagnostics"
        :key="i"
        class="warpp-view__diag"
        :class="'is-' + d.severity"
      >
        <strong>{{ d.code }}</strong> {{ d.message }}
      </div>
    </div>

    <div class="warpp-view__body">
      <aside class="warpp-view__catalog">
        <CatalogPanel @add="onAdd" />
      </aside>
      <main class="warpp-view__canvas">
        <CanvasPane v-if="editor.doc" />
        <div v-else class="warpp-view__placeholder">
          Create or open a workflow to begin.
        </div>
      </main>
      <aside class="warpp-view__inspector">
        <InspectorPanel />
      </aside>
    </div>

    <div v-if="showTimeline" class="warpp-view__drawer">
      <RunTimeline />
    </div>

    <div v-if="showRun" class="warpp-view__modal" @click.self="showRun = false">
      <div class="warpp-view__dialog">
        <div class="warpp-view__dialog-title">Run inputs</div>
        <div v-for="p in runInputs" :key="p.name" class="warpp-view__dialog-field">
          <label>{{ p.name }} <span>{{ p.type }}</span></label>
          <input v-model="runValues[p.name]" />
        </div>
        <div class="warpp-view__dialog-actions">
          <button @click="showRun = false">Cancel</button>
          <button @click="onRun">Run</button>
        </div>
      </div>
    </div>

    <div
      v-if="showDeleteConfirm"
      class="warpp-view__modal"
      @click.self="closeDeleteConfirm"
    >
      <div class="warpp-view__dialog">
        <div class="warpp-view__dialog-title">Delete workflow</div>
        <p class="warpp-view__dialog-text">
          Permanently delete
          <strong>{{ editor.doc?.name || editor.doc?.id }}</strong>? This cannot
          be undone.
        </p>
        <p v-if="deleteError" class="warpp-view__dialog-error">
          {{ deleteError }}
        </p>
        <div class="warpp-view__dialog-actions">
          <button :disabled="deletePending" @click="closeDeleteConfirm">
            Cancel
          </button>
          <button
            class="warpp-view__danger"
            :disabled="deletePending"
            @click="onDelete"
          >
            {{ deletePending ? "Deleting…" : "Delete" }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import CatalogPanel from "@/components/warpp/CatalogPanel.vue";
import CanvasPane from "@/components/warpp/CanvasPane.vue";
import InspectorPanel from "@/components/warpp/InspectorPanel.vue";
import RunTimeline from "@/components/warpp/RunTimeline.vue";
import { useWarppEditor } from "@/stores/warppEditor";
import { useWarppRun } from "@/stores/warppRun";

const editor = useWarppEditor();
const run = useWarppRun();

const newId = ref("");
const showRun = ref(false);
const showTimeline = ref(false);
const showDeleteConfirm = ref(false);
const deletePending = ref(false);
const deleteError = ref("");
const runValues = reactive<Record<string, string>>({});

const runInputs = computed(() => editor.doc?.inputs ?? []);

onMounted(async () => {
  await editor.loadCatalog();
  await editor.loadList();
});

async function onSelectWorkflow(e: Event): Promise<void> {
  const id = (e.target as HTMLSelectElement).value;
  if (id) await editor.load(id);
}

function onNew(): void {
  const id = newId.value.trim();
  if (!id) return;
  editor.create(id, id);
  newId.value = "";
}

function onAdd(type: string): void {
  const parent =
    editor.selectedPath &&
    editor.nodeAtPath(editor.selectedPath)?.type === "control.map"
      ? editor.selectedPath
      : undefined;
  editor.addNode(type, { x: 120, y: 120 }, parent);
}

async function onSave(): Promise<void> {
  const ok = await editor.save();
  if (ok) await editor.loadList();
}

function onRun(): void {
  if (!editor.doc) return;
  showRun.value = false;
  showTimeline.value = true;
  run.start(editor.doc.id, { ...runValues });
}

function openDeleteConfirm(): void {
  if (!editor.doc) return;
  deleteError.value = "";
  showDeleteConfirm.value = true;
}

function closeDeleteConfirm(): void {
  if (deletePending.value) return;
  showDeleteConfirm.value = false;
}

async function onDelete(): Promise<void> {
  const id = editor.doc?.id;
  if (!id) return;
  deletePending.value = true;
  deleteError.value = "";
  try {
    await editor.remove(id);
    showDeleteConfirm.value = false;
  } catch {
    deleteError.value = "Failed to delete workflow.";
  }
  deletePending.value = false;
}
</script>

<style scoped>
.warpp-view {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.warpp-view__toolbar {
  display: flex;
  gap: 8px;
  align-items: center;
  padding: 8px 12px;
  border-bottom: 1px solid var(--halo-border, #2b3242);
}
.warpp-view__toolbar select,
.warpp-view__toolbar input,
.warpp-view__toolbar button {
  padding: 5px 8px;
  border-radius: 6px;
  border: 1px solid var(--halo-border, #2b3242);
  background: var(--halo-surface, #161a22);
  color: inherit;
}
.warpp-view__dirty {
  color: #ffb46f;
}
.warpp-view__diags {
  border-bottom: 1px solid var(--halo-border, #2b3242);
  max-height: 120px;
  overflow: auto;
}
.warpp-view__diag {
  padding: 4px 12px;
  font-size: 12px;
}
.warpp-view__diag.is-error {
  color: #ff8f8f;
}
.warpp-view__diag.is-warning {
  color: #ffb46f;
}
.warpp-view__body {
  flex: 1;
  display: grid;
  grid-template-columns: 220px 1fr 280px;
  min-height: 0;
}
.warpp-view__catalog,
.warpp-view__inspector {
  border-right: 1px solid var(--halo-border, #2b3242);
  overflow: auto;
}
.warpp-view__inspector {
  border-right: none;
  border-left: 1px solid var(--halo-border, #2b3242);
}
.warpp-view__canvas {
  position: relative;
  min-height: 0;
}
.warpp-view__placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  opacity: 0.6;
}
.warpp-view__drawer {
  height: 200px;
  border-top: 1px solid var(--halo-border, #2b3242);
}
.warpp-view__modal {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
}
.warpp-view__dialog {
  background: var(--halo-surface, #161a22);
  border: 1px solid var(--halo-border, #2b3242);
  border-radius: 8px;
  padding: 16px;
  min-width: 320px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.warpp-view__dialog-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.warpp-view__dialog-field input {
  padding: 5px 7px;
  border-radius: 6px;
  border: 1px solid var(--halo-border, #2b3242);
  background: var(--halo-bg, #0f131a);
  color: inherit;
}
.warpp-view__dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
.warpp-view__dialog-text {
  font-size: 13px;
  line-height: 1.4;
  opacity: 0.85;
}
.warpp-view__dialog-error {
  font-size: 12px;
  color: #ff8f8f;
}
.warpp-view__toolbar button.warpp-view__danger {
  border-color: rgba(255, 143, 143, 0.5);
  color: #ff8f8f;
}
.warpp-view__toolbar button.warpp-view__danger:disabled {
  opacity: 0.5;
}
.warpp-view__dialog button.warpp-view__danger {
  border: 1px solid rgba(255, 143, 143, 0.5);
  border-radius: 6px;
  background: rgba(255, 143, 143, 0.12);
  color: #ff8f8f;
  padding: 5px 8px;
}
</style>
