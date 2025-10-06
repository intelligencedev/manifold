<template>
  <section class="flex flex-col h-full min-h-0">
    <div v-if="actionError" class="rounded-2xl border border-danger/60 bg-danger/10 p-3 text-danger-foreground text-sm">
      {{ actionError }}
    </div>

    <!-- list/edit layout; nested areas scroll but view itself doesn't -->
    <div class="flex gap-4 flex-1 min-h-0">
      <!-- left: list -->
      <div class="w-1/3 min-w-0 rounded-md border border-border/50 bg-surface p-3 min-h-0 overflow-auto">
        <div class="flex items-center justify-between mb-2">
          <h2 class="text-base font-semibold">Specialist Assistants</h2>
          <button @click="startCreate" class="rounded-md border border-border/60 px-2 py-1 text-xs font-medium text-muted-foreground hover:border-border">New</button>
        </div>
        <div class="w-full text-sm min-w-0">
          <table class="w-full text-sm">
            <thead class="text-subtle-foreground">
              <tr>
                <th class="text-left py-1">Name</th>
                <th class="text-left py-1">Model</th>
                <th class="text-left py-1">Tools</th>
                <th class="text-right py-1">Actions</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="s in specialists" :key="s.name" class="border-t border-border/50">
                <td class="py-1 font-medium">{{ s.name }}</td>
                <td class="py-1">{{ s.model }}</td>
                <td class="py-1">{{ s.enableTools ? 'enabled' : 'disabled' }}</td>
                <td class="py-1 text-right">
                  <div class="inline-flex items-center gap-2">
                    <button
                      type="button"
                      @click="edit(s)"
                      class="rounded border border-border/60 px-2 py-1 text-xs font-medium hover:border-border"
                    >
                      Edit
                    </button>
                    <button
                      type="button"
                      @click="togglePause(s)"
                      class="rounded border border-border/60 p-1 text-muted-foreground hover:border-border"
                      :title="s.paused ? 'Resume specialist' : 'Pause specialist'"
                      :aria-label="s.paused ? 'Resume specialist' : 'Pause specialist'"
                    >
                      <span class="sr-only">{{ s.paused ? 'Resume' : 'Pause' }}</span>
                      <SolarPlay v-if="s.paused" class="h-4 w-4" />
                      <SolarPause v-else class="h-4 w-4" />
                    </button>
                    <button
                      type="button"
                      @click="remove(s)"
                      class="rounded border border-danger/60 text-danger/70 px-2 py-1 text-xs font-medium hover:border-danger"
                    >
                      Delete
                    </button>
                  </div>
                </td>
              </tr>
              <tr v-if="loading"><td colspan="4" class="py-2 text-center text-faint-foreground">Loading…</td></tr>
              <tr v-if="error"><td colspan="4" class="py-2 text-center text-danger-foreground">Failed to load.</td></tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- right: editor -->
  <div class="w-2/3 min-w-0 min-h-0">
        <div v-if="editing" class="rounded-md border border-border/50 bg-surface p-3 h-full min-h-0 overflow-auto flex flex-col">
          <div class="flex flex-col gap-4 h-full min-h-0">
            <h2 class="text-base font-semibold">{{ form.name ? 'Edit' : 'Create' }} Specialist</h2>
            <!-- Three-column editor layout: left (settings), middle (tools), right (prompt). Implemented with explicit 12-col grid -->
            <div class="flex-1 min-h-0 grid grid-cols-1 lg:grid-cols-12 gap-3">
              <!-- Left column: core settings -->
              <div class="flex flex-col gap-2 min-h-0 overflow-auto p-0 lg:col-span-3">
                <div class="flex flex-col gap-1">
                  <label for="specialist-name" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Name</label>
                  <input id="specialist-name" v-model="form.name" class="w-full rounded border border-border/60 bg-surface-muted/40 px-2 py-1.5 text-sm" :disabled="!!original?.name" />
                </div>
                <div class="flex flex-col gap-1">
                  <label for="specialist-model" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Model</label>
                  <input id="specialist-model" v-model="form.model" class="w-full rounded border border-border/60 bg-surface-muted/40 px-2 py-1.5 text-sm" />
                </div>
                <div class="flex flex-col gap-1">
                  <label for="specialist-base-url" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Base URL</label>
                  <input id="specialist-base-url" v-model="form.baseURL" class="w-full rounded border border-border/60 bg-surface-muted/40 px-2 py-1.5 text-sm" />
                </div>
                <div class="flex flex-col gap-1">
                  <label for="specialist-api-key" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">API Key</label>
                  <input id="specialist-api-key" v-model="form.apiKey" type="password" class="w-full rounded border border-border/60 bg-surface-muted/40 px-2 py-1.5 text-sm" />
                </div>
                <div class="flex items-center gap-2 text-sm">
                  <input id="specialist-enable-tools" type="checkbox" v-model="form.enableTools" class="h-4 w-4" />
                  <label for="specialist-enable-tools" class="text-subtle-foreground">Enable Tools</label>
                </div>
                <div class="flex items-center gap-2 text-sm">
                  <input id="specialist-paused" type="checkbox" v-model="form.paused" class="h-4 w-4" />
                  <label for="specialist-paused" class="text-subtle-foreground">Paused</label>
                </div>
                <div class="flex flex-col gap-1">
                  <label for="specialist-reasoning" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Reasoning Effort</label>
                  <select id="specialist-reasoning" v-model="form.reasoningEffort" class="w-full rounded border border-border/60 bg-surface-muted/40 px-2 py-1.5 text-sm">
                    <option value="">(default)</option>
                    <option value="low">low</option>
                    <option value="medium">medium</option>
                    <option value="high">high</option>
                  </select>
                </div>
                <div class="flex flex-col gap-1">
                  <label for="specialist-extra-headers" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Extra Headers (JSON)</label>
                  <textarea id="specialist-extra-headers" v-model="extraHeadersRaw" rows="2" class="w-full rounded border border-border/60 bg-surface-muted/40 px-2 py-1.5 text-sm"></textarea>
                </div>
                <div class="flex flex-col gap-1">
                  <label for="specialist-extra-params" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Extra Params (JSON)</label>
                  <textarea id="specialist-extra-params" v-model="extraParamsRaw" rows="2" class="w-full rounded border border-border/60 bg-surface-muted/40 px-2 py-1.5 text-sm"></textarea>
                </div>
              </div>

              <!-- Middle column: tools list -->
              <div class="flex min-h-0 flex-col lg:col-span-2">
                <section class="flex min-h-0 flex-1 flex-col p-0">
                  <div class="mb-1 flex items-center justify-between">
                    <div class="text-xs font-medium text-foreground">Allowed Tools</div>
                    <div class="text-xs text-faint-foreground">{{ tools.length }} available</div>
                  </div>
                  <div v-if="toolsLoading" class="text-xs text-subtle-foreground">Loading tools…</div>
                  <div v-else-if="toolsError" class="text-xs text-danger-foreground">{{ toolsError }}</div>
                  <div v-else class="min-h-0 flex-1 overflow-auto rounded bg-surface p-1">
                    <div v-if="!tools.length" class="text-xs text-subtle-foreground">No tools available for this configuration.</div>
                    <div v-else class="flex flex-col gap-0.5">
                      <label
                        v-for="t in tools"
                        :key="t.name"
                        class="inline-flex items-center gap-2 rounded px-2 py-1 text-sm hover:bg-surface-muted/70"
                        :title="t.description ?? t.name"
                      >
                        <input
                          type="checkbox"
                          class="h-4 w-4"
                          :disabled="!form.enableTools"
                          :checked="form.allowTools?.includes(t.name)"
                          @change="() => toggleAllowedTool(t.name)"
                        />
                        <span class="truncate">{{ t.name }}</span>
                      </label>
                    </div>
                  </div>
                </section>
              </div>

              <!-- Right column: system prompt -->
              <div class="flex min-h-0 flex-col gap-2 lg:col-span-7">
                <section class="flex min-h-0 flex-1 flex-col gap-2 p-0">
                  <label for="specialist-system" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">System Prompt</label>
                  <textarea id="specialist-system" v-model="form.system" class="flex-1 min-h-0 resize-none rounded border border-border/60 bg-surface px-2 py-2 text-sm"></textarea>

                  <div class="rounded bg-surface p-2 flex flex-col gap-2 border border-border/40">
                    <div class="text-xs font-medium text-foreground">Apply saved prompt version</div>
                    <div class="grid gap-2 md:grid-cols-2">
                      <div class="flex flex-col gap-1">
                        <label for="prompt-select" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Prompt</label>
                        <select id="prompt-select" v-model="promptApply.promptId" @change="onSelectPrompt" class="w-full rounded border border-border/60 bg-surface-muted/40 px-2 py-1.5 text-sm">
                          <option value="">Select prompt</option>
                          <option v-for="p in availablePrompts" :key="p.id" :value="p.id">{{ p.name }}</option>
                        </select>
                      </div>
                      <div class="flex flex-col gap-1">
                        <label for="prompt-version-select" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Version</label>
                        <select id="prompt-version-select" v-model="promptApply.versionId" @change="onSelectVersion" :disabled="!promptApply.promptId || versionsLoading" class="w-full rounded border border-border/60 bg-surface-muted/40 px-2 py-1.5 text-sm">
                          <option value="">Select version</option>
                          <option v-for="v in availableVersions" :key="v.id" :value="v.id">{{ v.semver || formatDate(v.createdAt) }}</option>
                        </select>
                      </div>
                    </div>
                    <div v-if="applyVersionError" class="text-xs text-danger-foreground">{{ applyVersionError }}</div>
                  </div>
                </section>
              </div>
            </div>

            <div class="mt-2 flex flex-wrap gap-2">
              <button @click="save" class="rounded-md border border-border/60 px-2 py-1.5 text-sm font-semibold">Save</button>
              <button @click="cancel" class="rounded-md border border-border/60 px-2 py-1.5 text-sm">Cancel</button>
            </div>
          </div>
        </div>
        <div v-else class="rounded-md border border-border/50 bg-surface p-4 h-full min-h-0 flex items-center justify-center text-sm text-subtle-foreground">
          Select a specialist or click New to create one.
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { listSpecialists, upsertSpecialist, deleteSpecialist, type Specialist } from '@/api/client'
import { listPrompts, listPromptVersions, type Prompt, type PromptVersion } from '@/api/playground'
import { fetchWarppTools } from '@/api/warpp'
import type { WarppTool } from '@/types/warpp'
import SolarPause from '@/components/icons/SolarPause.vue'
import SolarPlay from '@/components/icons/SolarPlay.vue'

