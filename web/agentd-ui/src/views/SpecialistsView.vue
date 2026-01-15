<template>
  <section class="flex flex-col h-full min-h-0">
    <div v-if="actionError" class="rounded-2xl border border-danger/60 bg-danger/10 p-3 text-danger-foreground text-sm">
      {{ actionError }}
    </div>

    <!-- list/edit layout; nested areas scroll but view itself doesn't -->
    <div class="flex flex-col xl:flex-row gap-4 flex-1 min-h-0">
      <!-- left: card grid -->
      <div class="scrollbar-inset xl:w-1/2 min-w-0 min-h-0 overflow-auto rounded-[var(--radius-lg,26px)] glass-surface p-4">
        <div class="mb-4 flex items-center justify-between">
          <h2 class="text-base font-semibold">Specialist Assistants</h2>
          <button @click="startCreate" class="rounded-full border border-accent/50 px-3 py-1.5 text-xs font-semibold text-accent transition hover:bg-accent/10">
            New
          </button>
        </div>

        <div v-if="loading" class="rounded-[14px] border border-border/60 bg-surface-muted/20 p-4 text-sm text-faint-foreground">Loading…</div>
        <div v-else-if="error" class="rounded-[14px] border border-danger/60 bg-danger/10 p-4 text-sm text-danger-foreground">Failed to load specialists.</div>
        <div v-else-if="!specialists.length" class="rounded-[14px] border border-border/60 bg-surface-muted/20 p-4 text-sm text-faint-foreground">No specialists configured yet.</div>
        <div v-else class="grid gap-4 sm:grid-cols-1 lg:grid-cols-2">
          <GlassCard
            v-for="s in specialists"
            :key="s.name"
            class="flex flex-col transition-all duration-200 cursor-pointer"
            :class="{ 'ring-2 ring-accent/60 ring-offset-2 ring-offset-surface': isCurrentlyEditing(s.name) }"
            interactive
            @click="edit(s)"
          >
            <div class="flex items-start justify-between gap-3">
              <div>
                <h3 class="text-base font-semibold text-foreground">{{ s.name }}</h3>
                <p class="mt-1 text-[11px] uppercase tracking-wide text-subtle-foreground">Model</p>
                <p class="text-sm text-muted-foreground">{{ s.model || '—' }}</p>
              </div>
              <div class="flex items-center gap-2">
                <span
                  v-if="isCurrentlyEditing(s.name)"
                  class="rounded-full border border-accent/50 bg-accent/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-accent"
                >
                  Editing
                </span>
                <Pill :tone="s.paused ? 'danger' : 'success'" size="sm" :glow="!s.paused">
                  {{ s.paused ? 'Paused' : 'Active' }}
                </Pill>
              </div>
            </div>

            <p class="mt-4 text-sm leading-relaxed text-subtle-foreground line-clamp-4">{{ specialistDescription(s) }}</p>

            <div class="flex-grow"></div>

            <div class="mt-4 flex flex-wrap items-center gap-2 text-xs">
              <Pill :tone="s.enableTools ? 'accent' : 'neutral'" size="sm">
                {{ s.enableTools ? 'Tools enabled' : 'Tools disabled' }}
              </Pill>
              <span
                v-if="Array.isArray(s.allowTools) && s.allowTools.length > 0"
                class="inline-flex items-center rounded-full border border-white/10 bg-surface-muted/50 px-2 py-1 font-medium text-subtle-foreground"
              >
                Allow list · {{ s.allowTools.length }}
              </span>
            </div>

            <div class="mt-4 flex flex-wrap gap-2" @click.stop>
              <button
                type="button"
                @click="edit(s)"
                class="rounded-full border border-white/12 px-3 py-1.5 text-xs font-semibold text-subtle-foreground transition hover:border-accent/40 hover:text-accent"
              >
                Edit
              </button>
              <button
                type="button"
                @click="cloneSpecialist(s)"
                class="rounded-full border border-white/12 px-3 py-1.5 text-xs font-semibold text-subtle-foreground transition hover:border-accent/40 hover:text-accent"
                title="Duplicate this specialist"
              >
                Clone
              </button>
              <button
                type="button"
                @click="togglePause(s)"
                class="inline-flex items-center gap-1 rounded-full border border-white/12 px-3 py-1.5 text-xs font-semibold text-subtle-foreground transition hover:border-accent/40 hover:text-accent"
                :title="s.paused ? 'Resume specialist' : 'Pause specialist'"
                :aria-label="s.paused ? 'Resume specialist' : 'Pause specialist'"
              >
                <SolarPlay v-if="s.paused" class="h-4 w-4" />
                <SolarPause v-else class="h-4 w-4" />
                <span>{{ s.paused ? 'Resume' : 'Pause' }}</span>
              </button>
              <button
                type="button"
                @click="remove(s)"
                class="rounded-full border border-danger/60 px-3 py-1.5 text-xs font-semibold text-danger/80 transition hover:bg-danger/10"
              >
                Delete
              </button>
            </div>
          </GlassCard>
        </div>
      </div>

      <!-- right: editor -->
      <div class="xl:w-1/2 min-w-0 min-h-0">
        <GlassCard v-if="editing" class="h-full min-h-0 overflow-hidden">
          <EditSpecialistRoot
            :key="editorInitial?.name || 'new'"
            class="h-full"
            :initial="editorInitial!"
            :lockName="editorLockName"
            :credentialConfigured="editorCredentialConfigured"
            :providerDefaults="providerDefaultsMap"
            :providerOptions="providerOptions"
            @cancel="closeEditor"
            @saved="onSaved"
          />
        </GlassCard>
        <GlassCard v-else class="flex h-full min-h-0 items-center justify-center p-4 text-sm text-subtle-foreground">
          Select a specialist or click New to create one.
        </GlassCard>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { listSpecialists, upsertSpecialist, deleteSpecialist, listSpecialistDefaults, type Specialist, type SpecialistProviderDefaults } from '@/api/client'
