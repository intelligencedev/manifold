<template>
  <div class="flex h-full min-h-0 flex-col gap-4 overflow-hidden">
    <!-- Toolbar -->
    <header class="flex shrink-0 flex-wrap items-center justify-between gap-3">
      <div class="flex items-baseline gap-2.5">
        <h2 class="text-base font-semibold leading-none">Prompts</h2>
        <span class="font-mono text-xs text-faint-foreground">
          {{ countLabel }}
        </span>
      </div>

      <div class="flex items-center gap-2">
        <div class="relative">
          <svg
            class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-faint-foreground"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            aria-hidden="true"
          >
            <circle cx="11" cy="11" r="7" />
            <path d="m20 20-3.5-3.5" />
          </svg>
          <input
            v-model="search"
            type="search"
            placeholder="Search name or tag"
            aria-label="Search prompts"
            class="halo-focus w-56 rounded-md border border-[rgb(var(--line-strong))] bg-surface py-2 pl-9 pr-3 text-sm text-foreground outline-none placeholder:text-faint-foreground focus:w-64 transition-[width]"
          />
        </div>
        <AppButton variant="accent" size="sm" @click="toggleCreate">
          <svg
            class="h-4 w-4"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.2"
            stroke-linecap="round"
            aria-hidden="true"
          >
            <path d="M12 5v14M5 12h14" />
          </svg>
          New Prompt
        </AppButton>
      </div>
    </header>

    <!-- Create panel (collapsible) -->
    <Transition name="reveal">
      <GlassCard v-if="showCreate" as="section" subtle padded class="shrink-0 p-5">
        <form class="grid grid-cols-2 gap-4" @submit.prevent="handleCreate">
          <div class="col-span-2 flex items-center justify-between">
            <div>
              <h3 class="text-sm font-semibold">Create Prompt</h3>
              <p class="text-xs text-subtle-foreground">
                Define a prompt template and optional metadata.
              </p>
            </div>
            <button
              type="button"
              class="rounded-md p-1.5 text-faint-foreground transition-colors hover:bg-surface-muted hover:text-foreground"
              aria-label="Close create panel"
              @click="closeCreate"
            >
              <svg
                class="h-4 w-4"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                aria-hidden="true"
              >
                <path d="M18 6 6 18M6 6l12 12" />
              </svg>
            </button>
          </div>

          <label class="flex flex-col gap-1.5 text-sm">
            <span class="text-xs font-medium text-subtle-foreground">Name</span>
            <input
              ref="nameInput"
              v-model="form.name"
              required
              placeholder="e.g. orchestrator"
              :class="fieldClass"
            />
          </label>

          <label class="flex flex-col gap-1.5 text-sm">
            <span class="text-xs font-medium text-subtle-foreground">
              Tags <span class="text-faint-foreground">(comma separated)</span>
            </span>
            <input
              v-model="form.tags"
              placeholder="sales, onboarding"
              :class="fieldClass"
            />
          </label>

          <label class="col-span-2 flex flex-col gap-1.5 text-sm">
            <span class="text-xs font-medium text-subtle-foreground"
              >Description</span
            >
            <textarea
              v-model="form.description"
              rows="2"
              placeholder="What is this prompt for?"
              :class="[fieldClass, 'resize-none']"
            ></textarea>
          </label>

          <div class="col-span-2 flex items-center gap-3">
            <AppButton type="submit" variant="accent" size="sm" :loading="creating">
              Create prompt
            </AppButton>
            <AppButton type="button" variant="ghost" size="sm" @click="closeCreate">
              Cancel
            </AppButton>
            <Transition name="fade">
              <span
                v-if="createStatus"
                class="text-xs font-medium text-accent"
                >{{ createStatus }}</span
              >
            </Transition>
          </div>
        </form>
      </GlassCard>
    </Transition>

    <!-- Error banner -->
    <div
      v-if="store.promptsError"
      class="shrink-0 rounded-md border border-danger/50 bg-[rgb(var(--color-danger)_/_0.1)] px-3 py-2 text-sm text-danger"
    >
      {{ store.promptsError }}
    </div>

    <!-- Prompt grid -->
    <div class="-mx-1 min-h-0 flex-1 overflow-y-auto overscroll-contain px-1">
      <!-- Loading skeletons -->
      <div
        v-if="store.promptsLoading && !store.prompts.length"
        class="grid gap-3"
        :style="gridStyle"
      >
        <div
          v-for="n in 6"
          :key="n"
          class="halo-surface halo-surface-2 animate-pulse p-5"
        >
          <div class="flex items-center gap-3">
            <div class="h-9 w-9 rounded-md bg-surface-muted"></div>
            <div class="h-4 flex-1 rounded bg-surface-muted"></div>
          </div>
          <div class="mt-4 h-3 w-3/4 rounded bg-surface-muted"></div>
          <div class="mt-2 h-3 w-1/2 rounded bg-surface-muted"></div>
        </div>
      </div>

      <!-- Empty: no prompts at all -->
      <div
        v-else-if="!store.prompts.length"
        class="flex h-full flex-col items-center justify-center gap-4 text-center"
      >
        <div
          class="flex h-14 w-14 items-center justify-center rounded-xl border border-[rgb(var(--line-strong))] bg-surface-muted text-accent"
        >
          <svg
            class="h-7 w-7"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.6"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
          >
            <path d="M4 4h11l5 5v11a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1Z" />
            <path d="M14 4v5h5" />
            <path d="M8 13h6M8 17h4" />
          </svg>
        </div>
        <div>
          <p class="text-sm font-semibold">No prompts yet</p>
          <p class="mx-auto mt-1 max-w-xs text-sm text-subtle-foreground">
            Prompts hold versioned templates you can test and run in experiments.
          </p>
        </div>
        <AppButton variant="accent" size="sm" @click="openCreate">
          Create your first prompt
        </AppButton>
      </div>

      <!-- Empty: no search matches -->
      <div
        v-else-if="!filteredPrompts.length"
        class="flex h-full flex-col items-center justify-center gap-3 text-center"
      >
        <p class="text-sm font-medium text-subtle-foreground">
          No prompts match “{{ search }}”.
        </p>
        <AppButton variant="ghost" size="sm" @click="search = ''">
          Clear search
        </AppButton>
      </div>

      <!-- Cards -->
      <div v-else class="grid items-stretch gap-3" :style="gridStyle">
        <GlassCard
          v-for="prompt in filteredPrompts"
          :key="prompt.id"
          as="article"
          padded
          interactive
          class="group relative flex min-h-[13.5rem] flex-col gap-3.5 p-5"
        >
          <RouterLink
            :to="`/playground/prompts/${prompt.id}`"
            class="absolute inset-0 z-10 rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            :aria-label="`Open prompt ${prompt.name}`"
          />

          <!-- Header -->
          <div class="flex items-start gap-3">
            <div
              class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-[rgb(var(--line-strong))] bg-surface-muted text-accent"
            >
              <svg
                class="h-[18px] w-[18px]"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
                stroke-linecap="round"
                stroke-linejoin="round"
                aria-hidden="true"
              >
                <path d="m8 6-4 6 4 6M16 6l4 6-4 6" />
              </svg>
            </div>
            <div class="min-w-0 flex-1 pt-0.5">
              <h3 class="truncate text-sm font-semibold leading-tight text-foreground">
                {{ prompt.name }}
              </h3>
              <p class="mt-0.5 truncate font-mono text-[11px] text-faint-foreground">
                {{ shortId(prompt.id) }}
              </p>
            </div>
            <button
              class="relative z-20 -mr-1 -mt-1 rounded-md p-1.5 text-faint-foreground opacity-0 transition-all hover:bg-[rgb(var(--color-danger)_/_0.12)] hover:text-danger focus-visible:opacity-100 group-hover:opacity-100"
              :aria-label="`Delete prompt ${prompt.name}`"
              @click="confirmDeletePrompt(prompt.id)"
            >
              <svg
                class="h-4 w-4"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
                stroke-linecap="round"
                stroke-linejoin="round"
                aria-hidden="true"
              >
                <path d="M3 6h18M8 6V4h8v2M6 6l1 14h10l1-14" />
              </svg>
            </button>
          </div>

          <!-- Description (reserves 2 lines so cards align) -->
          <p
            v-if="prompt.description"
            class="line-clamp-2 min-h-[2.5rem] text-sm leading-snug text-muted-foreground"
          >
            {{ prompt.description }}
          </p>
          <p v-else class="min-h-[2.5rem] text-sm italic text-faint-foreground">
            No description
          </p>

          <!-- Tags -->
          <div v-if="prompt.tags?.length" class="flex flex-wrap gap-1.5">
            <Chip v-for="tag in prompt.tags.slice(0, 4)" :key="tag">{{ tag }}</Chip>
            <Chip v-if="prompt.tags.length > 4" muted>
              +{{ prompt.tags.length - 4 }}
            </Chip>
          </div>

          <!-- Footer -->
          <div
            class="mt-auto flex items-center justify-between border-t border-[rgb(var(--line-soft))] pt-3 font-mono text-[11px] text-faint-foreground"
          >
            <span>{{ formatDate(prompt.createdAt) }}</span>
            <span
              class="flex items-center gap-1 text-subtle-foreground opacity-0 transition-opacity group-hover:opacity-100"
            >
              Open
              <svg
                class="h-3.5 w-3.5"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
                aria-hidden="true"
              >
                <path d="M5 12h14M13 6l6 6-6 6" />
              </svg>
            </span>
          </div>
        </GlassCard>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { RouterLink } from "vue-router";