const qc = useQueryClient()
const { data, isLoading: loading, isError: error } = useQuery({ queryKey: ['specialists'], queryFn: listSpecialists, staleTime: 5_000 })
// Always present specialists sorted by name (case-insensitive)
const specialists = computed(() => {
  const list = data.value ?? []
  return [...list].sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }))
})

const editing = ref(false)
const original = ref<Specialist | null>(null)
const form = ref<Specialist>({ name: '', model: '', baseURL: '', apiKey: '', enableTools: false, paused: false, system: '', allowTools: [], reasoningEffort: '', extraHeaders: {}, extraParams: {} })
  // UI helpers for editing structured fields
const extraHeadersRaw = ref('')
const extraParamsRaw = ref('')
const actionError = ref<string | null>(null)

// Playground prompts integration
const availablePrompts = ref<Prompt[]>([])
const availableVersions = ref<PromptVersion[]>([])
const promptsLoading = ref(false)
const versionsLoading = ref(false)
const applyVersionError = ref<string | null>(null)
const promptApply = ref<{ promptId: string; versionId: string }>({ promptId: '', versionId: '' })

// Tools palette (dynamic like FlowView)
const tools = ref<WarppTool[]>([])
const toolsLoading = ref(false)
const toolsError = ref('')

async function loadTools() {
  if (toolsLoading.value) return
  toolsLoading.value = true
  toolsError.value = ''
  try {
    const resp = await fetchWarppTools().catch(() => [] as WarppTool[])
    tools.value = resp
  } catch (err: any) {
    toolsError.value = err?.message ?? 'Failed to load tools'
  } finally {
    toolsLoading.value = false
  }
}

