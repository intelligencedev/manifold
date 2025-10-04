<template>
  <section class="flex flex-col h-full min-h-0">
    <header class="flex items-center justify-between py-4">
      <div>
        <h1 class="text-2xl font-semibold text-foreground">Specialists</h1>
        <p class="text-sm text-subtle-foreground">Manage configured specialists used by the orchestrator.</p>
      </div>
      <button @click="startCreate" class="rounded-lg border border-border/70 px-3 py-2 text-sm font-semibold text-muted-foreground hover:border-border">New</button>
    </header>

    <div v-if="actionError" class="rounded-2xl border border-danger/60 bg-danger/10 p-3 text-danger-foreground text-sm">
      {{ actionError }}
    </div>

    <!-- two equal columns; nested areas scroll but view itself doesn't -->
    <div class="flex gap-6 flex-1 min-h-0">
      <!-- left: list -->
      <div class="w-1/2 min-w-0 rounded-2xl border border-border/70 bg-surface p-4 min-h-0 overflow-auto">
        <div class="w-full text-sm min-w-0">
          <table class="w-full text-sm">
            <thead class="text-subtle-foreground">
              <tr>
                <th class="text-left py-2">Name</th>
                <th class="text-left py-2">Model</th>
                <th class="text-left py-2">Tools</th>
                <th class="text-left py-2">Paused</th>
                <th class="text-right py-2">Actions</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="s in specialists" :key="s.name" class="border-t border-border/50">
                <td class="py-2 font-medium">{{ s.name }}</td>
                <td class="py-2">{{ s.model }}</td>
                <td class="py-2">{{ s.enableTools ? 'enabled' : 'disabled' }}</td>
                <td class="py-2"><span :class="s.paused ? 'text-warning-foreground' : 'text-success-foreground'">{{ s.paused ? 'yes' : 'no' }}</span></td>
                <td class="py-2 text-right space-x-2">
                  <button @click="edit(s)" class="rounded border border-border/70 px-2 py-1">Edit</button>
                  <button @click="togglePause(s)" class="rounded border border-border/70 px-2 py-1">{{ s.paused ? 'Resume' : 'Pause' }}</button>
                  <button @click="remove(s)" class="rounded border border-danger/60 text-danger/60 px-2 py-1">Delete</button>
                </td>
              </tr>
              <tr v-if="loading"><td colspan="5" class="py-4 text-center text-faint-foreground">Loading…</td></tr>
              <tr v-if="error"><td colspan="5" class="py-4 text-center text-danger-foreground">Failed to load.</td></tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- right: editor -->
      <div class="w-1/2 min-w-0 min-h-0">
        <div v-if="editing" class="rounded-2xl border border-border/70 bg-surface p-4 h-full min-h-0 overflow-auto flex flex-col">
          <div class="space-y-3 h-full min-h-0 flex flex-col">
            <h2 class="text-lg font-semibold">{{ form.name ? 'Edit' : 'Create' }} Specialist</h2>
            <div class="grid gap-3 md:grid-cols-2 flex-1 min-h-0">
              <label class="text-sm">
                <div class="text-subtle-foreground">Name</div>
                <input v-model="form.name" class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2" :disabled="!!original?.name" />
              </label>
              <label class="text-sm">
                <div class="text-subtle-foreground">Model</div>
                <input v-model="form.model" class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2" />
              </label>
              <label class="text-sm md:col-span-2">
                <div class="text-subtle-foreground">Base URL</div>
                <input v-model="form.baseURL" class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2" />
              </label>
              <label class="text-sm md:col-span-2">
                <div class="text-subtle-foreground">API Key</div>
                <input v-model="form.apiKey" type="password" class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2" />
              </label>
              <label class="text-sm">
                <div class="text-subtle-foreground">Enable Tools</div>
                <input type="checkbox" v-model="form.enableTools" />
              </label>
              <label class="text-sm">
                <div class="text-subtle-foreground">Paused</div>
                <input type="checkbox" v-model="form.paused" />
              </label>

              <!-- system prompt: take available vertical space -->
              <div class="md:col-span-2 flex flex-col gap-3 row-span-1 col-span-full">
                <div class="text-subtle-foreground text-sm">System Prompt</div>
                <div class="flex-1 min-h-0">
                  <textarea v-model="form.system" class="w-full h-full min-h-0 resize-none rounded border border-border/70 bg-surface-muted/60 px-3 py-2"></textarea>
                </div>
              </div>

              <!-- prompt/version selectors (auto-load on version selection) -->
              <div class="md:col-span-2 rounded-lg border border-border/60 bg-surface-muted/30 p-3 space-y-2">
                <div class="text-sm font-medium">Apply saved prompt version</div>
                <div class="grid gap-2 md:grid-cols-2">
                  <label class="text-sm">
                    <div class="text-subtle-foreground">Prompt</div>
                    <select v-model="promptApply.promptId" @change="onSelectPrompt" class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2">
                      <option value="">Select prompt</option>
                      <option v-for="p in availablePrompts" :key="p.id" :value="p.id">{{ p.name }}</option>
                    </select>
                  </label>
                  <label class="text-sm">
                    <div class="text-subtle-foreground">Version</div>
                    <select v-model="promptApply.versionId" @change="onSelectVersion" :disabled="!promptApply.promptId || versionsLoading" class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2">
                      <option value="">Select version</option>
                      <option v-for="v in availableVersions" :key="v.id" :value="v.id">{{ v.semver || formatDate(v.createdAt) }}</option>
                    </select>
                  </label>
                </div>
                <div class="flex items-center gap-2">
                  <span v-if="applyVersionError" class="text-sm text-danger-foreground">{{ applyVersionError }}</span>
                </div>
              </div>

            </div>

            <div class="flex gap-2">
              <button @click="save" class="rounded-lg border border-border/70 px-3 py-2 text-sm font-semibold">Save</button>
              <button @click="cancel" class="rounded-lg border border-border/70 px-3 py-2 text-sm">Cancel</button>
            </div>
          </div>
        </div>
        <div v-else class="rounded-2xl border border-border/70 bg-surface p-6 h-full min-h-0 flex items-center justify-center text-sm text-subtle-foreground">
          Select a specialist or click New to create one.
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { listSpecialists, upsertSpecialist, deleteSpecialist, type Specialist } from '@/api/client'
import { listPrompts, listPromptVersions, type Prompt, type PromptVersion } from '@/api/playground'

