<template>
  <nav
    class="halo-rail halo-hairline-r flex h-full w-[88px] flex-col px-1.5 py-3"
    aria-label="Primary navigation"
  >
    <div class="mb-3 flex flex-col items-center gap-0.5 px-0 text-center">
      <img
        :src="manifoldLogo"
        alt="Manifold"
        class="h-6 w-6 rounded-md object-contain"
      />
      <div class="min-w-0 max-w-full">
        <p
          class="truncate text-[11px] font-semibold leading-tight tracking-[-0.02em] text-foreground"
        >
          Manifold
        </p>
      </div>
    </div>

    <div class="flex min-h-0 flex-1 flex-col gap-1.5">
      <RouterLink
        v-for="item in mainItems"
        :key="item.name"
        :to="navTarget(item.name)"
        :class="itemClass(item.name)"
        :aria-label="item.label"
        :aria-current="isActive(item.name) ? 'page' : undefined"
        :title="item.label"
      >
        <span class="nav-icon-wrap">
          <component
            :is="item.icon"
            v-if="item.icon"
            :class="iconClass(item.name)"
            aria-hidden="true"
          />
          <span v-else>{{ item.glyph }}</span>
        </span>
        <span class="nav-label">{{ item.label }}</span>
      </RouterLink>
    </div>

    <RouterLink
      v-if="settingsItem"
      :to="navTarget(settingsItem.name)"
      :class="[itemClass(settingsItem.name), 'mt-auto']"
      :aria-label="settingsItem.label"
      :aria-current="isActive(settingsItem.name) ? 'page' : undefined"
      :title="settingsItem.label"
    >
      <span class="nav-icon-wrap">
        <component
          :is="settingsItem.icon"
          v-if="settingsItem.icon"
          :class="iconClass(settingsItem.name)"
          aria-hidden="true"
        />
        <span v-else>{{ settingsItem.glyph }}</span>
      </span>
      <span class="nav-label">{{ settingsItem.label }}</span>
    </RouterLink>
  </nav>
</template>

<script setup lang="ts">
import { computed, type Component } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import ChatButton from "@/components/icons/ChatButton.vue";
import FlowButton from "@/components/icons/FlowButton.vue";
import OverviewButton from "@/components/icons/OverviewButton.vue";
import PlaygroundButton from "@/components/icons/PlaygroundButton.vue";
import ProjectsButton from "@/components/icons/ProjectsButton.vue";
import PulseButton from "@/components/icons/PulseButton.vue";
import Realtime from "@/components/icons/Realtime.vue";
import SettingsButton from "@/components/icons/SettingsButton.vue";
import SpecialistsButton from "@/components/icons/SpecialistsButton.vue";
import manifoldLogo from "@/assets/images/manifold_logo.png";

type NavItem = {
  name: string;
  label: string;
  glyph: string;
  icon?: Component;
  order: number;
};

const router = useRouter();
const route = useRoute();

const betaEnabled = import.meta.env.VITE_MANIFOLD_FEATURE_GATE === "beta";

const navIcons: Record<string, Component> = {
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

const navItems = computed<NavItem[]>(() =>
  router
    .getRoutes()
    .filter((record) => record.meta?.nav && typeof record.name === "string")
    .filter((record) => {
      if (record.name !== "codeqa" && record.name !== "beliefs") return true;
      return betaEnabled;
    })
    .map((record) => ({
      name: record.name as string,
      label: String(record.meta.label ?? record.name),
      glyph: String(record.meta.glyph ?? ""),
      icon: navIcons[String(record.name)],
      order: Number(record.meta.order ?? 999),
    }))
    .sort((a, b) => a.order - b.order),
);

const mainItems = computed(() =>
  navItems.value.filter((item) => item.name !== "settings"),
);

const settingsItem = computed(() =>
  navItems.value.find((item) => item.name === "settings"),
);

function isActive(name: string) {
  return route.matched.some((record) => record.name === name);
}

function navTarget(name: string) {
  if (name === "playground") return { name: "playground-prompts" };
  return { name };
}

function iconClass(name: string) {
  return ["h-4 w-4", name === "flow" ? "rotate-90" : ""];
}

function itemClass(name: string) {
  const base =
    "halo-focus relative flex min-h-[52px] w-full flex-col items-center justify-center gap-0.5 rounded-md border px-1 py-2 text-center transition-colors duration-150";
  if (isActive(name)) {
    return [
      base,
      "border-[rgb(var(--color-accent)/0.38)] bg-[rgb(var(--color-accent)/0.12)] text-foreground shadow-[inset_0_1px_0_rgb(255_255_255/0.05)]",
    ];
  }
  return [
    base,
    "border-transparent text-subtle-foreground hover:border-[rgb(var(--color-border)/0.58)] hover:bg-surface-muted/70 hover:text-foreground focus-visible:bg-input",
  ];
}
</script>

<style scoped>
.nav-icon-wrap {
  display: grid;
  height: 1.375rem;
  width: 1.375rem;
  flex: 0 0 auto;
  place-items: center;
  border-radius: 0.6rem;
  color: currentColor;
}

.nav-label {
  display: block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--font-mono);
  font-size: 0.5rem;
  font-weight: 600;
  line-height: 1;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}
</style>
