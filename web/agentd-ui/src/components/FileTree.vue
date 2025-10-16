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
  (e: 'moved', payload: { from: string; to: string }): void
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
  const next = new Set(checked.value)
  if (next.has(path)) next.delete(path)
  else next.add(path)
  checked.value = next
}
function clearChecks() {
  checked.value = new Set()
}
defineExpose({
  isChecked,
  toggleCheck,
  clearChecks,
  checked,
})

type DragKind = 'file' | 'dir'

function dragData(event: DragEvent) {
  const dt = event.dataTransfer
  const path = (dt?.getData('application/x-project-path') || dt?.getData('text/plain') || '').trim()
  const kindRaw = (dt?.getData('application/x-project-kind') || '').trim()
  const kind: DragKind = kindRaw === 'dir' ? 'dir' : 'file'
  return { path, kind }
}

function baseName(path: string) {
  const clean = path.replace(/^\.\/+/, '').replace(/\/+$/, '')
  const parts = clean.split('/').filter(Boolean)
  return parts.pop() || clean
}

function normalizeDir(dir: string) {
  if (!dir || dir === '.') return '.'
  const withoutLeading = dir.replace(/^\.\/+/, '')
  const withoutTrailing = withoutLeading.replace(/\/+$/, '')
  return withoutTrailing || '.'
}

function buildDestination(dir: string, name: string) {
  const normalizedDir = normalizeDir(dir)
  if (!name) return normalizedDir === '.' ? '' : normalizedDir
  if (!normalizedDir || normalizedDir === '.') return name
  return `${normalizedDir}/${name}`
}

function canAcceptMove(src: string, dest: string, kind: DragKind) {
  if (!src || !dest) return false
  if (src === dest) return false
  if (kind === 'dir' && (dest === src || dest.startsWith(`${src}/`))) {
    return false
  }
  return true
}

function onRootDragOver(event: DragEvent) {
  const { path, kind } = dragData(event)
  const dest = buildDestination(rootPath.value, baseName(path))
  if (!canAcceptMove(path, dest, kind)) {
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'none'
    return
  }
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
}

async function onRootDrop(event: DragEvent) {
  const { path, kind } = dragData(event)
  const base = baseName(path)
  const dest = buildDestination(rootPath.value, base)
  if (!canAcceptMove(path, dest, kind)) return
  try {
    await store.movePath(path, dest)
    emit('moved', { from: path, to: dest })
  } catch (err) {
    console.error('move failed', err)
  }
}

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
    <div
      class="flex items-center gap-2 h-9 pl-3 pr-2 bg-surface-muted text-subtle-foreground shrink-0"
      @dragover.prevent="onRootDragOver"
      @drop.prevent="onRootDrop"
    >
      <span class="w-5" />
      <span class="w-5">🗂️</span>
      <span class="text-xs uppercase tracking-wide">Root</span>
    </div>
    <div
      class="min-h-0 flex-1 overflow-auto"
      @dragover.prevent.self="onRootDragOver"
      @drop.prevent.self="onRootDrop"
    >
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
        @moved="emit('moved', $event)"
      />
    </div>
  </div>
</template>
