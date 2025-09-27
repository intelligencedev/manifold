<template>
  <section class="space-y-6 flex-1 min-h-0 overflow-auto">
    <header class="space-y-2">
      <h1 class="text-2xl font-semibold text-foreground">Settings</h1>
      <p class="text-sm text-subtle-foreground">
        Configure integrations, authentication, and advanced execution knobs for agentd.
      </p>
    </header>

    <div class="grid gap-6 lg:grid-cols-2">
      <!-- Application (client-side) settings -->
      <form class="space-y-4 rounded-2xl border border-border/70 bg-surface p-6 lg:col-span-1">
        <header class="space-y-1">
          <h2 class="text-lg font-semibold text-foreground">Application</h2>
          <p class="text-sm text-subtle-foreground">Client-side settings stored in your browser.</p>
        </header>

        <div class="grid gap-2 md:grid-cols-5 md:items-start">
          <div class="md:col-span-2">
            <label class="text-sm font-medium text-muted-foreground" for="api-url">API Base URL</label>
            <p class="text-xs text-faint-foreground">Used during local development when the Go backend is proxied.</p>
          </div>
          <div class="md:col-span-3">
            <input
              id="api-url"
              v-model="apiUrl"
              type="url"
              placeholder="https://localhost:32180/api"
              class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
            />
          </div>
        </div>

        <div class="flex items-center justify-between border-t border-border/60 pt-4">
          <p class="text-sm text-subtle-foreground">
            Changes are stored locally and applied on next reload.
          </p>
          <div class="flex gap-3">
            <button
              type="button"
              class="rounded-lg border border-border/70 px-3 py-2 text-sm font-semibold text-muted-foreground transition hover:border-border"
              @click="resetToDefaults"
            >
              Reset
            </button>
            <button
              type="button"
              class="rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-foreground transition hover:bg-accent/90"
              @click="persist"
            >
              Save
            </button>
          </div>
        </div>
      </form>

      <section class="space-y-4 rounded-2xl border border-border/70 bg-surface p-6 lg:col-span-1">
        <header class="space-y-1">
          <h2 class="text-lg font-semibold text-foreground">Appearance</h2>
          <p class="text-sm text-subtle-foreground">
            Swap themes or follow your operating system. Changes apply instantly.
          </p>
        </header>
        <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          <button
            v-for="option in themeOptions"
            :key="option.id"
            type="button"
            :class="[
              'flex flex-col rounded-xl border px-4 py-3 text-left shadow-sm transition',
              option.id === themeSelection
                ? 'border-accent bg-accent/10'
                : 'border-border/60 bg-surface-muted/40 hover:border-border/80 hover:bg-surface-muted/70',
            ]"
            @click="selectTheme(option.id)"
          >
            <span class="text-sm font-semibold text-foreground">{{ option.label }}</span>
            <span class="text-xs text-subtle-foreground">{{ option.description }}</span>
            <span class="text-[10px] uppercase tracking-wide text-faint-foreground">
              {{ option.id === 'system' ? 'auto' : option.appearance }}
            </span>
          </button>
        </div>
      </section>

      <section v-if="isAdmin" class="space-y-4 rounded-2xl border border-border/70 bg-surface p-6 lg:col-span-2">
        <header class="space-y-1">
          <h2 class="text-lg font-semibold text-foreground">Users</h2>
          <p class="text-sm text-subtle-foreground">Create, modify, and delete users. Admin only.</p>
        </header>
        <div class="flex items-center justify-between">
          <button type="button" class="rounded-lg bg-accent px-3 py-2 text-sm font-semibold text-accent-foreground hover:bg-accent/90" @click="startCreate">New user</button>
          <div class="text-xs text-faint-foreground">Total: {{ users.length }}</div>
        </div>
        <div class="grid gap-4 lg:grid-cols-3">
          <div class="overflow-x-auto lg:col-span-2 min-w-0">
            <table class="w-full table-fixed border-collapse text-left text-sm">
              <thead>
                <tr class="border-b border-border/60 text-muted-foreground">
                  <th class="py-2 pr-2">Email</th>
                  <th class="py-2 pr-2">Name</th>
                  <th class="py-2 pr-2">Roles</th>
                  <th class="py-2 pr-2">Provider</th>
                  <th class="py-2 pr-2"></th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="u in users" :key="u.id" class="border-b border-border/40">
                  <td class="py-2 pr-2 truncate">{{ u.email }}</td>
                  <td class="py-2 pr-2 truncate">{{ u.name }}</td>
                  <td class="py-2 pr-2 truncate">{{ u.roles?.join(', ') }}</td>
                  <td class="py-2 pr-2 truncate">{{ u.provider }}</td>
                  <td class="py-2 flex gap-2">
                    <button class="rounded border border-border/70 px-2 py-1 text-xs hover:border-border" @click="edit(u)">Edit</button>
                    <button class="rounded border border-border/70 px-2 py-1 text-xs text-danger hover:border-border" @click="remove(u)">Delete</button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-if="editing" class="rounded-xl border border-border/60 bg-surface-muted/40 p-4 lg:col-span-1 lg:sticky lg:top-6 self-start">
            <h3 class="mb-2 text-sm font-semibold text-foreground">{{ form.id ? 'Edit user' : 'New user' }}</h3>
            <div class="grid gap-3 md:grid-cols-2">
              <div>
                <label class="text-xs text-muted-foreground">Email</label>
                <input v-model="form.email" type="email" class="mt-1 w-full rounded border border-border/70 bg-surface px-2 py-1 text-sm" />
              </div>
              <div>
                <label class="text-xs text-muted-foreground">Name</label>
                <input v-model="form.name" type="text" class="mt-1 w-full rounded border border-border/70 bg-surface px-2 py-1 text-sm" />
              </div>
              <div>
                <label class="text-xs text-muted-foreground">Roles (comma-separated)</label>
                <input v-model="rolesInput" type="text" placeholder="admin, user" class="mt-1 w-full rounded border border-border/70 bg-surface px-2 py-1 text-sm" />
              </div>
              <div>
                <label class="text-xs text-muted-foreground">Provider</label>
                <input v-model="form.provider" type="text" placeholder="oidc" class="mt-1 w-full rounded border border-border/70 bg-surface px-2 py-1 text-sm" />
              </div>
              <div class="md:col-span-2">
                <label class="text-xs text-muted-foreground">Subject</label>
                <input v-model="form.subject" type="text" class="mt-1 w-full rounded border border-border/70 bg-surface px-2 py-1 text-sm" />
              </div>
            </div>
            <div class="mt-3 flex gap-2">
              <button class="rounded bg-accent px-3 py-1 text-xs font-semibold text-accent-foreground hover:bg-accent/90" @click="save">Save</button>
              <button class="rounded border border-border/70 px-3 py-1 text-xs hover:border-border" @click="cancel">Cancel</button>
              <span class="text-xs text-faint-foreground" v-if="errorMsg">{{ errorMsg }}</span>
            </div>
          </div>
        </div>
      </section>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useThemeStore } from '@/stores/theme'