import SolarPause from '@/components/icons/SolarPause.vue'
import SolarPlay from '@/components/icons/SolarPlay.vue'
import GlassCard from '@/components/ui/GlassCard.vue'
import Pill from '@/components/ui/Pill.vue'
import EditSpecialistRoot from '@/components/specialists/EditSpecialistRoot.vue'

const qc = useQueryClient()
const { data, isLoading: loading, isError: error } = useQuery({ queryKey: ['specialists'], queryFn: listSpecialists, staleTime: 5_000 })
const { data: providerDefaultsData } = useQuery<Record<string, SpecialistProviderDefaults>>({
  queryKey: ['specialist-defaults'],
  queryFn: listSpecialistDefaults,
  staleTime: 30_000,
})

const providerDefaultsMap = computed<Record<string, SpecialistProviderDefaults> | undefined>(() => providerDefaultsData.value)
// Always present specialists sorted by name (case-insensitive)
const specialists = computed<Specialist[]>(() => {
  const list = data.value ?? []
  const unique: Specialist[] = []
  // Deduplicate by name so orchestrator-only config overlays don't render twice.
  for (const sp of list) {
    if (!sp?.name) {
      unique.push(sp)
      continue
    }
    if (!unique.some(existing => existing.name === sp.name)) {
      unique.push(sp)
    }
  }
  // Keep stable ordering from API but present alphabetically for UX.
  return [...unique].sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }))
})

const providerOptions = computed(() => {
  const defaults = providerDefaultsMap.value
  if (defaults && typeof defaults === 'object') {
    return Object.keys(defaults).sort()
  }
  return ['openai', 'anthropic', 'google', 'local']
})

const providerDropdownOptions = computed(() => providerOptions.value.map((opt) => ({ id: opt, label: opt, value: opt })))

const editing = ref(false)
const editorInitial = ref<Specialist | null>(null)
const editorLockName = ref(false)
const editorCredentialConfigured = ref(false)
const actionError = ref<string | null>(null)

// Track which specialist is currently being edited for visual feedback
const currentEditingName = computed(() => editing.value ? editorInitial.value?.name || null : null)

function isCurrentlyEditing(name: string): boolean {
  return editing.value && editorInitial.value?.name === name
}

