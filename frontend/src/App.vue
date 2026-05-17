<template>
  <div class="min-h-screen bg-background text-foreground">
    <!-- Top chrome -->
    <header class="sticky top-0 z-40 border-b border-border bg-surface/95 backdrop-blur">
      <div class="mx-auto flex max-w-[1600px] items-center justify-between gap-6 px-6 py-0">
        <!-- Brand -->
        <div class="flex items-center gap-3 py-3.5 shrink-0">
          <span class="flex h-7 w-7 items-center justify-center rounded-lg bg-accent/20 text-accent text-xs font-bold">M</span>
          <div class="leading-none">
            <div class="text-[10px] uppercase tracking-[0.3em] text-white/40">Manifold</div>
            <div class="text-sm font-semibold">Fleet Cockpit</div>
          </div>
        </div>

        <!-- Nav -->
        <nav class="flex flex-1 items-center gap-0.5 overflow-x-auto" aria-label="Main navigation">
          <RouterLink
            v-for="item in nav"
            :key="item.to"
            :to="item.to"
            class="relative flex items-center gap-1.5 rounded-md px-3 py-2 text-sm transition-colors"
            :class="isActive(item.to)
              ? 'text-foreground bg-white/8'
              : 'text-white/50 hover:text-white/80 hover:bg-white/5'"
          >
            {{ item.label }}
            <span
              v-if="item.badge && item.badge > 0"
              class="flex h-4 min-w-4 items-center justify-center rounded-full bg-red-500/80 px-1 text-[10px] font-bold text-white"
            >{{ item.badge }}</span>
          </RouterLink>
        </nav>

        <!-- Status indicators -->
        <div class="flex shrink-0 items-center gap-3 text-xs text-white/40">
          <div class="flex items-center gap-1.5">
            <span
              class="h-2 w-2 rounded-full transition-colors"
              :class="fleet.connected ? 'bg-emerald-400 shadow-[0_0_6px_theme(colors.emerald.400)]' : 'bg-white/20'"
              aria-label="Fleet SSE connection status"
            />
            <span>{{ fleet.connected ? 'live' : 'offline' }}</span>
          </div>
          <span>{{ runCount }} runs</span>
        </div>
      </div>
    </header>

    <!-- Page -->
    <main class="mx-auto max-w-[1600px] px-6 py-6">
      <RouterView v-slot="{ Component, route }">
        <Transition name="fade" mode="out-in">
          <component :is="Component" :key="route.path" />
        </Transition>
      </RouterView>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from "vue";
import { useRoute } from "vue-router";
import { useFleetStore } from "@/stores/fleet";
import { useInboxStore } from "@/stores/inbox";

const route = useRoute();
const fleet = useFleetStore();
const inbox = useInboxStore();

const runCount = computed(() => fleet.runs.length);
const inboxCount = computed(() => inbox.requests.length);

const nav = computed(() => [
  { to: "/fleet",        label: "Fleet",        badge: 0 },
  { to: "/intent",       label: "Intent",       badge: 0 },
  { to: "/inbox",        label: "Inbox",        badge: inboxCount.value },
  { to: "/telemetry",    label: "Telemetry",    badge: 0 },
  { to: "/memory",       label: "Memory",       badge: 0 },
  { to: "/replay",       label: "Replay",       badge: 0 },
  { to: "/constitution", label: "Constitution", badge: 0 },
  { to: "/settings",     label: "Settings",     badge: 0 },
]);

function isActive(to: string) {
  return route.path.startsWith(to);
}

onMounted(() => {
  fleet.refresh().catch(() => {});
  fleet.start();
});
</script>

<style>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 120ms ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
