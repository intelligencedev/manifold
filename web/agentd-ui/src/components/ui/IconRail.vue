<template>
  <nav
    class="halo-rail halo-hairline-r flex h-full w-[204px] flex-col px-3 py-3"
    aria-label="Primary navigation"
  >
    <div class="mb-5 flex items-center gap-3 px-1">
      <img
        :src="manifoldLogo"
        alt="Manifold"
        class="h-8 w-8 rounded-[11px] object-contain"
      />
      <div class="min-w-0">
        <p
          class="truncate text-sm font-semibold tracking-[-0.02em] text-foreground"
        >
          Manifold
        </p>
        <p
          class="truncate font-mono text-[10px] uppercase tracking-[0.16em] text-faint-foreground"
        >
          Agent Console
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
        <span class="truncate">{{ item.label }}</span>
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
      <span class="truncate">{{ settingsItem.label }}</span>
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
  if (name === "playground") return { name: "playground-overview" };
  return { name };
}

function iconClass(name: string) {
  return ["h-[18px] w-[18px]", name === "flow" ? "rotate-90" : ""];
}

function itemClass(name: string) {
  const base =
    "halo-focus relative flex h-10 w-full items-center gap-3 rounded-[14px] border px-3 text-sm font-medium transition-colors duration-150";
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
  height: 1.75rem;
  width: 1.75rem;
  flex: 0 0 auto;
  place-items: center;
  border-radius: 0.65rem;
  color: currentColor;
}
</style>