const qc = useQueryClient()
const { data, isLoading: loading, isError: error } = useQuery({ queryKey: ['specialists'], queryFn: listSpecialists, staleTime: 5_000 })
const specialists = computed(() => data.value ?? [])

const editing = ref(false)
const original = ref<Specialist | null>(null)
const form = ref<Specialist>({ name: '', model: '', baseURL: '', apiKey: '', enableTools: false, paused: false })
const actionError = ref<string | null>(null)

// Playground prompts integration
const availablePrompts = ref<Prompt[]>([])
const availableVersions = ref<PromptVersion[]>([])
const promptsLoading = ref(false)
const versionsLoading = ref(false)
const applyVersionError = ref<string | null>(null)
const promptApply = ref<{ promptId: string; versionId: string }>({ promptId: '', versionId: '' })

function setErr(e: unknown, fallback: string) {
  actionError.value = null
  const anyErr = e as any
  const msg = anyErr?.response?.data || anyErr?.message || fallback
  actionError.value = String(msg)
}

function startCreate() {
  original.value = null
  form.value = { name: '', model: '', baseURL: '', apiKey: '', enableTools: false, paused: false }
  editing.value = true
  void ensurePromptsLoaded()
}
function edit(s: Specialist) {
  original.value = s
  form.value = { ...s }
  editing.value = true
  void ensurePromptsLoaded()
}
async function save() {
  try {
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
</script>

