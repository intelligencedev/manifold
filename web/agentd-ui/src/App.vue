<template>
  <div v-if="isSetupRoute" class="min-h-screen bg-background text-foreground">
    <RouterView />
  </div>
  <AppShell v-else :inspector="false">
    <template #rail>
      <IconRail />
    </template>

    <template #topbar>
      <BreadcrumbTopbar :title="sectionTitle" :crumb="sectionCrumb">
        <template #actions>
          <AccountButton :username="user?.name || user?.email" />
        </template>
      </BreadcrumbTopbar>
    </template>

    <RouterView />
    <MCommandBar v-if="commandOpen" @close="closeCommand" />
  </AppShell>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { RouterView, useRoute } from "vue-router";
import AccountButton from "@/components/AccountButton.vue";
import AppShell from "@/components/ui/AppShell.vue";
import BreadcrumbTopbar from "@/components/ui/BreadcrumbTopbar.vue";
import IconRail from "@/components/ui/IconRail.vue";
import MCommandBar from "@/components/ui/MCommandBar.vue";

const route = useRoute();
const isSetupRoute = computed(() => route.name === "setup");

const user = ref<{ name?: string; email?: string; picture?: string } | null>(
  null,
);
const commandOpen = ref(false);

const metaSource = computed(() => {
  const matched = [...route.matched]
    .reverse()
    .find((record) => record.meta?.title || record.meta?.purpose);
  return matched?.meta ?? route.meta;
});

const sectionTitle = computed(() =>
  String(metaSource.value.title ?? metaSource.value.label ?? "Manifold"),
);

const sectionCrumb = computed(() => {
  const purpose = metaSource.value.purpose;
  return purpose ? String(purpose) : undefined;
});

function openCommand() {
  commandOpen.value = true;
}

function closeCommand() {
  commandOpen.value = false;
}

function handleGlobalKeydown(event: KeyboardEvent) {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
    event.preventDefault();
    openCommand();
  }
}

onMounted(async () => {
  window.addEventListener("keydown", handleGlobalKeydown);
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

onBeforeUnmount(() => {
  window.removeEventListener("keydown", handleGlobalKeydown);
});
</script>
