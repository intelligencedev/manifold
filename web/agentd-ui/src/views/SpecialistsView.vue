<template>
  <section class="flex flex-col h-full min-h-0">
    <div v-if="actionError" class="rounded-2xl border border-danger/60 bg-danger/10 p-3 text-danger-foreground text-sm">
      {{ actionError }}
    </div>

    <!-- list/edit layout; nested areas scroll but view itself doesn't -->
    <div class="flex flex-col xl:flex-row gap-4 flex-1 min-h-0">
      <!-- left: card grid -->
      <div class="ap-panel ap-hover xl:w-1/2 min-w-0 rounded-md bg-surface p-4 min-h-0 overflow-auto">
        <div class="flex items-center justify-between mb-4">
          <h2 class="text-base font-semibold">Specialist Assistants</h2>
          <button @click="startCreate" class="rounded-md border border-border/60 px-3 py-1.5 text-xs font-semibold text-muted-foreground hover:border-border">
            New
          </button>
        </div>

        <div v-if="loading" class="rounded-md border border-border/60 bg-surface-muted/20 p-4 text-sm text-faint-foreground">Loading…</div>
        <div v-else-if="error" class="rounded-md border border-danger/60 bg-danger/10 p-4 text-sm text-danger-foreground">Failed to load specialists.</div>
        <div v-else-if="!specialists.length" class="rounded-md border border-border/60 bg-surface-muted/20 p-4 text-sm text-faint-foreground">No specialists configured yet.</div>
        <div v-else class="grid gap-4 sm:grid-cols-1 lg:grid-cols-2">
          <article
            v-for="s in specialists"
            :key="s.name"
            class="ap-card flex flex-col rounded-2xl bg-surface p-5"
          >
            <div class="flex items-start justify-between gap-3">
              <div>
                <h3 class="text-base font-semibold text-foreground">{{ s.name }}</h3>
                <p class="mt-1 text-xs uppercase tracking-wide text-subtle-foreground">Model</p>
                <p class="text-sm text-muted-foreground">{{ s.model || '—' }}</p>
              </div>
              <span :class="statusBadgeClass(s.paused)">{{ s.paused ? 'Paused' : 'Active' }}</span>
            </div>

            <p class="mt-4 text-sm leading-relaxed text-subtle-foreground line-clamp-4">{{ specialistDescription(s) }}</p>

            <div class="mt-4 flex flex-wrap items-center gap-2 text-xs">
              <span :class="toolsBadgeClass(s.enableTools)">{{ s.enableTools ? 'Tools enabled' : 'Tools disabled' }}</span>
              <span
                v-if="Array.isArray(s.allowTools) && s.allowTools.length > 0"
                class="inline-flex items-center rounded-full border border-border/50 bg-surface-muted/30 px-2 py-1 font-medium text-subtle-foreground"
              >
                Allow list · {{ s.allowTools.length }}
              </span>
              <span
                v-if="s.reasoningEffort"
                class="inline-flex items-center rounded-full border border-info/40 bg-info/10 px-2 py-1 font-medium text-info"
              >
                Reasoning: {{ s.reasoningEffort }}
              </span>
            </div>

            <div class="mt-4 flex flex-wrap gap-2">
              <button
                type="button"
                @click="edit(s)"
                class="rounded border border-border/60 px-3 py-1.5 text-xs font-semibold text-subtle-foreground hover:border-border"
              >
                Edit
              </button>
              <button
                type="button"
                @click="togglePause(s)"
                class="inline-flex items-center gap-1 rounded border border-border/60 px-3 py-1.5 text-xs font-semibold text-subtle-foreground hover:border-border"
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
                class="rounded border border-danger/60 px-3 py-1.5 text-xs font-semibold text-danger/80 hover:border-danger"
              >
                Delete
              </button>
            </div>
          </article>
        </div>
      </div>

      <!-- right: editor -->
      <div class="xl:w-1/2 min-w-0 min-h-0">
        <div v-if="editing" class="ap-panel ap-hover rounded-md bg-surface p-3 h-full min-h-0 overflow-auto flex flex-col">
          <div class="flex flex-col gap-4 h-full min-h-0">
            <h2 class="text-base font-semibold">{{ form.name ? 'Edit' : 'Create' }} Specialist</h2>
            <!-- Two-column layout: left (params), right (system prompt + tools). Implemented with explicit grid -->
            <div class="flex-1 min-h-0 grid items-stretch grid-cols-1 md:grid-cols-6 lg:grid-cols-10 xl:grid-cols-12 gap-3">
              <!-- Left column: core settings -->
              <div class="flex flex-col gap-2 min-h-0 h-full p-0 md:col-span-2 lg:col-span-3 xl:col-span-3 lg:sticky lg:top-3 lg:self-start">
                <div class="flex flex-col gap-1">
                  <label for="specialist-name" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Name</label>
                  <input id="specialist-name" v-model="form.name" class="w-full rounded border border-border/60 bg-surface-muted/40 px-2 py-1.5 text-sm" :disabled="!!original?.name" />
                </div>
                <div class="flex flex-col gap-1">
                  <label for="specialist-description" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Description</label>
                  <textarea id="specialist-description" v-model="form.description" rows="3" class="w-full rounded border border-border/60 bg-surface-muted/40 px-2 py-1.5 text-sm"></textarea>
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
                <div class="flex flex-col gap-1 min-h-0 flex-1">
                  <label for="specialist-extra-params" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Extra Params (JSON)</label>
                  <textarea id="specialist-extra-params" v-model="extraParamsRaw" class="flex-1 min-h-0 w-full rounded border border-border/60 bg-surface-muted/40 px-2 py-1.5 text-sm resize-none"></textarea>
                </div>
              </div>
              
              <!-- Right column: system prompt -->
              <div class="flex min-h-0 flex-col gap-2 md:col-span-4 lg:col-span-7 xl:col-span-9 overflow-auto">
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

                <!-- Tool Access: now part of the right column, below system prompt -->
                <section class="mt-3 rounded-md border border-border/50 bg-surface p-3 shrink-0">
                  <div class="flex items-start justify-between gap-3">
                    <div>
                      <p class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Tool Access</p>
                      <p class="text-sm text-muted-foreground">{{ toolAccessDescription }}</p>
                    </div>
                    <button
                      type="button"
                      class="inline-flex items-center rounded border border-border/60 bg-surface-muted px-3 py-1 text-xs font-semibold text-subtle-foreground hover:border-border disabled:cursor-not-allowed disabled:opacity-50"
                      @click="openToolsModal"
                      :disabled="toolsLoading && !tools.length"
                    >
                      Manage tools
                    </button>
                  </div>

                  <!-- Horizontal card layout for tool modes -->
                  <div class="mt-3 grid grid-cols-1 md:grid-cols-3 gap-2 text-sm items-stretch">
                    <label
                      class="flex cursor-pointer items-start gap-3 rounded-lg border px-3 py-3 transition-colors h-full"
                      :class="toolAccessMode === 'disabled' ? 'border-border/80 bg-surface-muted/60' : 'border-border/50 hover:border-border'"
                    >
                      <input
                        class="mt-1 h-4 w-4 shrink-0"
                        type="radio"
                        name="tools-mode"
                        value="disabled"
                        v-model="toolAccessMode"
                      />
                      <div class="min-w-0">
                        <p class="font-medium text-foreground">Disable tools</p>
                        <p class="text-xs text-subtle-foreground">Specialist will never call tools.</p>
                      </div>
                    </label>
                    <label
                      class="flex cursor-pointer items-start gap-3 rounded-lg border px-3 py-3 transition-colors h-full"
                      :class="toolAccessMode === 'all' ? 'border-border/80 bg-surface-muted/60' : 'border-border/50 hover:border-border'"
                    >
                      <input
                        class="mt-1 h-4 w-4 shrink-0"
                        type="radio"
                        name="tools-mode"
                        value="all"
                        v-model="toolAccessMode"
                      />
                      <div class="min-w-0">
                        <p class="font-medium text-foreground">Allow any tool</p>
                        <p class="text-xs text-subtle-foreground">Every available tool can be invoked.</p>
                      </div>
                    </label>
                    <label
                      class="flex cursor-pointer items-start gap-3 rounded-lg border px-3 py-3 transition-colors h-full"
                      :class="toolAccessMode === 'custom' ? 'border-border/80 bg-surface-muted/60' : 'border-border/50 hover:border-border'"
                    >
                      <input
                        class="mt-1 h-4 w-4 shrink-0"
                        type="radio"
                        name="tools-mode"
                        value="custom"
                        v-model="toolAccessMode"
                      />
                      <div class="min-w-0">
                        <p class="font-medium text-foreground">Use an allow list</p>
                        <p class="text-xs text-subtle-foreground">Only selected tools will be enabled.</p>
                      </div>
                    </label>
                  </div>

                  <div v-if="toolAccessMode === 'custom'" class="mt-3 rounded-md border border-dashed border-border/60 bg-surface-muted/40 p-3">
                    <div class="flex items-center justify-between text-xs font-semibold uppercase tracking-wide text-subtle-foreground">
                      <span>Selected tools</span>
                      <span>{{ selectedToolNames.length }} total</span>
                    </div>
                    <div v-if="selectedToolNames.length" class="mt-2 flex flex-wrap gap-1.5">
                      <button
                        v-for="name in selectedToolPreview"
                        :key="name"
                        type="button"
                        class="group inline-flex items-center gap-1 rounded-full border border-border/60 bg-surface px-2 py-0.5 text-xs text-foreground hover:border-border"
                        @click="removeAllowedTool(name)"
                        :title="`Remove ${name}`"
                      >
                        <span>{{ name }}</span>
                        <svg viewBox="0 0 12 12" class="h-3 w-3 text-subtle-foreground group-hover:text-foreground" aria-hidden="true">
                          <path d="M3 3l6 6m0-6-6 6" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
                        </svg>
                      </button>
                      <span
                        v-if="toolChipOverflow"
                        class="inline-flex items-center rounded-full border border-border/40 bg-surface px-2 py-0.5 text-xs text-subtle-foreground"
                      >
                        +{{ toolChipOverflow }} more
                      </span>
                    </div>
                    <p v-else class="mt-2 text-xs text-subtle-foreground">No tools selected yet.</p>
                    <div class="mt-3 flex flex-wrap gap-2 text-xs">
                      <button
                        type="button"
                        class="rounded border border-border/60 px-2 py-1 text-subtle-foreground hover:border-border disabled:cursor-not-allowed disabled:opacity-40"
                        @click="clearToolSelection"
                        :disabled="!selectedToolNames.length"
                      >
                        Clear selection
                      </button>
                    </div>
                  </div>

                  <p v-if="toolsLoading" class="mt-3 text-xs text-subtle-foreground">Loading tools…</p>
                  <p v-else-if="toolsError" class="mt-3 text-xs text-danger-foreground">{{ toolsError }}</p>
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

    <!-- Tools modal -->
    <div v-if="showToolsModal" class="fixed inset-0 z-50 flex items-center justify-center px-4 py-8">
      <div class="absolute inset-0 bg-surface/70 backdrop-blur-sm" @click="closeToolsModal"></div>
      <div class="relative z-10 flex w-full max-w-4xl flex-col overflow-hidden rounded-xl border border-border/70 bg-surface shadow-2xl">
        <div class="flex items-center justify-between border-b border-border/60 px-5 py-4">
          <div>
            <h3 class="text-base font-semibold text-foreground">Manage tools</h3>
            <p class="text-xs text-subtle-foreground">{{ toolsSummaryLabel }}</p>
          </div>
          <button
            type="button"
            class="rounded border border-border/60 bg-surface-muted px-3 py-1 text-xs font-semibold text-subtle-foreground hover:border-border"
            @click="closeToolsModal"
          >
            Close
          </button>
        </div>
        <div class="flex flex-col gap-4 px-5 py-4">
          <div class="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
            <div class="flex-1">
              <label for="tools-search" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Search</label>
              <input
                id="tools-search"
                v-model="toolsSearch"
                type="text"
                placeholder="Search by name or description"
                class="mt-1 w-full rounded border border-border/60 bg-surface-muted/40 px-3 py-2 text-sm text-foreground"
              />
            </div>
            <div class="flex items-center gap-2 text-xs">
              <button
                type="button"
                class="rounded border border-border/60 bg-surface px-3 py-1 font-semibold text-subtle-foreground hover:border-border disabled:cursor-not-allowed disabled:opacity-40"
                :disabled="!tools.length"
                @click="selectAllTools"
              >
                Select all
              </button>
              <button
                type="button"
                class="rounded border border-border/60 bg-surface px-3 py-1 text-subtle-foreground hover:border-border disabled:cursor-not-allowed disabled:opacity-40"
                :disabled="!selectedToolNames.length"
                @click="clearToolSelection"
              >
                Clear
              </button>
            </div>
          </div>
          <div class="min-h-[280px] max-h-[60vh] overflow-y-auto rounded border border-border/60 bg-surface-muted/40">
            <div v-if="toolsLoading" class="flex h-full items-center justify-center text-sm text-subtle-foreground">Loading tools…</div>
            <div v-else-if="toolsError" class="flex h-full items-center justify-center px-4 text-center text-sm text-danger-foreground">{{ toolsError }}</div>
            <div v-else-if="!tools.length" class="flex h-full items-center justify-center px-4 text-center text-sm text-subtle-foreground">No tools available.</div>
            <div v-else-if="!filteredTools.length" class="flex h-full items-center justify-center px-4 text-center text-sm text-subtle-foreground">No tools match "{{ toolsSearch }}".</div>
            <ul v-else class="divide-y divide-border/40">
              <li v-for="t in filteredTools" :key="t.name">
                <label class="flex cursor-pointer items-start gap-3 px-4 py-3 hover:bg-surface">
                  <input
                    type="checkbox"
                    class="mt-1 h-4 w-4"
                    :checked="selectedToolSet.has(t.name)"
                    @change="onToolCheckboxChange(t.name, $event)"
                  />
                  <div class="min-w-0">
                    <p class="text-sm font-medium text-foreground break-words">{{ t.name }}</p>
                    <p class="text-xs text-subtle-foreground break-words">{{ t.description || 'No description provided.' }}</p>
                  </div>
                </label>
              </li>
            </ul>
          </div>
        </div>
        <div class="flex items-center justify-between border-t border-border/60 px-5 py-3 text-xs text-subtle-foreground">
          <span>{{ selectedToolNames.length }} {{ selectedToolNames.length === 1 ? 'tool' : 'tools' }} selected</span>
          <button
            type="button"
            class="rounded border border-border/60 bg-surface-muted px-3 py-1 text-xs font-semibold text-subtle-foreground hover:border-border"
            @click="closeToolsModal"
          >
            Done
          </button>
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

