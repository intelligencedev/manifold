import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { ProjectSummary, FileEntry } from '@/api/client'
import { listProjects, createProject, deleteProject, listProjectTree, uploadFile, deletePath, createDir, moveProjectPath } from '@/api/client'

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

  function normalizePath(path: string) {
    const trimmed = path.trim()
    if (!trimmed || trimmed === '.') return '.'
    const withoutLeading = trimmed.replace(/^\.?\/+/, '')
    const collapsed = withoutLeading.replace(/\/{2,}/g, '/')
    const withoutTrailing = collapsed.replace(/\/+$/, '')
    return withoutTrailing || '.'
  }

  function invalidateCachedSubtree(projectID: string, prefix: string) {
    if (!prefix) return
    const cleanPrefix = normalizePath(prefix)
    const keyPrefix = `${projectID}:${cleanPrefix}`
    for (const key of Object.keys(treeByPath.value)) {
      if (key === keyPrefix || key.startsWith(`${keyPrefix}/`)) {
        delete treeByPath.value[key]
      }
    }
  }

  function parentPath(path: string) {
    const clean = normalizePath(path)
    if (clean === '.' || clean === '') return '.'
    const idx = clean.lastIndexOf('/')
    if (idx === -1) return '.'
    const parent = clean.slice(0, idx)
    return parent || '.'
  }

  async function movePath(from: string, to: string) {
    if (!currentProjectId.value) return
    const projectID = currentProjectId.value
    const src = normalizePath(from)
    const dest = normalizePath(to)
    if (!src || src === '.' || !dest || dest === '.') return
    if (src === dest) return
    await moveProjectPath(projectID, src, dest)
    const srcParent = parentPath(src)
    const destParent = parentPath(dest)
    invalidateCachedSubtree(projectID, src)
    await ensureTree(srcParent)
    if (destParent !== srcParent) {
      await ensureTree(destParent)
    }
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
    movePath,
    upload,
    create,
    remove,
  }
})
