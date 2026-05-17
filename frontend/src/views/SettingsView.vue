<template>
  <section class="space-y-6">
    <h1 class="text-xl font-semibold">Settings</h1>

    <div class="grid gap-6 xl:grid-cols-3">
      <!-- Projects -->
      <Card title="Projects" description="Workspace-aware execution projects. Safety rules applied via existing Manifold sandboxing.">
        <template #header>
          <Button size="sm" variant="ghost" :loading="loadingProjects" @click="projects.refresh">↻</Button>
        </template>
        <div class="space-y-1.5">
          <div
            v-for="project in projects.projects"
            :key="project.id"
            class="flex items-center gap-2 rounded-lg border border-border/50 bg-black/10 px-3 py-2.5 text-xs"
          >
            <span class="text-white/80 font-medium truncate">{{ project.name || project.id }}</span>
            <span class="ml-auto font-mono text-[10px] text-white/30">{{ project.id?.slice(0, 8) }}</span>
          </div>
          <EmptyState v-if="projects.projects.length === 0" icon="📁" title="No projects" description="Create projects from the classic UI or CLI." class="py-6" />
        </div>
      </Card>

      <!-- Theme -->
      <Card title="Display theme" description="Switch between operator mood themes.">
        <div class="space-y-2">
          <button
            v-for="option in themeOptions"
            :key="option.id"
            class="flex w-full items-center gap-3 rounded-lg border px-4 py-3 text-sm transition-colors"
            :class="themeStore.theme === option.id
              ? 'border-accent/40 bg-accent/10 text-accent'
              : 'border-border/50 bg-black/10 text-white/60 hover:border-border hover:text-white/80'"
            @click="themeStore.theme = option.id"
          >
            <span class="text-lg" aria-hidden="true">{{ option.emoji }}</span>
            <div class="text-left">
              <div class="font-medium">{{ option.label }}</div>
              <div class="text-xs opacity-60">{{ option.description }}</div>
            </div>
            <span v-if="themeStore.theme === option.id" class="ml-auto text-accent">✓</span>
          </button>
        </div>
      </Card>

      <!-- Keyboard shortcuts -->
      <Card title="Keyboard shortcuts" description="Global bindings active across the cockpit.">
        <div class="space-y-3">
          <div v-for="group in shortcutGroups" :key="group.label">
            <div class="mb-1.5 text-[10px] uppercase tracking-wider text-white/30">{{ group.label }}</div>
            <div class="space-y-1">
              <div v-for="sc in group.shortcuts" :key="sc.key" class="flex items-center justify-between text-xs">
                <span class="text-white/60">{{ sc.description }}</span>
                <kbd class="rounded border border-border bg-black/20 px-1.5 py-0.5 font-mono text-[11px] text-white/50">{{ sc.key }}</kbd>
              </div>
            </div>
          </div>
        </div>
      </Card>
    </div>

    <!-- About -->
    <Card title="About Manifold Cockpit" class="text-xs text-white/40">
      <p>The cockpit is an additive operator UI for the Manifold agent platform. It runs alongside the classic UI and shares all backend APIs. No existing chat, session, or tool-execution contracts are changed.</p>
      <p class="mt-2">Build with: <code class="rounded bg-white/5 px-1">make build-cockpit</code> · Dev mode: <code class="rounded bg-white/5 px-1">make dev-cockpit</code></p>
    </Card>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import { useProjectsStore } from "@/stores/projects";
import { useThemeStore } from "@/stores/theme";

const projects = useProjectsStore();
const themeStore = useThemeStore();
const loadingProjects = ref(false);

onMounted(async () => {
  loadingProjects.value = true;
  try { await projects.refresh(); } finally { loadingProjects.value = false; }
});

const themeOptions = [
  { id: "cockpit" as const, label: "Cockpit", emoji: "🎛", description: "Dark glass, high-contrast operator interface" },
  { id: "garden" as const, label: "Garden", emoji: "🌿", description: "Softer greens for low-intensity monitoring" },
  { id: "theatre" as const, label: "Theatre", emoji: "🎭", description: "High-drama, purple-dominant debug mode" },
];

const shortcutGroups = [
  {
    label: "Inbox",
    shortcuts: [
      { key: "j", description: "Next request" },
      { key: "k", description: "Previous request" },
      { key: "a", description: "Approve / submit" },
      { key: "d", description: "Dismiss" },
    ],
  },
  {
    label: "Replay",
    shortcuts: [
      { key: "j", description: "Next event" },
      { key: "k", description: "Previous event" },
    ],
  },
  {
    label: "Intent Console",
    shortcuts: [
      { key: "⌘↵", description: "Submit run" },
    ],
  },
];
</script>