const TOOL_PREVIEW_LIMIT = 8

const qc = useQueryClient()
const { data, isLoading: loading, isError: error } = useQuery({ queryKey: ['specialists'], queryFn: listSpecialists, staleTime: 5_000 })
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

const editing = ref(false)
const original = ref<Specialist | null>(null)
const form = ref<Specialist>({ name: '', description: '', model: '', baseURL: '', apiKey: '', enableTools: false, paused: false, system: '', allowTools: [], reasoningEffort: '', extraHeaders: {}, extraParams: {} })
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
const showToolsModal = ref(false)
const toolsSearch = ref('')

const selectedToolNames = computed(() => (Array.isArray(form.value.allowTools) ? form.value.allowTools : []))
const selectedToolSet = computed(() => new Set(selectedToolNames.value))
const selectedToolPreview = computed(() => selectedToolNames.value.slice(0, TOOL_PREVIEW_LIMIT))
const toolChipOverflow = computed(() => Math.max(0, selectedToolNames.value.length - TOOL_PREVIEW_LIMIT))
type ToolAccessMode = 'disabled' | 'all' | 'custom'
const toolAccessMode = computed<ToolAccessMode>({
  get() {
    if (!form.value.enableTools) {
      return 'disabled'
    }
    return selectedToolNames.value.length > 0 ? 'custom' : 'all'
  },
  set(mode) {
    if (mode === 'disabled') {
      form.value.enableTools = false
      form.value.allowTools = []
      return
    }
    form.value.enableTools = true
    if (mode === 'all') {
      form.value.allowTools = []
      return
    }
    if (!Array.isArray(form.value.allowTools)) {
      form.value.allowTools = []
    }
  },
})
const toolAccessDescription = computed(() => {
  switch (toolAccessMode.value) {
    case 'disabled':
      return 'Specialist will never call tools.'
    case 'all':
      return 'Every available tool can be invoked.'
    case 'custom':
      return 'Only the tools you select will be enabled.'
    default:
      return ''
  }
})
const filteredTools = computed(() => {
  const query = toolsSearch.value.trim().toLowerCase()
  if (!query) {
    return tools.value
  }
  return tools.value.filter(tool => {
    const desc = (tool.description || '').toLowerCase()
    const name = tool.name.toLowerCase()
    return name.includes(query) || desc.includes(query)
  })
})
const toolsSummaryLabel = computed(() => {
  if (toolsLoading.value) return 'Loading available tools…'
  if (toolsError.value) return toolsError.value || 'Unable to load tools'
  if (!tools.value.length) return 'No tools available'
  return `${tools.value.length} available`
})

