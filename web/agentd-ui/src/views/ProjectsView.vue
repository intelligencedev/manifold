<script setup lang="ts">
import { onMounted, ref, computed, watch } from 'vue'
import { useProjectsStore } from '@/stores/projects'
import { projectFileUrl } from '@/api/client'
import FileTree from '@/components/FileTree.vue'

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

function onProjectChange() {
  cwd.value = '.'
  selectedFile.value = ''
  void store.ensureTree('.')
}

watch(
  () => store.currentProjectId,
  () => {
    cwd.value = '.'
    selectedFile.value = ''
    void store.ensureTree('.')
  },
)
</script>

<template>
  <section class="space-y-6">
    <header class="flex items-center gap-4">
      <h1 class="text-xl font-semibold text-foreground">Projects</h1>
      <div class="ml-auto flex items-center gap-3">
        <div class="flex items-center gap-2">
          <label class="sr-only" for="new-project">New project name</label>
          <input
            id="new-project"
            v-model="newProjectName"
            placeholder="New project name"
            class="h-10 w-64 px-4 rounded-4 border border-border bg-surface text-foreground placeholder:text-subtle-foreground focus-visible:outline-none focus-visible:shadow-outline transition ease-out-custom duration-150"
          />
          <button
            class="inline-flex items-center justify-center gap-2 h-10 px-4 rounded-4 border border-transparent bg-accent text-accent-foreground shadow-2 hover:bg-accent/90 active:shadow-1 focus-visible:outline-none focus-visible:shadow-outline transition ease-out-custom duration-150"
            @click="createProject"
          >Create</button>
        </div>
        <div class="flex items-center gap-2">
          <label class="sr-only" for="project-select">Select project</label>
          <select
            id="project-select"
            class="h-10 px-4 rounded-4 border border-border bg-surface text-foreground focus-visible:outline-none focus-visible:shadow-outline"
            v-model="store.currentProjectId"
            @change="onProjectChange"
          >
            <option v-for="p in store.projects" :key="p.id" :value="p.id">{{ p.name }}</option>
          </select>
          <button
            v-if="store.currentProjectId"
            class="inline-flex items-center justify-center gap-2 h-10 px-4 rounded-4 border border-danger/60 text-danger hover:bg-danger/10 focus-visible:outline-none focus-visible:shadow-outline transition ease-out-custom duration-150"
            @click="() => store.remove(store.currentProjectId)"
          >Delete</button>
        </div>
      </div>
    </header>

    <p v-if="current" class="text-sm text-faint-foreground">
      Created {{ new Date(current.createdAt).toLocaleString() }} · {{ current.files }} files · {{ (current.sizeBytes/1024).toFixed(1) }} KB
    </p>

    <div v-if="store.currentProjectId" class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <!-- Left: Tree Panel -->
      <div class="rounded-5 border border-border bg-surface shadow-2 p-4 lg:p-6">
        <div class="flex items-center gap-3 mb-4">
          <button
            class="h-9 px-3 rounded-4 border border-transparent text-subtle-foreground hover:bg-surface-muted focus-visible:outline-none focus-visible:shadow-outline transition ease-out-custom"
            @click="() => openDir('.')"
          >Root</button>
          <div class="text-sm text-faint-foreground truncate">{{ cwd }}</div>
          <div class="ml-auto flex items-center gap-2">
            <button
              class="h-9 px-3 rounded-4 border border-border text-foreground bg-surface hover:bg-surface-muted focus-visible:outline-none focus-visible:shadow-outline transition ease-out-custom"
              @click="mkdir"
            >New Folder</button>
            <button
              class="h-9 px-3 rounded-4 border border-border text-foreground bg-surface hover:bg-surface-muted focus-visible:outline-none focus-visible:shadow-outline transition ease-out-custom"
              @click="pickUpload"
            >Upload</button>
            <input ref="uploadInput" type="file" multiple class="sr-only" @change="onFiles" />
          </div>
        </div>

        <FileTree
          :selected="selectedFile"
          :root-path="cwd"
          @select="openFile"
          @open-dir="openDir"
          @delete="delPath"
        />
      </div>

      <!-- Right: Preview Panel -->
      <div class="rounded-5 border border-border bg-surface shadow-2 p-4 lg:p-6 min-h-[320px]">
        <div class="flex items-center justify-between text-sm text-faint-foreground mb-3">
          <div class="uppercase tracking-wide">Preview</div>
          <div class="truncate max-w-[70%] text-subtle-foreground" v-if="selectedFile">{{ selectedFile }}</div>
        </div>
        <div v-if="!selectedFile" class="p-2 text-subtle-foreground">Select a file to preview</div>
        <template v-else>
          <div v-if="/\.(png|jpe?g|gif|svg|webp)$/i.test(selectedFile)" class="max-h-[480px] overflow-auto">
            <img :src="previewUrl" alt="preview" class="max-w-full rounded-4 border border-border" />
          </div>
          <iframe
            v-else-if="/\.(md|txt|log|json|js|ts|go|py|java|c|cpp|yml|yaml|toml|ini|sh|csv)$/i.test(selectedFile)"
            :src="previewUrl"
            class="w-full h-[480px] rounded-4 border border-border"
          />
          <div v-else class="text-sm text-subtle-foreground">
            Preview not available.
            <a :href="previewUrl" target="_blank" class="text-accent hover:underline">Open</a>
          </div>
        </template>
      </div>
    </div>

    <div v-else class="rounded-5 border border-border bg-surface p-6 text-subtle-foreground">
      No project selected. Create one to get started.
    </div>
  </section>
  
</template>

<style scoped>
/* Use Tailwind utilities with theme tokens; no local component theming */
</style>

