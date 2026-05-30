<template>
  <Teleport to="body">
    <div
      class="fixed inset-0 z-[80] bg-[rgb(8_9_12_/_0.6)]"
      role="presentation"
      @mousedown.self="close"
    >
      <div
        ref="panelRef"
        class="mx-auto mt-[18vh] w-full max-w-[560px] rounded-lg border border-[rgb(var(--line-strong))] bg-surface p-2"
        role="dialog"
        aria-modal="true"
        aria-label="Command bar"
        @keydown="onPanelKeydown"
      >
        <div
          class="flex items-center gap-3 rounded-md border border-[rgb(var(--line-strong))] bg-surface px-3.5 py-[11px]"
        >
          <span class="font-mono text-[12px] text-[rgb(var(--accent-hi))]">
            &gt;
          </span>
          <input
            ref="inputRef"
            v-model="query"
            class="min-w-0 flex-1 bg-transparent font-sans text-[14.5px] text-foreground outline-none placeholder:text-faint-foreground"
            placeholder="Open..."
            @keydown.enter.prevent="selectFirst"
          />
          <span
            class="rounded-[5px] border border-border px-1.5 py-0.5 font-mono text-[10px] text-faint-foreground"
          >
            ⌘K
          </span>
        </div>

        <div class="mt-2 max-h-[320px] overflow-auto">
          <button
            v-for="item in filteredItems"
            :key="item.name"
            type="button"
            class="halo-focus flex w-full items-center gap-3 rounded-md border border-transparent px-3 py-2.5 text-left transition-colors hover:bg-surface-muted"
            @click="selectItem(item.name)"
          >
            <span
              class="grid h-7 w-7 place-items-center rounded-[7px] bg-input font-mono text-[10px] text-[rgb(var(--accent-hi))]"
            >
              {{ item.glyph }}
            </span>
            <span class="min-w-0">
              <span class="block text-sm font-medium text-foreground">
                {{ item.label }}
              </span>
              <span
                class="block truncate font-mono text-[10px] uppercase tracking-[0.1em] text-faint-foreground"
              >
                {{ item.purpose }}
              </span>
            </span>
          </button>
          <p
            v-if="filteredItems.length === 0"
            class="px-3 py-8 text-center text-sm text-faint-foreground"
          >
            No commands
          </p>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from "vue";
import { useRouter } from "vue-router";

type CommandItem = {
  name: string;
  label: string;
  glyph: string;
  purpose: string;
  order: number;
};

const emit = defineEmits<{
  close: [];
}>();

const router = useRouter();
const query = ref("");
const inputRef = ref<HTMLInputElement | null>(null);
const panelRef = ref<HTMLElement | null>(null);
const previousActive = ref<HTMLElement | null>(null);
const betaEnabled = import.meta.env.VITE_MANIFOLD_FEATURE_GATE === "beta";

const items = computed<CommandItem[]>(() =>
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
      purpose: String(record.meta.purpose ?? ""),
      order: Number(record.meta.order ?? 999),
    }))
    .sort((a, b) => a.order - b.order),
);

const filteredItems = computed(() => {
  const needle = query.value.trim().toLowerCase();
  if (!needle) return items.value;
  return items.value.filter((item) =>
    `${item.label} ${item.purpose}`.toLowerCase().includes(needle),
  );
});

function close() {
  emit("close");
}

async function selectItem(name: string) {
  await router.push({ name });
  close();
}

function selectFirst() {
  const first = filteredItems.value[0];
  if (first) void selectItem(first.name);
}

function focusableElements() {
  const panel = panelRef.value;
  if (!panel) return [];
  return Array.from(
    panel.querySelectorAll<HTMLElement>(
      'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
    ),
  ).filter((el) => !el.hasAttribute("disabled"));
}

function onPanelKeydown(event: KeyboardEvent) {
  if (event.key === "Escape") {
    event.preventDefault();
    close();
    return;
  }
  if (event.key !== "Tab") return;

  const focusable = focusableElements();
  if (focusable.length === 0) return;

  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  const active = document.activeElement;

  if (event.shiftKey && active === first) {
    event.preventDefault();
    last.focus();
  } else if (!event.shiftKey && active === last) {
    event.preventDefault();
    first.focus();
  }
}

onMounted(() => {
  previousActive.value = document.activeElement as HTMLElement | null;
  void nextTick(() => inputRef.value?.focus());
});

onBeforeUnmount(() => {
  previousActive.value?.focus?.();
});
</script>
