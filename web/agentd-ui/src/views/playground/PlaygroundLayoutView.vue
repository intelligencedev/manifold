<template>
  <section class="flex h-full min-h-0 flex-col">
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
  { label: "Prompts", value: "/playground/prompts" },
  { label: "Datasets", value: "/playground/datasets" },
  { label: "Experiments", value: "/playground/experiments" },
];

const activeTab = computed({
  get: () => {
    const match = [...tabs]
      .reverse()
      .find((item) => route.path === item.value || route.path.startsWith(`${item.value}/`));
    return match?.value ?? "/playground/prompts";
  },
  set: (value: string) => {
    void router.push(value);
  },
});
</script>