function statusBadgeClass(paused: boolean): string {
  return paused
    ? 'inline-flex items-center rounded-full border border-border/60 bg-border/20 px-2 py-1 text-xs font-semibold text-subtle-foreground'
    : 'inline-flex items-center rounded-full border border-success/40 bg-success/10 px-2 py-1 text-xs font-semibold text-success'
}

function toolsBadgeClass(enabled: boolean): string {
  return enabled
    ? 'inline-flex items-center rounded-full border border-success/40 bg-success/10 px-2 py-1 font-medium text-success'
    : 'inline-flex items-center rounded-full border border-border/50 bg-surface-muted/30 px-2 py-1 font-medium text-subtle-foreground'
}

function specialistDescription(s: Specialist): string {
  const primary = (s.description ?? '').trim()
  if (primary) {
    return primary
  }
  const systemSnippet = (s.system || '').trim()
  if (!systemSnippet) {
    return 'No description provided yet.'
  }
  const condensed = systemSnippet.replace(/\s+/g, ' ')
  return condensed.length > 180 ? `${condensed.slice(0, 177)}…` : condensed
}

function setErr(e: unknown, fallback: string) {
  actionError.value = null
  const anyErr = e as any
  const msg = anyErr?.response?.data || anyErr?.message || fallback
  actionError.value = String(msg)
}

function startCreate() {
  const defaultProvider = providerOptions.value[0] || 'openai'
  editorInitial.value = { name: '', description: '', provider: defaultProvider, model: '', baseURL: '', enableTools: false, paused: false, system: '', allowTools: [], extraHeaders: {}, extraParams: {} }
  editorLockName.value = false
  editorCredentialConfigured.value = false
  editing.value = true
}
function edit(s: Specialist) {
  // If already editing the same specialist, do nothing
  if (editing.value && editorInitial.value?.name === s.name) {
    return
  }
  editorInitial.value = { ...s, provider: s.provider || providerOptions.value[0] || 'openai', description: s.description ?? '', apiKey: '' }
  editorLockName.value = true
  editorCredentialConfigured.value = !!s.apiKey
  editing.value = true
}
function cloneSpecialist(s: Specialist) {
  const baseName = `${s.name || 'specialist'} copy`
  const uniqueName = generateUniqueName(baseName)
  const clonedHeaders = { ...(s.extraHeaders || {}) }
  const clonedParams = s.extraParams
    ? JSON.parse(JSON.stringify(s.extraParams))
    : {}
  const clonedAllowTools = Array.isArray(s.allowTools)
    ? [...s.allowTools]
    : s.allowTools
  editorInitial.value = {
    ...s,
    name: uniqueName,
    paused: true,
    description: s.description ?? '',
    apiKey: '',
    allowTools: clonedAllowTools,
    extraHeaders: clonedHeaders,
    extraParams: clonedParams,
  }
  if (!editorInitial.value.provider) {
    editorInitial.value.provider = providerOptions.value[0] || 'openai'
  }
  editorLockName.value = false
  editorCredentialConfigured.value = false
  editing.value = true
}
function generateUniqueName(base: string) {
  const names = new Set((data.value ?? []).map(sp => sp.name))
  if (!names.has(base)) {
    return base
  }
  let suffix = 2
  let candidate = `${base} ${suffix}`
  while (names.has(candidate)) {
    suffix += 1
    candidate = `${base} ${suffix}`
  }
  return candidate
}
function closeEditor() {
  editing.value = false
  editorInitial.value = null
  editorLockName.value = false
  editorCredentialConfigured.value = false
}

function onSaved(saved: Specialist) {
  // Clear any previous errors
  actionError.value = null
  
  // Update the editor state to reflect the saved specialist
  // This keeps the editor showing the saved specialist with updated state
  editorInitial.value = {
    ...saved,
    provider: saved.provider || providerOptions.value[0] || 'openai',
    description: saved.description ?? '',
    apiKey: '', // Never keep the secret in memory
  }
  editorLockName.value = true
  editorCredentialConfigured.value = !!saved.apiKey
}
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
</script>
