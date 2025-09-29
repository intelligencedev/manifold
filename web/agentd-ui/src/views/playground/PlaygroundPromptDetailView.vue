<template>
  <div v-if="prompt" class="flex h-full min-h-0 flex-col gap-6 overflow-hidden">
    <!-- Prompt header/summary -->
    <section class="rounded-2xl border border-border/70 bg-surface p-4 space-y-2">
      <div class="flex items-center justify-between gap-4">
        <div>
          <h2 class="text-xl font-semibold break-words">{{ prompt.name }}</h2>
          <p class="text-sm text-subtle-foreground break-words">{{ prompt.description || 'No description provided.' }}</p>
        </div>
        <div class="flex items-center gap-2 shrink-0">
          <RouterLink to="/playground/prompts" class="text-sm text-accent hover:underline">Back to prompts</RouterLink>
          <button class="rounded border border-danger/60 text-danger/60 px-3 py-1.5 text-sm" @click="deletePrompt(promptId)">Delete</button>
        </div>
      </div>
      <div class="text-sm text-subtle-foreground break-words">Tags: {{ prompt.tags?.join(', ') || '—' }}</div>
      <div class="text-sm text-subtle-foreground">Created: {{ formatDate(prompt.createdAt) }}</div>
    </section>

    <!-- Content grid: left = versions list, right = selected details + create form -->
    <div class="flex-1 min-h-0 grid gap-6 lg:grid-cols-[minmax(0,0.7fr)_minmax(0,1.3fr)]">
      <!-- Left: Versions list (scrollable) -->
      <section class="flex min-h-0 flex-col rounded-2xl border border-border/70 bg-surface p-4 gap-3">
        <header class="flex items-center justify-between">
          <div>
            <h3 class="text-lg font-semibold">Versions</h3>
            <p class="text-sm text-subtle-foreground">Most recent first.</p>
          </div>
          <button @click="refreshVersions(promptId)" class="rounded border border-border/70 px-3 py-2 text-sm">Refresh</button>
        </header>
        <div class="flex-1 min-h-0 overflow-auto overscroll-contain pr-1">
          <table class="w-full text-sm table-fixed">
            <thead class="text-subtle-foreground sticky top-0 bg-surface">
              <tr>
                <th class="text-left py-2 pr-2">Version</th>
                <th class="text-left py-2 pr-2">Created</th>
                <th class="text-left py-2">Hash</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="version in versions"
                :key="version.id"
                class="border-t border-border/60 cursor-pointer transition-colors"
                :class="{ 'bg-accent/10': version.id === selectedVersionId, 'hover:bg-muted/60': version.id !== selectedVersionId }"
                @click="selectVersion(version.id)"
              >
                <td class="py-2 pr-2 font-medium">{{ version.semver || version.id }}</td>
                <td class="py-2 pr-2 text-subtle-foreground">{{ formatDate(version.createdAt) }}</td>
                <td class="py-2 text-xs font-mono text-subtle-foreground break-all">{{ version.contentHash || '—' }}</td>
              </tr>
              <tr v-if="versionsLoading"><td colspan="3" class="py-3 text-center text-subtle-foreground">Loading…</td></tr>
              <tr v-else-if="versions.length === 0"><td colspan="3" class="py-3 text-center text-subtle-foreground">No versions yet.</td></tr>
            </tbody>
          </table>
        </div>
      </section>

      <!-- Right: Selected version details + Create form -->
      <div class="flex min-h-0 flex-col overflow-hidden gap-6">
        <!-- Selected version details -->
        <section v-if="selectedVersion" class="flex-1 min-h-0 flex flex-col rounded-2xl border border-border/70 bg-surface p-4 gap-4">
          <header class="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h3 class="text-lg font-semibold">Selected Version</h3>
              <p class="text-sm text-subtle-foreground">
                Version {{ selectedVersion.semver || selectedVersion.id }} · Created {{ formatDate(selectedVersion.createdAt) }}
              </p>
            </div>
            <div class="flex items-center gap-2">
              <button @click="loadIntoForm(selectedVersion)" class="rounded border border-border/70 px-3 py-2 text-sm">Load into form</button>
            </div>
          </header>
        </section>
        <section v-else class="rounded-2xl border border-border/70 bg-surface p-4 text-sm text-subtle-foreground">
          Select a version to view details.
        </section>

        <!-- Create Version -->
        <section class="rounded-2xl border border-border/70 bg-surface p-4 space-y-4">
          <header>
            <h3 class="text-lg font-semibold">Create Version</h3>
            <p class="text-sm text-subtle-foreground">Provide a template and optional configuration.</p>
          </header>
          <form class="grid gap-3 md:grid-cols-2" @submit.prevent="handleCreateVersion">
            <label class="text-sm">
              <span class="text-subtle-foreground mb-1">Version (semver)</span>
              <input v-model="versionForm.semver" placeholder="1.0.0" class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2" />
            </label>
            <label class="text-sm">
              <span class="text-subtle-foreground mb-1">Variables (JSON map)</span>
              <textarea v-model="versionForm.variables" rows="3" placeholder='{"name":{"type":"string"}}' class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2"></textarea>
            </label>
            <label class="text-sm md:col-span-2">
              <span class="text-subtle-foreground mb-1">Template</span>
              <textarea v-model="versionForm.template" required rows="6" class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 font-mono text-sm"></textarea>
            </label>
            <label class="text-sm md:col-span-2">
              <span class="text-subtle-foreground mb-1">Guardrails (JSON)</span>
              <textarea v-model="versionForm.guardrails" rows="3" placeholder='{"maxTokens": 200}' class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2"></textarea>
            </label>
            <div class="md:col-span-2 flex gap-3 items-center">
              <button type="submit" class="rounded border border-border/70 px-3 py-2 text-sm font-semibold">Create version</button>
              <span v-if="createMessage" class="text-sm text-subtle-foreground">{{ createMessage }}</span>
            </div>
          </form>
        </section>
      </div>
    </div>
  </div>
  <p v-else class="text-subtle-foreground text-sm">Loading prompt…</p>
