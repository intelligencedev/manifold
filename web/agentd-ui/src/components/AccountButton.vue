<template>
  <div class="relative">
    <button ref="btnRef" @click="onToggle" class="flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium hover:bg-surface-muted/60">
      <img v-if="avatar" :src="avatar" alt="avatar" class="h-6 w-6 rounded-full" />
      <span v-else class="inline-flex h-6 w-6 items-center justify-center rounded-full bg-accent/20 text-accent">U</span>
      <span class="hidden sm:inline">Account</span>
      <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 9l6 6 6-6"/></svg>
    </button>
    <Teleport to="body">
      <div v-if="open" class="fixed w-48 rounded-md border border-border/70 bg-surface py-1 shadow-lg z-50" :style="menuStyle">
        <a href="/api/me" class="block px-4 py-2 text-sm hover:bg-surface-muted/60">Profile</a>
        <!-- Use a JS navigation to force a full-page redirect so the backend can set cookies and redirect correctly -->
        <button @click="onLogout" class="w-full text-left block px-4 py-2 text-sm hover:bg-surface-muted/60">Logout</button>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, nextTick } from 'vue'

const open = ref(false)
const avatar = ''
const btnRef = ref<HTMLElement | null>(null)
const menuStyle = ref<Record<string, string>>({})

function positionMenu() {
  const btn = btnRef.value
  if (!btn) return
  const rect = btn.getBoundingClientRect()
  const top = rect.bottom + window.scrollY + 8 // 8px gap
  const left = rect.right + window.scrollX - 192 // 192px = menu width (w-48)
  menuStyle.value = {
    top: `${top}px`,
    left: `${left}px`,
  }
}

function onToggle() {
  open.value = !open.value
  if (open.value) {
    nextTick(positionMenu)
  }
}

function onDocClick(e: MouseEvent) {
  const btn = btnRef.value
  if (!btn) return
  if (open.value) {
    const target = e.target as Node
    if (!btn.contains(target)) {
      open.value = false
    }
  }
}

function onLogout() {
  // Force a full page navigation to the logout endpoint so the server can clear session and redirect
  window.location.href = '/auth/logout?next=/auth/login'
}

onMounted(() => {
  document.addEventListener('click', onDocClick, true)
  window.addEventListener('resize', positionMenu)
  window.addEventListener('scroll', positionMenu, true)
})
onBeforeUnmount(() => {
  document.removeEventListener('click', onDocClick, true)
  window.removeEventListener('resize', positionMenu)
  window.removeEventListener('scroll', positionMenu, true)
})
</script>
