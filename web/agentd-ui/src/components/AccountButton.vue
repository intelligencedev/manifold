<template>
  <div class="relative">
    <button
      ref="btnRef"
      @click="onToggle"
      class="flex items-center gap-2 rounded-2xl border border-border/60 bg-surface/70 px-3 py-2.5 text-sm font-semibold tracking-tight text-foreground transition hover:border-accent/40 hover:bg-surface-muted/70"
    >
      <img
        v-if="avatar"
        :src="avatar"
        alt="avatar"
        class="h-7 w-7 rounded-full"
      />
      <span
        v-else
        class="inline-flex h-7 w-7 items-center justify-center rounded-full bg-accent/15 text-accent"
        >U</span
      >
      <span class="hidden sm:inline">{{ username || "Account" }}</span>
      <svg
        class="h-4 w-4 text-subtle-foreground"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          stroke-width="2"
          d="M6 9l6 6 6-6"
        />
      </svg>
    </button>
    <Teleport to="body">
      <div
        v-if="open"
        class="fixed z-50 w-56 rounded-[20px] border border-border/70 bg-surface p-1.5 shadow-[0_18px_50px_rgba(0,0,0,0.22)]"
        :style="menuStyle"
      >
        <a
          href="/api/me"
          class="block rounded-2xl px-4 py-2.5 text-sm font-medium text-foreground transition hover:bg-surface-muted/60"
          >Profile</a
        >
        <!-- Use a JS navigation to force a full-page redirect so the backend can set cookies and redirect correctly -->
        <button
          @click="onLogout"
          class="block w-full rounded-2xl px-4 py-2.5 text-left text-sm font-medium text-foreground transition hover:bg-surface-muted/60"
        >
          Logout
        </button>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, nextTick } from "vue";

defineProps<{
  username?: string;
}>();

const open = ref(false);
const avatar = "";
const btnRef = ref<HTMLElement | null>(null);
const menuStyle = ref<Record<string, string>>({});

function positionMenu() {
  const btn = btnRef.value;
  if (!btn) return;
  const rect = btn.getBoundingClientRect();
  const top = rect.bottom + window.scrollY + 8; // 8px gap
  const left = rect.right + window.scrollX - 192; // 192px = menu width (w-48)
  menuStyle.value = {
    top: `${top}px`,
    left: `${left}px`,
  };
}

function onToggle() {
  open.value = !open.value;
  if (open.value) {
    nextTick(positionMenu);
  }
}

function onDocClick(e: MouseEvent) {
  const btn = btnRef.value;
  if (!btn) return;
  if (open.value) {
    const target = e.target as Node;
    if (!btn.contains(target)) {
      open.value = false;
    }
  }
}

function onLogout() {
  console.log("=== LOGOUT DEBUG ===");
  console.log("Logout clicked - current URL:", window.location.href);

  // Close menu
  open.value = false;

  // Clear any client-side storage that might contain auth state
  localStorage.clear();
  sessionStorage.clear();

  // IMPORTANT: Use a full navigation so the browser follows the server redirect
  // to the IdP end-session endpoint (Keycloak) and truly ends the SSO session.
  const logoutUrl = "/auth/logout?next=/auth/login&t=" + Date.now();
  console.log("Navigating (top-level) to:", logoutUrl);
  window.location.href = logoutUrl;
}

onMounted(() => {
  document.addEventListener("click", onDocClick, true);
  window.addEventListener("resize", positionMenu);
  window.addEventListener("scroll", positionMenu, true);
});
onBeforeUnmount(() => {
  document.removeEventListener("click", onDocClick, true);
  window.removeEventListener("resize", positionMenu);
  window.removeEventListener("scroll", positionMenu, true);
});
</script>