import { computed, nextTick, onMounted, reactive, ref } from "vue";
import { usePlaygroundStore } from "@/stores/playground";
import GlassCard from "@/components/ui/GlassCard.vue";
import AppButton from "@/components/ui/AppButton.vue";
import Chip from "@/components/ui/Chip.vue";

const store = usePlaygroundStore();
const form = reactive({ name: "", description: "", tags: "" });
const createStatus = ref("");
const creating = ref(false);
const search = ref("");
const deleting = ref<string | null>(null);
const showCreate = ref(false);
const nameInput = ref<HTMLInputElement | null>(null);

const fieldClass =
  "halo-focus w-full rounded-md border border-[rgb(var(--line-strong))] bg-surface px-[13px] py-2.5 font-sans text-sm text-foreground outline-none placeholder:text-faint-foreground";

const gridStyle = {
  gridTemplateColumns: "repeat(auto-fill, minmax(260px, 1fr))",
};

onMounted(async () => {
  if (!store.prompts.length) {
    await store.loadPrompts();
  }
});

const filteredPrompts = computed(() => {
  if (!search.value) return store.prompts;
  const term = search.value.toLowerCase();
  return store.prompts.filter((p) => {
    return (
      p.name.toLowerCase().includes(term) ||
      (p.description && p.description.toLowerCase().includes(term)) ||
      (p.tags && p.tags.some((t) => t.toLowerCase().includes(term)))
    );
  });
});

