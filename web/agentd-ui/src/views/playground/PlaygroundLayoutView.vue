<template>
  <section class="flex h-full min-h-0 flex-col">
    <header class="mb-5 flex items-start justify-between gap-4">
      <div>
        <h1 class="font-display text-2xl font-semibold text-foreground">
          Playground
        </h1>
        <p class="text-sm text-muted-foreground">
          Prompt and experiment lab.
        </p>
      </div>
    </header>

    <MTabs v-model="activeTab" :tabs="tabs" />

    <div class="min-h-0 flex-1 overflow-hidden pt-5">
      <RouterView />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { RouterView, useRoute, useRouter } from "vue-router";
import MTabs from "@/components/ui/MTabs.vue";

const route = useRoute();
const router = useRouter();

const tabs = [
  { label: "Overview", value: "/playground" },
  { label: "Prompts", value: "/playground/prompts" },
  { label: "Datasets", value: "/playground/datasets" },
  { label: "Experiments", value: "/playground/experiments" },
];

const activeTab = computed({
  get: () => {
    const match = [...tabs]
      .reverse()
      .find((item) => route.path === item.value || route.path.startsWith(`${item.value}/`));
    return match?.value ?? "/playground";
  },
  set: (value: string) => {
    void router.push(value);
  },
});
</script>