function toggleAllowedTool(name: string) {
  const set = new Set(form.value.allowTools || [])
  if (set.has(name)) set.delete(name)
  else set.add(name)
  form.value.allowTools = Array.from(set)
}

function setErr(e: unknown, fallback: string) {
  actionError.value = null
  const anyErr = e as any
  const msg = anyErr?.response?.data || anyErr?.message || fallback
  actionError.value = String(msg)
}

function startCreate() {
  original.value = null
  form.value = { name: '', model: '', baseURL: '', apiKey: '', enableTools: false, paused: false, system: '', allowTools: [], reasoningEffort: '', extraHeaders: {}, extraParams: {} }
  extraHeadersRaw.value = ''
  extraParamsRaw.value = ''
  editing.value = true
  void ensurePromptsLoaded()
  void loadTools()
}
function edit(s: Specialist) {
  original.value = s
  form.value = { ...s }
	// populate raw editors for structured fields
  extraHeadersRaw.value = s.extraHeaders ? JSON.stringify(s.extraHeaders, null, 2) : ''
  extraParamsRaw.value = s.extraParams ? JSON.stringify(s.extraParams, null, 2) : ''
  editing.value = true
  void ensurePromptsLoaded()
  void loadTools()
}
async function save() {
  try {
	// Merge raw editor values into structured fields before saving
    if (extraHeadersRaw.value && extraHeadersRaw.value.trim().length > 0) {
      try {
        form.value.extraHeaders = JSON.parse(extraHeadersRaw.value)
      } catch (err) {
        setErr(err, 'Invalid JSON in Extra Headers')
        return
      }
    } else {
      form.value.extraHeaders = {}
    }
    if (extraParamsRaw.value && extraParamsRaw.value.trim().length > 0) {
      try {
        form.value.extraParams = JSON.parse(extraParamsRaw.value)
      } catch (err) {
        setErr(err, 'Invalid JSON in Extra Params')
        return
      }
    } else {
      form.value.extraParams = {}
    }

    await upsertSpecialist(form.value)
    actionError.value = null
    editing.value = false
    original.value = null
    await qc.invalidateQueries({ queryKey: ['specialists'] })
    await qc.invalidateQueries({ queryKey: ['agent-status'] })
  } catch (e) {
    setErr(e, 'Failed to save specialist.')
  }
}
function cancel() { editing.value = false; original.value = null }
async function togglePause(s: Specialist) {
  try {
    await upsertSpecialist({ ...s, paused: !s.paused })
    actionError.value = null
    await qc.invalidateQueries({ queryKey: ['specialists'] })
    await qc.invalidateQueries({ queryKey: ['agent-status'] })
  } catch (e) {
    setErr(e, 'Failed to update pause state.')
  }
}
async function remove(s: Specialist) {
  if (!confirm(`Delete specialist ${s.name}?`)) return
  try {
    await deleteSpecialist(s.name)
    actionError.value = null
    await qc.invalidateQueries({ queryKey: ['specialists'] })
    await qc.invalidateQueries({ queryKey: ['agent-status'] })
  } catch (e) {
    setErr(e, 'Failed to delete specialist.')
  }
}

