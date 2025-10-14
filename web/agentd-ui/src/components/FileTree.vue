<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useProjectsStore } from '@/stores/projects'
import FileTreeNode from './FileTreeNode.vue'

const props = defineProps<{
  selected?: string
  cwd?: string
  rootPath?: string
}>()

const emit = defineEmits<{
  (e: 'select', path: string): void
  (e: 'open-dir', path: string): void
}>()

const store = useProjectsStore()
const rootPath = computed(() => props.rootPath ?? '.')

// Track expanded folders
const expanded = ref<Set<string>>(new Set([rootPath.value]))
// Track checked items
const checked = ref<Set<string>>(new Set())

async function ensure(path: string) {
  await store.ensureTree(path || '.')
}

function isExpanded(path: string) {
  return expanded.value.has(path || '.')
}

async function toggle(path: string) {
  const p = path || '.'
  if (expanded.value.has(p)) {
    expanded.value.delete(p)
  } else {
    expanded.value.add(p)
    await ensure(p)
  }
}

function selectFile(path: string) {
  emit('select', path)
}

function openDir(path: string) {
  emit('open-dir', path || '.')
}

function isChecked(path: string) {
  return checked.value.has(path)
}
function toggleCheck(path: string) {
  if (checked.value.has(path)) checked.value.delete(path)
  else checked.value.add(path)
}
function clearChecks() {
  checked.value.clear()
}
defineExpose({
  isChecked,
  toggleCheck,
  clearChecks,
  checked,
})

onMounted(async () => {
  if (store.currentProjectId) {
    await ensure(rootPath.value)
  }
})

watch(
  () => store.currentProjectId,
  async () => {
    expanded.value = new Set([rootPath.value])
    checked.value.clear()
    if (store.currentProjectId) await ensure(rootPath.value)
  },
)
</script>

<template>
  <div class="rounded-4 border border-border/70 overflow-hidden flex min-h-0 flex-col">
    <div class="flex items-center gap-2 h-9 px-2 bg-surface-muted text-subtle-foreground shrink-0">
      <span class="w-5" />
      <span class="w-5">🗂️</span>
      <span class="text-xs uppercase tracking-wide">Root</span>
    </div>
    <div class="min-h-0 flex-1 overflow-auto">
      <FileTreeNode
        :path="rootPath"
        :depth="0"
        :selected="selected"
        :is-expanded="isExpanded"
        :toggle="toggle"
        :is-checked="isChecked"
        :toggle-check="toggleCheck"
        @select="selectFile"
        @open-dir="openDir"
      />
    </div>
  </div>
</template>

