<script setup lang="ts">
import { computed } from 'vue'
import { useProjectsStore } from '@/stores/projects'

const props = defineProps<{
  path: string
  depth: number
  selected?: string
  isExpanded: (p: string) => boolean
  toggle: (p: string) => void | Promise<void>
}>()

const emit = defineEmits<{
  (e: 'select', path: string): void
  (e: 'open-dir', path: string): void
  (e: 'delete', path: string): void
}>()

const store = useProjectsStore()
const key = computed(() => `${store.currentProjectId}:${props.path || '.'}`)
const list = computed(() => store.treeByPath[key.value] || [])
const children = computed(() => list.value.filter((e: any) => e.isDir && props.isExpanded(e.path)))

function select(path: string) {
  emit('select', path)
}
function openDir(path: string) {
  emit('open-dir', path)
}
function del(path: string) {
  emit('delete', path)
}
</script>

<template>
  <ul>
    <li
      v-for="e in list"
      :key="e.path"
      class="group flex items-center gap-2 h-9 pr-2 border-b border-border/70 last:border-b-0 hover:bg-surface-muted cursor-pointer"
      :class="{ 'bg-surface-muted': selected === e.path }"
    >
      <div class="flex items-center shrink-0" :style="{ paddingLeft: `${depth * 16}px` }">
        <button
          v-if="e.isDir"
          class="w-5 h-5 mr-1 rounded-3 text-subtle-foreground hover:bg-surface-muted/70 focus-visible:outline-none focus-visible:shadow-outline"
          :title="isExpanded(e.path) ? 'Collapse' : 'Expand'"
          @click.stop="toggle(e.path)"
        >
          {{ isExpanded(e.path) ? '▾' : '▸' }}
        </button>
        <span v-else class="w-5 h-5 mr-1" />
        <span class="w-5 text-subtle-foreground">{{ e.isDir ? '📁' : '📄' }}</span>
      </div>
      <span
        class="text-foreground truncate flex-1 min-w-0"
        @click.stop="e.isDir ? openDir(e.path) : select(e.path)"
      >
        {{ e.name }}
      </span>
      <span class="ml-auto text-xs text-faint-foreground">{{ e.isDir ? '' : `${e.sizeBytes} B` }}</span>
      <button
        class="ml-2 h-8 px-2 rounded-4 border border-danger/40 text-danger hover:bg-danger/10 focus-visible:outline-none focus-visible:shadow-outline"
        title="Delete"
        @click.stop="del(e.path)"
      >
        Delete
      </button>
    </li>
    <li v-for="e in children" :key="e.path + '__children'" class="border-0 p-0 m-0">
      <FileTreeNode
        :path="e.path"
        :depth="depth + 1"
        :selected="selected"
        :is-expanded="isExpanded"
        :toggle="toggle"
        @select="emit('select', $event)"
        @open-dir="emit('open-dir', $event)"
        @delete="emit('delete', $event)"
      />
    </li>
  </ul>
</template>

