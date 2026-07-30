<template>
  <nav
    class="flex h-full min-h-0 flex-col border-r border-border bg-muted"
    :class="collapsed ? 'w-[52px]' : 'w-[232px]'"
    aria-label="Primary navigation"
  >
    <div
      class="flex h-[52px] shrink-0 items-center gap-2 border-b border-border px-3"
    >
      <span class="h-5 w-1 rounded-sm bg-accent" aria-hidden="true"></span>
      <img
        :src="manifoldLogo"
        alt=""
        class="h-5 w-5 rounded object-contain"
        aria-hidden="true"
      />
      <strong v-if="!collapsed" class="truncate text-[13px] font-semibold">
        Manifold
      </strong>
    </div>

    <div class="min-h-0 flex-1 overflow-y-auto p-2">
      <section
        v-for="group in navigationGroups"
        :key="group.label"
        class="mb-3"
      >
        <p
          v-if="!collapsed"
          class="mb-1 px-2 pt-1 font-mono text-[9px] font-semibold uppercase tracking-[0.15em] text-faint-foreground"
        >
          {{ group.label }}
        </p>
        <div class="space-y-0.5">
          <RouterLink
            v-for="item in group.items"
            :key="item.name"
            :to="navTarget(item.name)"
            :class="itemClass(item.name)"
            :title="collapsed ? item.label : undefined"
            :aria-label="item.label"
            :aria-current="isActive(item.name) ? 'page' : undefined"
          >
            <component
              :is="item.icon"
              v-if="item.icon"
              class="h-4 w-4 shrink-0"
              aria-hidden="true"
            />
            <span v-else class="w-4 text-center font-mono text-[8px] font-bold">
              {{ item.glyph }}
            </span>
            <span v-if="!collapsed" class="truncate">{{ item.label }}</span>
          </RouterLink>
        </div>
      </section>
    </div>

    <div class="shrink-0 border-t border-border p-2">
      <RouterLink
        v-if="settingsItem"
        :to="navTarget(settingsItem.name)"
        :class="itemClass(settingsItem.name)"
        :title="collapsed ? settingsItem.label : undefined"
        :aria-label="settingsItem.label"
      >
        <component :is="settingsItem.icon" class="h-4 w-4 shrink-0" />
        <span v-if="!collapsed">{{ settingsItem.label }}</span>
      </RouterLink>
    </div>
  </nav>
</template>

<script setup lang="ts">
import { computed, type Component } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import manifoldLogo from "@/assets/images/manifold_logo.png";
import ChatButton from "@/components/icons/ChatButton.vue";
import FlowButton from "@/components/icons/FlowButton.vue";
import OverviewButton from "@/components/icons/OverviewButton.vue";
import PlaygroundButton from "@/components/icons/PlaygroundButton.vue";
import ProjectsButton from "@/components/icons/ProjectsButton.vue";
import PulseButton from "@/components/icons/PulseButton.vue";
import Realtime from "@/components/icons/Realtime.vue";
import SettingsButton from "@/components/icons/SettingsButton.vue";
import SpecialistsButton from "@/components/icons/SpecialistsButton.vue";

defineProps<{ collapsed: boolean }>();
defineEmits<{ toggle: [] }>();

type NavItem = {
  name: string;
  label: string;
  glyph: string;
  icon?: Component;
  order: number;
};

const route = useRoute();
const router = useRouter();
const betaEnabled = import.meta.env.VITE_MANIFOLD_FEATURE_GATE === "beta";
const icons: Record<string, Component> = {
  overview: OverviewButton,
  projects: ProjectsButton,
  specialists: SpecialistsButton,
  chat: ChatButton,
  realtime: Realtime,
  pulse: PulseButton,
  playground: PlaygroundButton,
  flow: FlowButton,
  settings: SettingsButton,
};

const allItems = computed<NavItem[]>(() =>
  router
    .getRoutes()
    .filter((record) => record.meta?.nav && typeof record.name === "string")
    .filter((record) =>
      record.name === "codeqa" || record.name === "beliefs"
        ? betaEnabled
        : true,
    )
    .map((record) => ({
      name: String(record.name),
      label: String(record.meta.label ?? record.name),
      glyph: String(record.meta.glyph ?? ""),
      icon: icons[String(record.name)],
      order: Number(record.meta.order ?? 999),
    }))
    .sort((a, b) => a.order - b.order),
);

const namesByGroup = [
  { label: "Work", names: ["chat", "realtime", "projects"] },
  { label: "Build", names: ["specialists", "playground", "flow"] },
  {
    label: "Operate",
    names: ["overview", "pulse", "durable", "codeqa", "beliefs"],
  },
];

const navigationGroups = computed(() =>
  namesByGroup
    .map((group) => ({
      label: group.label,
      items: group.names
        .map((name) => allItems.value.find((item) => item.name === name))
        .filter((item): item is NavItem => Boolean(item)),
    }))
    .filter((group) => group.items.length),
);

const settingsItem = computed(() =>
  allItems.value.find((item) => item.name === "settings"),
);

function isActive(name: string) {
  if (name === "playground") {
    return route.matched.some((record) => record.name === "playground");
  }
  return route.matched.some((record) => record.name === name);
}

function navTarget(name: string) {
  return name === "playground" ? { name: "playground-prompts" } : { name };
}

function itemClass(name: string) {
  const base =
    "halo-focus flex h-8 items-center gap-2 rounded border px-2 text-[12px] font-medium transition-colors";
  const width = collapsed ? "justify-center" : "w-full";
  return isActive(name)
    ? [base, width, "border-accent/35 bg-accent/10 text-foreground"]
    : [
        base,
        width,
        "border-transparent text-subtle-foreground hover:bg-surface-muted hover:text-foreground",
      ];
}
</script>
