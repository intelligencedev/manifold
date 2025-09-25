<template>
  <section class="space-y-8">
    <header class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-semibold text-foreground">Specialists</h1>
        <p class="text-sm text-subtle-foreground">Manage configured specialists used by the orchestrator.</p>
      </div>
      <button @click="startCreate" class="rounded-lg border border-border/70 px-3 py-2 text-sm font-semibold text-muted-foreground hover:border-border">New</button>
    </header>

    <div v-if="actionError" class="rounded-2xl border border-danger/60 bg-danger/10 p-3 text-danger-foreground text-sm">
      {{ actionError }}
    </div>

    <div class="rounded-2xl border border-border/70 bg-surface p-4">
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
            <td class="py-2">
              <span :class="s.paused ? 'text-warning-foreground' : 'text-success-foreground'">{{ s.paused ? 'yes' : 'no' }}</span>
            </td>
            <td class="py-2 text-right space-x-2">
              <button @click="edit(s)" class="rounded border border-border/70 px-2 py-1">Edit</button>
              <button @click="togglePause(s)" class="rounded border border-border/70 px-2 py-1">{{ s.paused ? 'Resume' : 'Pause' }}</button>
              <button @click="remove(s)" class="rounded border border-danger/60 text-danger-foreground px-2 py-1">Delete</button>
            </td>
          </tr>
          <tr v-if="loading"><td colspan="5" class="py-4 text-center text-faint-foreground">Loading…</td></tr>
          <tr v-if="error"><td colspan="5" class="py-4 text-center text-danger-foreground">Failed to load.</td></tr>
        </tbody>
      </table>
    </div>

    <div v-if="editing" class="rounded-2xl border border-border/70 bg-surface p-4 space-y-3">
      <h2 class="text-lg font-semibold">{{ form.name ? 'Edit' : 'Create' }} Specialist</h2>
      <div class="grid gap-3 md:grid-cols-2">
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
        <label class="text-sm md:col-span-2">
          <div class="text-subtle-foreground">System Prompt</div>
          <textarea v-model="form.system" rows="3" class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2"></textarea>
        </label>
      </div>
      <div class="flex gap-2">
        <button @click="save" class="rounded-lg border border-border/70 px-3 py-2 text-sm font-semibold">Save</button>
        <button @click="cancel" class="rounded-lg border border-border/70 px-3 py-2 text-sm">Cancel</button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { listSpecialists, upsertSpecialist, deleteSpecialist, type Specialist } from '@/api/client'

const qc = useQueryClient()
const { data, isLoading: loading, isError: error } = useQuery({ queryKey: ['specialists'], queryFn: listSpecialists, staleTime: 5_000 })
const specialists = computed(() => data.value ?? [])

const editing = ref(false)
const original = ref<Specialist | null>(null)
const form = ref<Specialist>({ name: '', model: '', baseURL: '', apiKey: '', enableTools: false, paused: false })
const actionError = ref<string | null>(null)

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
}
function edit(s: Specialist) {
  original.value = s
  form.value = { ...s }
  editing.value = true
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
</script>