const countLabel = computed(() => {
  const total = store.prompts.length;
  if (!total) return "";
  if (search.value && filteredPrompts.value.length !== total) {
    return `${filteredPrompts.value.length} of ${total}`;
  }
  return `${total} prompt${total === 1 ? "" : "s"}`;
});

function toggleCreate() {
  if (showCreate.value) {
    closeCreate();
  } else {
    openCreate();
  }
}

function openCreate() {
  showCreate.value = true;
  void nextTick(() => nameInput.value?.focus());
}

function closeCreate() {
  showCreate.value = false;
}

async function handleCreate() {
  if (creating.value) return;
  creating.value = true;
  try {
    await store.addPrompt(form);
    createStatus.value = "Prompt created.";
    form.name = "";
    form.description = "";
    form.tags = "";
    void nextTick(() => nameInput.value?.focus());
    setTimeout(() => (createStatus.value = ""), 3_000);
  } finally {
    creating.value = false;
  }
}

async function confirmDeletePrompt(id: string) {
  if (deleting.value) return;
  const ok = window.confirm("Delete this prompt and all its versions?");
  if (!ok) return;
  try {
    deleting.value = id;
    await store.removePrompt(id);
  } finally {
    deleting.value = null;
  }
}

function shortId(id: string) {
  return id.length > 12 ? `${id.slice(0, 8)}…${id.slice(-4)}` : id;
}

function formatDate(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}
</script>

<style scoped>
.reveal-enter-active,
.reveal-leave-active {
  transition:
    opacity 0.18s ease,
    transform 0.18s ease;
}
.reveal-enter-from,
.reveal-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