async function loadTools() {
  if (toolsLoading.value) return
  toolsLoading.value = true
  toolsError.value = ''
  try {
    const resp = await fetchWarppTools().catch(() => [] as WarppTool[])
    tools.value = resp
      .filter(t => !!t?.name)
      .sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }))
  } catch (err: any) {
    toolsError.value = err?.message ?? 'Failed to load tools'
  } finally {
    toolsLoading.value = false
  }
}

function openToolsModal() {
  if (!showToolsModal.value) {
    toolsSearch.value = ''
  }
  showToolsModal.value = true
  void loadTools()
}

function closeToolsModal() {
  showToolsModal.value = false
}

function setToolSelection(name: string, enabled: boolean) {
  const set = new Set(selectedToolNames.value)
  if (enabled) set.add(name)
  else set.delete(name)
  form.value.allowTools = Array.from(set)
  toolAccessMode.value = 'custom'
}

function removeAllowedTool(name: string) {
  setToolSelection(name, false)
}

function clearToolSelection() {
  form.value.allowTools = []
}

function selectAllTools() {
  if (!tools.value.length) return
  form.value.allowTools = tools.value.map(t => t.name)
  toolAccessMode.value = 'custom'
}

function onToolCheckboxChange(name: string, event: Event) {
  const target = event.target as HTMLInputElement | null
  setToolSelection(name, !!target?.checked)
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
  original.value = null
  form.value = { name: '', description: '', model: '', baseURL: '', apiKey: '', enableTools: false, paused: false, system: '', allowTools: [], reasoningEffort: '', extraHeaders: {}, extraParams: {} }
  extraHeadersRaw.value = ''
  extraParamsRaw.value = ''
  editing.value = true
  void ensurePromptsLoaded()
  void loadTools()
}
function edit(s: Specialist) {
  original.value = s
  form.value = { ...s, description: s.description ?? '' }
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
