<template>
  <div v-if="prompt" class="flex h-full min-h-0 flex-col overflow-hidden">

    <!-- Single-row header: back · name/meta · delete -->
    <header class="mb-4 flex shrink-0 items-center gap-4 border-b border-border/60 pb-3">
      <RouterLink
        to="/playground/prompts"
        class="flex shrink-0 items-center gap-1.5 rounded-lg border border-border/70 px-3 py-1.5 text-sm font-medium transition-colors hover:bg-muted/60"
      >
        ← All Prompts
      </RouterLink>

      <div class="min-w-0 flex-1">
        <h2 class="truncate text-base font-semibold leading-tight">{{ prompt.name }}</h2>
        <div class="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-0 text-xs text-subtle-foreground">
          <span v-if="prompt.description" class="max-w-sm truncate">{{ prompt.description }}</span>
          <span v-if="prompt.tags?.length">{{ prompt.tags.join(", ") }}</span>
          <span>{{ formatDate(prompt.createdAt) }}</span>
          <span>{{ versions.length }} version{{ versions.length !== 1 ? "s" : "" }}</span>
        </div>
      </div>

      <button
        class="shrink-0 rounded border border-danger/50 px-3 py-1.5 text-sm text-danger/70 transition-colors hover:bg-danger/10"
        @click="deletePrompt(promptId)"
      >
        Delete
      </button>
    </header>

    <!-- Two-column workspace: editor left, versions right -->
    <div class="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_280px] gap-4">

      <!-- Editor column -->
      <form class="flex min-h-0 flex-col" @submit.prevent="handleCreateVersion">
        <textarea
          v-model="versionForm.template"
          required
          placeholder="Write the system prompt here…"
          class="min-h-0 flex-1 resize-none rounded-t-xl border border-border/70 bg-surface-muted/60 px-4 py-3 font-mono text-sm leading-6 outline-none placeholder:text-subtle-foreground/50 focus:border-accent/60"
        ></textarea>

        <!-- Action bar — attached flush to the textarea bottom -->
        <div class="flex flex-wrap items-center gap-3 rounded-b-xl border border-t-0 border-border/70 bg-surface/80 px-4 py-2.5">
          <input
            v-model="versionForm.semver"
            placeholder="semver (1.0.0)"
            class="w-28 shrink-0 rounded border border-border/70 bg-surface-muted/70 px-2.5 py-1.5 text-xs outline-none focus:border-accent/60"
          />
          <label class="flex min-w-0 flex-1 items-center gap-2">
            <span class="shrink-0 text-xs text-subtle-foreground">Variables</span>
            <input
              v-model="versionForm.variables"
              placeholder='{"name":{"type":"string"}}'
              class="min-w-0 flex-1 rounded border border-border/70 bg-surface-muted/70 px-2.5 py-1.5 font-mono text-xs outline-none focus:border-accent/60"
            />
          </label>
          <label class="flex min-w-0 flex-1 items-center gap-2">
            <span class="shrink-0 text-xs text-subtle-foreground">Guardrails</span>
            <input
              v-model="versionForm.guardrails"
              placeholder='{"maxTokens":200}'
              class="min-w-0 flex-1 rounded border border-border/70 bg-surface-muted/70 px-2.5 py-1.5 font-mono text-xs outline-none focus:border-accent/60"
            />
          </label>
          <div class="flex shrink-0 items-center gap-3">
            <span v-if="createMessage" class="text-xs text-subtle-foreground">{{ createMessage }}</span>
            <button
              type="submit"
              class="rounded border border-border/70 px-4 py-1.5 text-sm font-semibold transition-colors hover:bg-muted/60"
            >
              Save version
            </button>
          </div>
        </div>
      </form>

      <!-- Versions sidebar -->
      <aside class="flex min-h-0 flex-col overflow-hidden rounded-xl border border-border/60 bg-surface/60">
        <div class="flex shrink-0 items-center justify-between border-b border-border/60 px-4 py-2.5">
          <h3 class="text-sm font-semibold">Versions</h3>
          <span class="text-xs text-subtle-foreground">{{ versions.length }} total</span>
        </div>

        <div class="min-h-0 flex-1 overflow-auto overscroll-contain">
          <div v-if="versionsLoading" class="p-4 text-center text-sm text-subtle-foreground">
            Loading…
          </div>
          <div v-else-if="versions.length === 0" class="p-4 text-center text-sm text-subtle-foreground">
            No versions yet.
          </div>
          <div
            v-else
            v-for="version in versions"
            :key="version.id"
            class="flex cursor-pointer items-center justify-between gap-3 border-b border-border/40 px-4 py-2.5 transition-colors"
            :class="{
              'bg-accent/10': version.id === selectedVersionId,
              'hover:bg-muted/60': version.id !== selectedVersionId,
            }"
            @click="selectVersion(version.id)"
          >
            <div class="min-w-0">
              <div class="truncate text-sm font-medium">{{ version.semver || version.id }}</div>
              <div class="text-xs text-subtle-foreground">{{ formatDate(version.createdAt) }}</div>
            </div>
            <button
              v-if="version.id === selectedVersionId"
              @click.stop="loadIntoForm(version)"
              class="shrink-0 rounded border border-border/70 px-2.5 py-1 text-xs transition-colors hover:bg-muted/60"
            >
              Load
            </button>
          </div>
        </div>
      </aside>
    </div>
  </div>
  <p v-else class="text-sm text-subtle-foreground">Loading prompt…</p>
