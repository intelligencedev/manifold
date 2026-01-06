<template>
  <div
    class="fixed inset-0 z-50 flex items-center justify-center px-4 py-8"
    @keydown="onKeydown"
  >
    <div
      class="absolute inset-0 bg-surface/70 backdrop-blur-sm"
      @click="emitCancel"
    ></div>

    <div
      ref="panel"
      class="relative z-10 flex w-full max-w-5xl flex-col overflow-hidden rounded-xl border border-border/70 bg-surface shadow-2xl"
    >
      <div
        class="flex items-center justify-between border-b border-border/60 px-5 py-4"
      >
        <div>
          <h3 class="text-base font-semibold text-foreground">Manage tools</h3>
          <p class="text-xs text-subtle-foreground">{{ summaryLabel }}</p>
        </div>
        <button
          ref="closeBtn"
          type="button"
          class="rounded border border-border/60 bg-surface-muted px-3 py-1 text-xs font-semibold text-subtle-foreground hover:border-border"
          @click="emitCancel"
        >
          Close
        </button>
      </div>

      <div
        class="grid min-h-0 flex-1 grid-cols-1 gap-4 px-5 py-4 md:grid-cols-5"
      >
        <!-- left: list -->
        <div class="md:col-span-3 flex min-h-0 flex-col gap-3">
          <div
            class="flex flex-col gap-3 md:flex-row md:items-end md:justify-between"
          >
            <div class="flex-1">
              <label
                for="tools-search"
                class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
                >Search</label
              >
              <input
                id="tools-search"
                ref="searchInput"
                v-model="search"
                type="text"
                placeholder="Search by name or description"
                class="mt-1 w-full rounded border border-border/60 bg-surface-muted/40 px-3 py-2 text-sm text-foreground"
              />
            </div>
            <div class="flex items-center gap-2 text-xs">
              <button
                type="button"
                class="rounded border border-border/60 bg-surface px-3 py-1 font-semibold text-subtle-foreground hover:border-border disabled:cursor-not-allowed disabled:opacity-40"
                :disabled="!tools.length"
                @click="selectAll"
              >
                Select all
              </button>
              <button
                type="button"
                class="rounded border border-border/60 bg-surface px-3 py-1 text-subtle-foreground hover:border-border disabled:cursor-not-allowed disabled:opacity-40"
                :disabled="!localSelected.length"
                @click="clear"
              >
                Clear
              </button>
            </div>
          </div>

          <div
            class="min-h-[280px] flex-1 overflow-y-auto rounded border border-border/60 bg-surface-muted/40"
          >
            <div
              v-if="loading"
              class="flex h-full items-center justify-center text-sm text-subtle-foreground"
            >
              Loading tools…
            </div>
            <div
              v-else-if="error"
              class="flex h-full items-center justify-center px-4 text-center text-sm text-danger-foreground"
            >
              {{ error }}
            </div>
            <div
              v-else-if="!tools.length"
              class="flex h-full items-center justify-center px-4 text-center text-sm text-subtle-foreground"
            >
              No tools available.
            </div>
            <div
              v-else-if="!filtered.length"
              class="flex h-full items-center justify-center px-4 text-center text-sm text-subtle-foreground"
            >
              No tools match "{{ search }}".
            </div>
            <ul v-else class="divide-y divide-border/40">
              <li v-for="t in filtered" :key="t.name">
                <label
                  class="flex cursor-pointer items-start gap-3 px-4 py-3 hover:bg-surface"
                  @mouseenter="highlighted = t"
                  @focusin="highlighted = t"
                >
                  <input
                    type="checkbox"
                    class="mt-1 h-4 w-4"
                    :checked="localSet.has(t.name)"
                    @change="onToggle(t.name, $event)"
                  />
                  <div class="min-w-0">
                    <p class="text-sm font-medium text-foreground break-words">
                      {{ t.name }}
                    </p>
                    <p class="text-xs text-subtle-foreground break-words">
                      {{ t.description || "No description provided." }}
                    </p>
                  </div>
                </label>
              </li>
            </ul>
          </div>
        </div>

        <!-- right: details -->
        <div class="md:col-span-2 min-h-0">
          <div
            class="h-full rounded border border-border/60 bg-surface-muted/20 p-4"
          >
            <p
              class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
            >
              Details
            </p>
            <div v-if="highlighted" class="mt-2">
              <p class="text-sm font-semibold text-foreground break-words">
                {{ highlighted.name }}
              </p>
              <p class="mt-1 text-xs text-subtle-foreground break-words">
                {{ highlighted.description || "No description provided." }}
              </p>
            </div>
            <p v-else class="mt-2 text-sm text-subtle-foreground">
              Hover a tool to see details.
            </p>
          </div>
        </div>
      </div>

      <div
        class="flex items-center justify-between border-t border-border/60 px-5 py-3 text-xs text-subtle-foreground"
      >
        <span
          >{{ localSelected.length }}
          {{ localSelected.length === 1 ? "tool" : "tools" }} selected</span
        >
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="rounded border border-border/60 bg-surface px-3 py-1 text-xs font-semibold text-subtle-foreground hover:border-border"
            @click="emitCancel"
          >
            Cancel
          </button>
          <button
            type="button"
            class="rounded border border-border/60 bg-surface-muted px-3 py-1 text-xs font-semibold text-subtle-foreground hover:border-border"
            @click="emitApply"
          >
            Apply
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";
import type { WarppTool } from "@/types/warpp";