</template>

<script setup lang="ts">
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { onMounted, reactive, ref, watch, computed } from 'vue'
import { usePlaygroundStore } from '@/stores/playground'
import type { Prompt, PromptVersion } from '@/api/playground'

const route = useRoute()
const router = useRouter()
const store = usePlaygroundStore()
const promptId = ref(route.params.promptId as string)

const prompt = ref<Prompt | null>(null)
const versions = ref<PromptVersion[]>(store.promptVersions[promptId.value] ?? [])
const versionsLoading = ref(false)
const createMessage = ref('')
const versionForm = reactive({ semver: '', template: '', variables: '', guardrails: '' })
const formDirty = ref(false)

const selectedVersionId = ref<string | null>(null)
const selectedVersion = computed<PromptVersion | null>(() => {
  if (!selectedVersionId.value) return null
  return versions.value.find(v => v.id === selectedVersionId.value) ?? null
})

async function refreshVersions(id: string) {
  versionsLoading.value = true
  await store.loadPromptVersions(id)
  versions.value = store.promptVersions[id] ?? []
  versionsLoading.value = false
  ensureVersionSelection()
  prefillFromLatest()
}

onMounted(async () => {
  const ok = await loadPrompt(promptId.value)
  if (ok) {
    await refreshVersions(promptId.value)
  }
})

watch(
  () => route.params.promptId,
  async (next) => {
    if (typeof next !== 'string') return
    promptId.value = next
    versions.value = []
    selectedVersionId.value = null
    formDirty.value = false
    const ok = await loadPrompt(next)
    if (ok) {
      await refreshVersions(next)
    }
  }
)

async function handleCreateVersion() {
  await store.addPromptVersion(promptId.value, versionForm)
  createMessage.value = 'Version created.'
  versionForm.semver = ''
  versionForm.template = ''
  versionForm.variables = ''
  versionForm.guardrails = ''
  formDirty.value = false
  await refreshVersions(promptId.value)
  setTimeout(() => (createMessage.value = ''), 3000)
}

async function deletePrompt(id: string) {
  const ok = window.confirm('Delete this prompt and all versions?')
  if (!ok) return
  await store.removePrompt(id)
  await router.replace('/playground/prompts')
}

function formatDate(value?: string) {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

async function loadPrompt(id: string) {
  prompt.value = await store.ensurePrompt(id)
  if (!prompt.value) {
    await router.replace('/playground/prompts')
    return false
  }
  return true
}

function ensureVersionSelection() {
  const current = versions.value
  if (selectedVersionId.value && current.some(v => v.id === selectedVersionId.value)) {
    return
  }
  const first = current[0]
  selectedVersionId.value = first ? first.id : null
}

function asPrettyJSON(value: unknown) {
  if (!value) return ''
  try {
    return JSON.stringify(value, null, 2)
  } catch (err) {
    return String(value)
  }
}

function loadIntoForm(v: PromptVersion) {
  versionForm.semver = v.semver || ''
  versionForm.template = v.template || ''
  versionForm.variables = v.variables ? asPrettyJSON(v.variables) : ''
  versionForm.guardrails = v.guardrails ? asPrettyJSON(v.guardrails) : ''
  formDirty.value = true
}

function prefillFromLatest() {
  if (formDirty.value) return
  if (!versionForm.template && versions.value.length > 0) {
    const v = versions.value[0]
    versionForm.template = v.template || ''
    versionForm.variables = v.variables ? asPrettyJSON(v.variables) : ''
    versionForm.guardrails = v.guardrails ? asPrettyJSON(v.guardrails) : ''
    // leave semver blank by default for new version
  }
}

function selectVersion(id: string) {
  if (selectedVersionId.value === id) return
  selectedVersionId.value = id
  const v = versions.value.find(x => x.id === id)
  if (v) {
    loadIntoForm(v)
  }
}

// Track manual edits to avoid overwriting user's input
watch(
  () => ({ ...versionForm }),
  () => {
    // If user starts typing, mark dirty. Programmatic updates also trigger this,
    // but we explicitly set formDirty in loadIntoForm/prefillFromLatest accordingly.
    formDirty.value = true
  }
)
</script>
