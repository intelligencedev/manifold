<template>
  <div class="space-y-6" v-if="prompt">
    <section class="rounded-2xl border border-border/70 bg-surface p-4 space-y-2">
      <div class="flex items-center justify-between">
        <div>
          <h2 class="text-xl font-semibold">{{ prompt.name }}</h2>
          <p class="text-sm text-subtle-foreground">{{ prompt.description || 'No description provided.' }}</p>
        </div>
        <RouterLink to="/playground/prompts" class="text-sm text-accent hover:underline">Back to prompts</RouterLink>
      </div>
      <div class="text-sm text-subtle-foreground">Tags: {{ prompt.tags?.join(', ') || '—' }}</div>
      <div class="text-sm text-subtle-foreground">Created: {{ formatDate(prompt.createdAt) }}</div>
    </section>

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

    <section class="rounded-2xl border border-border/70 bg-surface p-4 space-y-3">
      <header>
        <h3 class="text-lg font-semibold">Versions</h3>
        <p class="text-sm text-subtle-foreground">Most recent versions first.</p>
      </header>
      <table class="w-full text-sm">
        <thead class="text-subtle-foreground">
          <tr>
            <th class="text-left py-2">Version</th>
            <th class="text-left py-2">Created</th>
            <th class="text-left py-2">Hash</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="version in versions" :key="version.id" class="border-t border-border/60">
            <td class="py-2 font-medium">{{ version.semver || version.id }}</td>
            <td class="py-2 text-subtle-foreground">{{ formatDate(version.createdAt) }}</td>
            <td class="py-2 text-xs font-mono text-subtle-foreground break-all">{{ version.contentHash || '—' }}</td>
          </tr>
          <tr v-if="versionsLoading"><td colspan="3" class="py-3 text-center text-subtle-foreground">Loading…</td></tr>
          <tr v-else-if="versions.length === 0"><td colspan="3" class="py-3 text-center text-subtle-foreground">No versions yet.</td></tr>
        </tbody>
      </table>
    </section>
  </div>
  <p v-else class="text-subtle-foreground text-sm">Loading prompt…</p>
</template>

<script setup lang="ts">
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { onMounted, reactive, ref, watch } from 'vue'
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

async function refreshVersions(id: string) {
  versionsLoading.value = true
  await store.loadPromptVersions(id)
  versions.value = store.promptVersions[id] ?? []
  versionsLoading.value = false
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
  await refreshVersions(promptId.value)
  setTimeout(() => (createMessage.value = ''), 3000)
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
</script>