import type { ThemeChoice } from '@/theme/themes'
import { listUsers, createUser, updateUser, deleteUser } from '@/api/client'

const apiUrl = ref('')

const STORAGE_KEY = 'agentd.ui.settings'

type Settings = {
  apiUrl: string
}

const themeStore = useThemeStore()
const themeOptions = computed(() => themeStore.options)
const themeSelection = computed(() => themeStore.selection)

// Admin-only Users section state
type User = {
  id: number
  email: string
  name: string
  provider?: string
  subject?: string
  roles: string[]
}
const users = ref<User[]>([])
const isAdmin = ref(false)
const editing = ref(false)
const form = ref<Partial<User>>({})
const rolesInput = ref('')
const errorMsg = ref('')

onMounted(() => {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored) {
      const parsed = JSON.parse(stored) as Settings
      apiUrl.value = parsed.apiUrl
    }
  } catch (error) {
    console.warn('Unable to parse stored settings', error)
  }
  // Determine admin by probing a protected admin endpoint indirectly: list users.
  // If it succeeds, current user is authenticated and has access (auth middleware already enforces auth).
  refreshUsers()
})

function persist() {
  const payload: Settings = {
    apiUrl: apiUrl.value,
  }
  localStorage.setItem(STORAGE_KEY, JSON.stringify(payload))
}

function resetToDefaults() {
  localStorage.removeItem(STORAGE_KEY)
  apiUrl.value = ''
}

function selectTheme(choice: ThemeChoice) {
  themeStore.setTheme(choice)
}

async function refreshUsers() {
  try {
    const data = await listUsers()
    users.value = data
    isAdmin.value = true // if call succeeds, treat as admin-capable page
  } catch (e) {
    // Not admin or not authenticated; keep hidden
    isAdmin.value = false
  }
}

function startCreate() {
  form.value = { id: 0, email: '', name: '', provider: 'oidc', subject: '', roles: ['user'] }
  rolesInput.value = (form.value.roles || []).join(', ')
  errorMsg.value = ''
  editing.value = true
}

function edit(u: User) {
  form.value = { ...u }
  rolesInput.value = (u.roles || []).join(', ')
  errorMsg.value = ''
  editing.value = true
}

async function save() {
  if (!form.value) return
  const payload: any = {
    email: form.value.email,
    name: form.value.name,
    provider: form.value.provider,
    subject: form.value.subject,
    roles: rolesInput.value.split(',').map((s) => s.trim()).filter(Boolean),
  }
  try {
    if (!form.value.id || form.value.id === 0) {
      await createUser(payload)
    } else {
      await updateUser(form.value.id, payload)
    }
    editing.value = false
    await refreshUsers()
  } catch (e: any) {
    errorMsg.value = e?.response?.data || 'Save failed'
  }
}

function cancel() {
  editing.value = false
}

async function remove(u: User) {
  if (!confirm(`Delete user ${u.email}?`)) return
  try {
    await deleteUser(u.id)
    await refreshUsers()
  } catch (e) {
    // ignore
  }
}
</script>
