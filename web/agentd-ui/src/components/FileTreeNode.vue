<script setup lang="ts">
import { computed } from 'vue'
import { useProjectsStore } from '@/stores/projects'

const props = defineProps<{
  path: string
  depth: number
  selected?: string
  isExpanded: (p: string) => boolean
  toggle: (p: string) => void | Promise<void>
  isChecked: (p: string) => boolean
  toggleCheck: (p: string) => void
}>()

const emit = defineEmits<{
  (e: 'select', path: string): void
  (e: 'open-dir', path: string): void
}>()

const store = useProjectsStore()
const key = computed(() => `${store.currentProjectId}:${props.path || '.'}`)
const list = computed(() => store.treeByPath[key.value] || [])

function select(path: string) {
  emit('select', path)
}
function openDir(path: string) {
  emit('open-dir', path)
}
</script>

<template>
  <ul>
    <template v-for="e in list" :key="e.path">
      <li
        class="group flex items-center gap-2 h-9 pr-2 border-b border-border/70 last:border-b-0 hover:bg-surface-muted cursor-pointer"
        :class="{ 'bg-surface-muted': selected === e.path }"
      >
        <div class="flex items-center shrink-0" :style="{ paddingLeft: `${depth * 16}px` }">
          <input
            type="checkbox"
            class="w-5 h-5 mr-1 rounded-3 border-border text-danger focus-visible:outline-none focus-visible:shadow-outline"
            :checked="isChecked(e.path)"
            @click.stop
            @change.stop="() => toggleCheck(e.path)"
            :aria-label="`Select ${e.name}`"
          />
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
      </li>
      <li v-if="e.isDir && isExpanded(e.path)" :key="e.path + '__children'" class="border-0 p-0 m-0">
        <FileTreeNode
          :path="e.path"
          :depth="depth + 1"
          :selected="selected"
          :is-expanded="isExpanded"
          :toggle="toggle"
          :is-checked="isChecked"
          :toggle-check="toggleCheck"
          @select="emit('select', $event)"
          @open-dir="emit('open-dir', $event)"
        />
      </li>
    </template>
  </ul>
</template>