const props = defineProps<{
  open: boolean;
  tools: WarppTool[];
  loading: boolean;
  error: string;
  selected: string[];
}>();

const emit = defineEmits<{ apply: [string[]]; cancel: [] }>();

const search = ref("");
const highlighted = ref<WarppTool | null>(null);

const localSelected = ref<string[]>([]);
const localSet = computed(() => new Set(localSelected.value));

const panel = ref<HTMLElement | null>(null);
const searchInput = ref<HTMLInputElement | null>(null);
const closeBtn = ref<HTMLButtonElement | null>(null);
const restoreFocusEl = ref<HTMLElement | null>(null);

watch(
  () => props.open,
  async (open) => {
    if (!open) return;
    restoreFocusEl.value = document.activeElement as HTMLElement | null;
    localSelected.value = [...(props.selected || [])];
    search.value = "";
    highlighted.value = null;
    await nextTick();
    (searchInput.value || closeBtn.value)?.focus();
  },
);

const summaryLabel = computed(() => {
  if (props.loading) return "Loading available tools…";
  if (props.error) return props.error || "Unable to load tools";
  if (!props.tools.length) return "No tools available";
  return `${props.tools.length} available`;
});

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase();
  if (!q) return props.tools;
  return props.tools.filter((t) => {
    const name = (t.name || "").toLowerCase();
    const desc = (t.description || "").toLowerCase();
    return name.includes(q) || desc.includes(q);
  });
});

function onToggle(name: string, event: Event) {
  const checked = !!(event.target as HTMLInputElement | null)?.checked;
  const set = new Set(localSelected.value);
  if (checked) set.add(name);
  else set.delete(name);
  localSelected.value = Array.from(set).sort((a, b) =>
    a.localeCompare(b, undefined, { sensitivity: "base" }),
  );
}

function selectAll() {
  localSelected.value = props.tools.map((t) => t.name).filter(Boolean);
}

function clear() {
  localSelected.value = [];
}

function emitCancel() {
  emit("cancel");
  nextTick(() => restoreFocusEl.value?.focus());
}

function emitApply() {
  emit("apply", [...localSelected.value]);
  nextTick(() => restoreFocusEl.value?.focus());
}

function focusables(): HTMLElement[] {
  const root = panel.value;
  if (!root) return [];
  return Array.from(
    root.querySelectorAll<HTMLElement>(
      'a[href],button:not([disabled]),textarea:not([disabled]),input:not([disabled]),select:not([disabled]),[tabindex]:not([tabindex="-1"])',
    ),
  ).filter((el) => !el.hasAttribute("disabled") && el.tabIndex !== -1);
}

function onKeydown(e: KeyboardEvent) {
  if (!props.open) return;
  if (e.key === "Escape") {
    e.preventDefault();
    emitCancel();
    return;
  }
  if (e.key !== "Tab") return;
  const els = focusables();
  if (!els.length) return;
  const first = els[0];
  const last = els[els.length - 1];
  const active = document.activeElement as HTMLElement | null;

  if (e.shiftKey) {
    if (active === first || !rootContains(active)) {
      e.preventDefault();
      last.focus();
    }
  } else {
    if (active === last || !rootContains(active)) {
      e.preventDefault();
      first.focus();
    }
  }
}

function rootContains(el: HTMLElement | null): boolean {
  return !!(panel.value && el && panel.value.contains(el));
}

onMounted(() => {
  if (props.open) {
    restoreFocusEl.value = document.activeElement as HTMLElement | null;
  }
});
</script>
