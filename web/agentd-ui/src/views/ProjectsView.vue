<script setup lang="ts">
import { onMounted, ref, computed, watch } from 'vue'
import { useProjectsStore } from '@/stores/projects'
import { projectFileUrl } from '@/api/client'

const store = useProjectsStore()
const newProjectName = ref('')
const uploadInput = ref<HTMLInputElement | null>(null)
const cwd = ref('.')
const selectedFile = ref<string>('')
const previewUrl = computed(() => {
  if (!store.currentProjectId || !selectedFile.value) return ''
  return projectFileUrl(store.currentProjectId, selectedFile.value)
})

onMounted(() => {
  void store.refresh().then(() => store.ensureTree(cwd.value))
})

const current = computed(() => store.projects.find(p => p.id === store.currentProjectId) || null)
const entries = computed(() => store.treeByPath[`${store.currentProjectId}:${cwd.value}`] || [])

function pickUpload() {
  uploadInput.value?.click()
}

async function onFiles(e: Event) {
  const input = e.target as HTMLInputElement
  const files = input.files
  if (!files || !files.length) return
  for (const f of Array.from(files)) {
    await store.upload(cwd.value, f)
  }
  input.value = ''
}

async function mkdir() {
  const name = prompt('Folder name?')
  if (!name) return
  const path = (cwd.value === '.' ? '' : cwd.value + '/') + name
  await store.makeDir(path)
  await store.ensureTree(cwd.value)
}

async function delPath(p: string) {
  if (!confirm(`Delete ${p}?`)) return
  await store.removePath(p)
  await store.ensureTree(cwd.value)
  if (selectedFile.value === p) selectedFile.value = ''
}

async function openDir(path: string) {
  cwd.value = path || '.'
  await store.ensureTree(cwd.value)
  selectedFile.value = ''
}

async function createProject() {
  const name = newProjectName.value.trim()
  if (!name) return
  await store.create(name)
  newProjectName.value = ''
  cwd.value = '.'
  await store.ensureTree('.')
}

function openFile(path: string) {
  selectedFile.value = path
}
</script>

<template>
  <div class="p-4 space-y-6">
    <h1 class="text-xl font-semibold">Projects</h1>

    <div class="flex items-center gap-2">
      <input v-model="newProjectName" placeholder="New project name" class="input input-bordered input-sm" />
      <button class="btn btn-sm btn-primary" @click="createProject">Create</button>
      <select class="select select-sm select-bordered ml-4" v-model="store.currentProjectId" @change="() => { cwd='.'; store.ensureTree('.') }">
        <option v-for="p in store.projects" :key="p.id" :value="p.id">{{ p.name }}</option>
      </select>
      <button v-if="store.currentProjectId" class="btn btn-sm btn-error ml-2" @click="() => store.remove(store.currentProjectId)">Delete</button>
      <span class="ml-auto text-sm text-gray-500" v-if="current">Created {{ new Date(current.createdAt).toLocaleString() }} · {{ current.files }} files · {{ (current.sizeBytes/1024).toFixed(1) }} KB</span>
    </div>

    <div v-if="store.currentProjectId" class="grid grid-cols-2 gap-4">
      <div class="border rounded p-3">
        <div class="flex items-center gap-2 mb-2">
          <button class="btn btn-xs" @click="() => openDir('.')">Root</button>
          <div class="text-sm text-gray-500">{{ cwd }}</div>
          <button class="btn btn-xs ml-auto" @click="mkdir">New Folder</button>
          <button class="btn btn-xs" @click="pickUpload">Upload</button>
          <input ref="uploadInput" type="file" multiple class="hidden" @change="onFiles" />
        </div>
        <ul class="menu bg-base-100 w-full rounded-box">
          <li v-for="e in entries" :key="e.path">
            <a @click.prevent="e.isDir ? openDir(e.path) : openFile(e.path)" :class="{'bg-base-200': selectedFile===e.path}">
              <span v-if="e.isDir">📁</span>
              <span v-else>📄</span>
              <span class="ml-2">{{ e.name }}</span>
              <span class="ml-auto text-xs text-gray-500">{{ e.isDir ? '' : (e.sizeBytes + ' B') }}</span>
              <button class="btn btn-ghost btn-xs" @click.stop="delPath(e.path)">Delete</button>
            </a>
          </li>
        </ul>
      </div>
      <div class="border rounded p-3 min-h-[300px]">
        <div class="flex items-center justify-between text-sm text-gray-500 mb-2">
          <div>Preview</div>
          <div class="truncate max-w-[70%]" v-if="selectedFile">{{ selectedFile }}</div>
        </div>
        <div v-if="!selectedFile" class="p-2 text-gray-400">Select a file to preview</div>
        <template v-else>
          <div v-if="/\.(png|jpe?g|gif|svg|webp)$/i.test(selectedFile)" class="max-h-[480px] overflow-auto">
            <img :src="previewUrl" alt="preview" class="max-w-full rounded border border-border" />
          </div>
          <iframe v-else-if="/\.(md|txt|log|json|js|ts|go|py|java|c|cpp|yml|yaml|toml|ini|sh|csv)$/i.test(selectedFile)"
                  :src="previewUrl"
                  class="w-full h-[480px] rounded border"/>
          <div v-else class="text-sm text-gray-500">Preview not available. <a :href="previewUrl" target="_blank" class="link">Open</a></div>
        </template>
      </div>
    </div>

    <div v-else class="text-gray-500">No project selected.</div>
  </div>
  
</template>

<style scoped>
.input { @apply px-2 py-1 rounded border; }
.btn { @apply px-2 py-1 rounded border; }
.btn-primary { @apply bg-blue-600 text-white border-blue-600; }
.btn-error { @apply bg-red-600 text-white border-red-600; }
.select { @apply px-2 py-1 rounded border; }
</style>

