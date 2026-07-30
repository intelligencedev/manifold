<template>
  <div class="warpp-node" :class="statusClass">
    <div class="warpp-node__title">
      <span class="warpp-node__type">{{ manifest?.title || node.type }}</span>
      <span class="warpp-node__id">{{ node.id }}</span>
    </div>

    <div class="warpp-node__ports">
      <div class="warpp-node__col">
        <div
          v-for="port in manifest?.inputs ?? []"
          :key="'in-' + port.name"
          class="warpp-node__port"
        >
          <Handle
            :id="port.name"
            type="target"
            :position="Position.Left"
            :style="{ background: portColor(port.type) }"
          />
          <span class="warpp-node__portname">{{ port.name }}</span>
        </div>
      </div>
      <div class="warpp-node__col warpp-node__col--out">
        <div
          v-for="port in manifest?.outputs ?? []"
          :key="'out-' + port.name"
          class="warpp-node__port warpp-node__port--out"
        >
          <span class="warpp-node__portname">{{ port.name }}</span>
          <Handle
            :id="port.name"
            type="source"
            :position="Position.Right"
            :style="{ background: portColor(port.type) }"
          />
        </div>
      </div>
    </div>

    <div v-if="outputPreview" class="warpp-node__preview">{{ outputPreview }}</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { Handle, Position, type NodeProps } from "@vue-flow/core";
import { portColor } from "@/lib/warppTypes";
import { useWarppEditor } from "@/stores/warppEditor";
import { useWarppRun } from "@/stores/warppRun";
import type { WarppNode } from "@/types/warpp";

const props = defineProps<NodeProps>();

const editor = useWarppEditor();
const run = useWarppRun();

const node = computed<WarppNode>(() => props.data.node as WarppNode);
const manifest = computed(() => editor.manifestByType(node.value.type));

const statusClass = computed(() => {
  const s = run.nodeStatus[props.id];
  return s ? `is-${s}` : "";
});

const outputPreview = computed(() => {
  const out = run.nodeOutputs[props.id];
  if (!out) return "";
  const parts = Object.entries(out).map(([k, v]) => {
    const s = typeof v === "string" ? v : JSON.stringify(v);
    return `${k}: ${s}`;
  });
  const text = parts.join("  ");
  return text.length > 120 ? text.slice(0, 117) + "…" : text;
});
</script>

<style scoped>
.warpp-node {
  min-width: 180px;
  border: 1px solid var(--halo-border, #2b3242);
  border-radius: 8px;
  background: var(--halo-surface, #161a22);
  font-size: 12px;
  color: var(--halo-text, #e6e9ef);
}
.warpp-node.is-running {
  border-color: #6fb3ff;
}
.warpp-node.is-completed {
  border-color: #8bd17c;
}
.warpp-node.is-failed {
  border-color: #ff6f6f;
}
.warpp-node.is-skipped {
  opacity: 0.55;
}
.warpp-node__title {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  padding: 6px 10px;
  border-bottom: 1px solid var(--halo-border, #2b3242);
}
.warpp-node__type {
  font-weight: 600;
}
.warpp-node__id {
  opacity: 0.6;
}
.warpp-node__ports {
  display: flex;
  justify-content: space-between;
  padding: 6px 0;
}
.warpp-node__col {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.warpp-node__col--out {
  align-items: flex-end;
}
.warpp-node__port {
  position: relative;
  padding: 2px 10px;
}
.warpp-node__portname {
  opacity: 0.85;
}
.warpp-node__preview {
  padding: 4px 10px;
  border-top: 1px solid var(--halo-border, #2b3242);
  opacity: 0.75;
  word-break: break-word;
}
</style>