</template>

<script setup lang="ts">
import { RouterLink, useRoute, useRouter } from "vue-router";
import { onMounted, reactive, ref, watch, computed } from "vue";
import { usePlaygroundStore } from "@/stores/playground";
import type { Prompt, PromptVersion } from "@/api/playground";

const route = useRoute();
const router = useRouter();
const store = usePlaygroundStore();
const promptId = ref(route.params.promptId as string);

const prompt = ref<Prompt | null>(null);
const versions = ref<PromptVersion[]>(
  store.promptVersions[promptId.value] ?? [],
);
const versionsLoading = ref(false);
const createMessage = ref("");
const versionForm = reactive({
  semver: "",
  template: "",
  variables: "",
  guardrails: "",
});
const formDirty = ref(false);

const selectedVersionId = ref<string | null>(null);
const selectedVersion = computed<PromptVersion | null>(() => {
  if (!selectedVersionId.value) return null;
  return versions.value.find((v) => v.id === selectedVersionId.value) ?? null;
});

async function refreshVersions(id: string) {
  versionsLoading.value = true;
  await store.loadPromptVersions(id);
  versions.value = store.promptVersions[id] ?? [];
  versionsLoading.value = false;
  ensureVersionSelection();
  prefillFromLatest();
}

onMounted(async () => {
  const ok = await loadPrompt(promptId.value);
  if (ok) {
    await refreshVersions(promptId.value);
  }
});

watch(
  () => route.params.promptId,
  async (next) => {
    if (typeof next !== "string") return;
    promptId.value = next;
    versions.value = [];
    selectedVersionId.value = null;
    formDirty.value = false;
    const ok = await loadPrompt(next);
    if (ok) {
      await refreshVersions(next);
    }
  },
);

async function handleCreateVersion() {
  await store.addPromptVersion(promptId.value, versionForm);
  createMessage.value = "Version created.";
  versionForm.semver = "";
  versionForm.template = "";
  versionForm.variables = "";
  versionForm.guardrails = "";
  formDirty.value = false;
  await refreshVersions(promptId.value);
  setTimeout(() => (createMessage.value = ""), 3000);
}

async function deletePrompt(id: string) {
  const ok = window.confirm("Delete this prompt and all versions?");
  if (!ok) return;
  await store.removePrompt(id);
  await router.replace("/playground/prompts");
}

function formatDate(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

async function loadPrompt(id: string) {
  prompt.value = await store.ensurePrompt(id);
  if (!prompt.value) {
    await router.replace("/playground/prompts");
    return false;
  }
  return true;
}

function ensureVersionSelection() {
  const current = versions.value;
  if (
    selectedVersionId.value &&
    current.some((v) => v.id === selectedVersionId.value)
  ) {
    return;
  }
  const first = current[0];
  selectedVersionId.value = first ? first.id : null;
}

function asPrettyJSON(value: unknown) {
  if (!value) return "";
  try {
    return JSON.stringify(value, null, 2);
  } catch (err) {
    return String(value);
  }
}

function loadIntoForm(v: PromptVersion) {
  versionForm.semver = v.semver || "";
  versionForm.template = v.template || "";
  versionForm.variables = v.variables ? asPrettyJSON(v.variables) : "";
  versionForm.guardrails = v.guardrails ? asPrettyJSON(v.guardrails) : "";
  formDirty.value = true;
}

function prefillFromLatest() {
  if (formDirty.value) return;
  if (!versionForm.template && versions.value.length > 0) {
    const v = versions.value[0];
    versionForm.template = v.template || "";
    versionForm.variables = v.variables ? asPrettyJSON(v.variables) : "";
    versionForm.guardrails = v.guardrails ? asPrettyJSON(v.guardrails) : "";
  }
}

function selectVersion(id: string) {
  if (selectedVersionId.value === id) return;
  selectedVersionId.value = id;
  const v = versions.value.find((x) => x.id === id);
  if (v) {
    loadIntoForm(v);
  }
}

watch(
  () => ({ ...versionForm }),
  () => {
    formDirty.value = true;
  },
);
</script>
