import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { ProjectSummary, FileEntry } from '@/api/client'
import { listProjects, createProject, deleteProject, listProjectTree, uploadFile, deletePath, createDir } from '@/api/client'

export const useProjectsStore = defineStore('projects', () => {
  const projects = ref<ProjectSummary[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const currentProjectId = ref<string>('')
  const treeByPath = ref<Record<string, FileEntry[]>>({})

  async function refresh() {
    loading.value = true
    error.value = null
    try {
      projects.value = await listProjects()
      if (projects.value.length && !projects.value.find(p => p.id === currentProjectId.value)) {
        currentProjectId.value = projects.value[0].id
      }
    } catch (e) {
      error.value = 'Failed to load projects'
      console.error(e)
    } finally {
      loading.value = false
    }
  }

  async function setCurrent(id: string) {
    currentProjectId.value = id
  }

  async function ensureTree(path = '.') {
    const id = currentProjectId.value
    if (!id) return
    const key = `${id}:${path}`
    treeByPath.value[key] = await listProjectTree(id, path)
  }

  async function makeDir(path: string) {
    if (!currentProjectId.value) return
    await createDir(currentProjectId.value, path)
    await ensureTree(path.split('/').slice(0, -1).join('/') || '.')
  }

  async function removePath(path: string) {
    if (!currentProjectId.value) return
    await deletePath(currentProjectId.value, path)
    await ensureTree(path.split('/').slice(0, -1).join('/') || '.')
  }

  async function upload(path: string, file: File) {
    if (!currentProjectId.value) return
    await uploadFile(currentProjectId.value, path, file)
    await ensureTree(path || '.')
  }

  async function create(name: string) {
    const p = await createProject(name)
    projects.value = [p, ...projects.value]
    currentProjectId.value = p.id
  }

  async function remove(id: string) {
    await deleteProject(id)
    projects.value = projects.value.filter(p => p.id !== id)
    if (currentProjectId.value === id) currentProjectId.value = projects.value[0]?.id || ''
  }

  return {
    // state
    projects,
    loading,
    error,
    currentProjectId,
    treeByPath,
    // actions
    refresh,
    setCurrent,
    ensureTree,
    makeDir,
    removePath,
    upload,
    create,
    remove,
  }
})

