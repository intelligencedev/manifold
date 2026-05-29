<template>
  <nav
    class="halo-rail halo-hairline-r flex h-full w-16 flex-col items-center gap-1.5 px-0 py-3.5"
    aria-label="Primary navigation"
  >
    <img
      :src="manifoldLogo"
      alt="Manifold"
      class="mb-3 h-[26px] w-[26px] rounded-[7px] object-contain"
    />

    <RouterLink
      v-for="item in mainItems"
      :key="item.name"
      :to="navTarget(item.name)"
      :class="itemClass(item.name)"
      :aria-label="item.label"
      :aria-current="isActive(item.name) ? 'page' : undefined"
      :title="item.label"
    >
      <component
        :is="item.icon"
        v-if="item.icon"
        :class="iconClass(item.name)"
        aria-hidden="true"
      />
      <span v-else>{{ item.glyph }}</span>
    </RouterLink>

    <RouterLink
      v-if="settingsItem"
      :to="navTarget(settingsItem.name)"
      :class="[itemClass(settingsItem.name), 'mt-auto']"
      :aria-label="settingsItem.label"
      :aria-current="isActive(settingsItem.name) ? 'page' : undefined"
      :title="settingsItem.label"
    >
      <component
        :is="settingsItem.icon"
        v-if="settingsItem.icon"
        :class="iconClass(settingsItem.name)"
        aria-hidden="true"
      />
      <span v-else>{{ settingsItem.glyph }}</span>
    </RouterLink>
  </nav>
</template>

<script setup lang="ts">
import { computed, type Component } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import ChatButton from "@/components/icons/ChatButton.vue";
import DurableButton from "@/components/icons/DurableButton.vue";
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
  durable: DurableButton,
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
  return ["h-5 w-5", name === "flow" ? "rotate-90" : ""];
}

function itemClass(name: string) {
  const base =
    "halo-focus relative grid h-[38px] w-[38px] place-items-center rounded-md border border-transparent font-mono text-[11px] text-faint-foreground transition-colors duration-150";
  if (isActive(name)) {
    return [
      base,
      "bg-input text-[rgb(var(--accent-hi))] shadow-[inset_0_0_0_1px_rgb(var(--line-strong))]",
    ];
  }
  return [
    base,
    "hover:bg-surface-muted hover:text-foreground focus-visible:bg-input",
  ];
}
</script>