async function ensurePromptsLoaded() {
  if (availablePrompts.value.length > 0 || promptsLoading.value) return
  try {
    promptsLoading.value = true
    availablePrompts.value = await listPrompts()
  } catch (err: any) {
    applyVersionError.value = err?.message || 'Failed to load prompts.'
  } finally {
    promptsLoading.value = false
  }
}

async function onSelectPrompt() {
  promptApply.value.versionId = ''
  availableVersions.value = []
  if (!promptApply.value.promptId) return
  try {
    versionsLoading.value = true
    availableVersions.value = await listPromptVersions(promptApply.value.promptId)
  } catch (err: any) {
    applyVersionError.value = err?.message || 'Failed to load versions.'
  } finally {
    versionsLoading.value = false
  }
}

// Auto-load selected version into the textarea
async function onSelectVersion() {
  applyVersionError.value = null
  const vid = promptApply.value.versionId
  if (!vid) return
  const v = availableVersions.value.find(x => x.id === vid)
  if (!v) {
    applyVersionError.value = 'Prompt version not found.'
    return
  }
  // Replace the current System Prompt directly with the selected version template
  form.value.system = v.template || ''
  if (!form.value.system || form.value.system.trim().length === 0) {
    applyVersionError.value = 'Selected prompt version has an empty template.'
  }
}

function formatDate(value?: string) {
  if (!value) return '—'
  try { return new Date(value).toLocaleString() } catch (_) { return value }
}

onMounted(() => {
  // Preload prompts and tools for a snappier UX; they are also loaded on edit/start
  void ensurePromptsLoaded()
  void loadTools()
})
</script>