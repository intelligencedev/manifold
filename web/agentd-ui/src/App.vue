<template>
  <div
    :class="[
      'relative h-screen min-w-[1280px] overflow-hidden bg-background text-foreground',
      isObsDash ? 'theme-obsdash' : '',
    ]"
  >
    <div
      v-if="isObsDash"
      class="pointer-events-none absolute inset-0 opacity-90"
    ></div>
    <div
      v-if="isObsDash"
      class="pointer-events-none absolute inset-0 bg-grain mix-blend-soft-light opacity-35"
    ></div>

    <div class="relative z-10 flex h-full flex-col">
      <div class="px-1.5 pt-1.5 md:px-3 md:pt-2.5">
        <Topbar>
          <template #logo>
            <div class="flex items-center gap-3 min-w-0">
              <img
                :src="manifoldLogo"
                alt="Manifold logo"
                class="h-10 w-10 rounded object-contain"
              />
              <div class="min-w-0">
                <p class="text-base font-semibold leading-none">Manifold</p>
              </div>
            </div>
          </template>

          <template #nav>
            <RouterLink
              v-for="item in navigation"
              :key="item.to"
              :to="item.to"
              :class="navClass(item.to)"
              :aria-current="isActive(item.to) ? 'page' : undefined"
            >
              {{ item.label }}
            </RouterLink>
          </template>

          <template #actions>
            <div class="hidden items-center gap-2 sm:flex">
              <span
                class="text-[10px] font-semibold uppercase tracking-wide text-subtle-foreground"
                >Project</span
              >
              <DropdownSelect
                v-model="selectedProjectId"
                :options="projectOptions"
                size="sm"
                placeholder="Project"
                :title="selectedProjectTitle"
                aria-label="Project select"
                class="w-56 truncate"
              />
            </div>
            <div class="ml-1">
              <AccountButton :username="user?.name || user?.email" />
            </div>
          </template>
        </Topbar>
      </div>

      <div class="md:hidden px-1.5 pt-1 pb-0.5">
        <div class="flex items-center gap-2 overflow-x-auto text-sm">
          <RouterLink
            v-for="item in navigation"
            :key="item.to"
            :to="item.to"
            :class="navClass(item.to, true)"
            :aria-current="isActive(item.to) ? 'page' : undefined"
          >
            {{ item.label }}
          </RouterLink>
        </div>
      </div>

      <main
        class="relative z-10 flex min-h-0 flex-1 flex-col overflow-hidden px-1.5 pb-1.5 pt-1.5 md:px-3 md:pb-3 md:pt-2.5"
      >
        <RouterView />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { RouterLink, RouterView, useRoute } from "vue-router";
import AccountButton from "@/components/AccountButton.vue";
import DropdownSelect from "@/components/DropdownSelect.vue";
import Topbar from "@/components/ui/Topbar.vue";
import manifoldLogo from "@/assets/images/manifold_logo.png";
import { useProjectsStore } from "@/stores/projects";
import { useThemeStore } from "@/stores/theme";

const themeStore = useThemeStore();
const route = useRoute();
const isObsDash = computed(() => themeStore.resolvedThemeId === "obsdash-dark");

const user = ref<{ name?: string; email?: string; picture?: string } | null>(
  null,
);
onMounted(async () => {
  try {
    const res = await fetch("/api/me", { credentials: "include" });
    if (res.ok) user.value = await res.json();
    else {
      const g = (window as any).__MANIFOLD_USER__;
      if (g) user.value = g;
    }
  } catch (_) {
    const g = (window as any).__MANIFOLD_USER__;
    if (g) user.value = g;
  }
});

const navigation = [
  { label: "Overview", to: "/" },
  { label: "Projects", to: "/projects" },
  { label: "Specialists", to: "/specialists" },
  { label: "Chat", to: "/chat" },
  { label: "Playground", to: "/playground" },
  { label: "Flow", to: "/flow" },
  { label: "Settings", to: "/settings" },
];

function isActive(path: string) {
  return route.path === path || route.path.startsWith(`${path}/`);
}

function navClass(path: string, dense = false) {
  const base = [
    "inline-flex items-center justify-center rounded-full border transition-colors whitespace-nowrap",
    dense
      ? "px-3 py-2 text-xs"
      : "px-3 py-2 text-sm min-h-[38px] min-w-[42px] gap-2",
  ];
  if (isActive(path)) {
    base.push(
      "border-white/12 bg-surface-muted/80 text-foreground shadow-[0_12px_40px_rgba(0,0,0,0.30)]",
    );
  } else {
    base.push(
      "border-transparent text-subtle-foreground hover:border-white/10 hover:text-foreground hover:bg-surface-muted/60",
    );
  }
  return base;
}

const projectsStore = useProjectsStore();
onMounted(async () => {
  await projectsStore.refresh();
  // Restore last active project from user preferences
  await projectsStore.initFromPreferences();
});

const projectOptions = computed(() =>
  projectsStore.projects.map((p) => ({ id: p.id, label: p.name, value: p.id })),
);

const selectedProjectTitle = computed(() => {
  const id = projectsStore.currentProjectId;
  if (!id) return "Project";
  const found = projectsStore.projects.find((p) => p.id === id);
  return found?.name || "Project";
});

const selectedProjectId = computed({
  get: () => projectsStore.currentProjectId || "",
  set: (v: string) => {
    void projectsStore.setCurrent(v);
  },
});
</script>
