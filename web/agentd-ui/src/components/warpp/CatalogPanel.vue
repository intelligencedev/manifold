<template>
  <div class="warpp-catalog">
    <input
      v-model="query"
      class="warpp-catalog__search"
      placeholder="Search nodes…"
    />
    <div v-for="group in groups" :key="group.category" class="warpp-catalog__group">
      <div class="warpp-catalog__heading">{{ group.category }}</div>
      <button
        v-for="m in group.items"
        :key="m.type"
        class="warpp-catalog__item"
        :title="m.description"
        @click="emit('add', m.type)"
      >
        {{ m.title }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { useWarppEditor } from "@/stores/warppEditor";
import type { WarppManifest } from "@/types/warpp";

const emit = defineEmits<{ (e: "add", type: string): void }>();
const editor = useWarppEditor();
const query = ref("");

const groups = computed(() => {
  const q = query.value.trim().toLowerCase();
  const manifests = (editor.catalog?.manifests ?? []).filter(
    (m) =>
      !q ||
      m.type.toLowerCase().includes(q) ||
      m.title.toLowerCase().includes(q),
  );
  const byCat = new Map<string, WarppManifest[]>();
  for (const m of manifests) {
    const list = byCat.get(m.category) ?? [];
    list.push(m);
    byCat.set(m.category, list);
  }
  return [...byCat.entries()].map(([category, items]) => ({ category, items }));
});
</script>

<style scoped>
.warpp-catalog {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px;
  overflow-y: auto;
}
.warpp-catalog__search {
  width: 100%;
  padding: 6px 8px;
  border-radius: 6px;
  border: 1px solid var(--halo-border, #2b3242);
  background: var(--halo-surface, #161a22);
  color: inherit;
}
.warpp-catalog__heading {
  font-size: 11px;
  text-transform: uppercase;
  opacity: 0.6;
  margin: 8px 0 4px;
}
.warpp-catalog__item {
  display: block;
  width: 100%;
  text-align: left;
  padding: 6px 8px;
  border-radius: 6px;
  border: 1px solid transparent;
  background: transparent;
  color: inherit;
  cursor: pointer;
}
.warpp-catalog__item:hover {
  border-color: var(--halo-border, #2b3242);
  background: var(--halo-surface-hover, #1c2230);
}
</style>
