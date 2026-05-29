<template>
  <nav
    class="halo-rail halo-hairline-r flex h-full w-16 flex-col items-center gap-1.5 px-0 py-3.5"
    aria-label="Primary navigation"
  >
    <div
      class="mb-3 h-[26px] w-[26px] rounded-[7px]"
      :style="{
        background:
          'conic-gradient(from 210deg, rgb(var(--color-accent)), rgb(var(--data)))',
      }"
      aria-hidden="true"
    ></div>

    <RouterLink
      v-for="item in mainItems"
      :key="item.name"
      :to="{ name: item.name }"
      :class="itemClass(item.name)"
      :aria-label="item.label"
      :aria-current="isActive(item.name) ? 'page' : undefined"
      :title="item.label"
    >
      {{ item.glyph }}
    </RouterLink>

    <RouterLink
      v-if="settingsItem"
      :to="{ name: settingsItem.name }"
      :class="[itemClass(settingsItem.name), 'mt-auto']"
      :aria-label="settingsItem.label"
      :aria-current="isActive(settingsItem.name) ? 'page' : undefined"
      :title="settingsItem.label"
    >
      {{ settingsItem.glyph }}
    </RouterLink>
  </nav>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";

type NavItem = {
  name: string;
  label: string;
  glyph: string;
  order: number;
};

const router = useRouter();
const route = useRoute();

const betaEnabled = import.meta.env.VITE_MANIFOLD_FEATURE_GATE === "beta";

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
