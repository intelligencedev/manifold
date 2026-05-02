<template>
  <Teleport to="body">
    <Transition name="drawer-overlay">
      <div
        v-if="open"
        class="fixed inset-0 z-50"
        role="dialog"
        aria-modal="true"
        aria-labelledby="log-detail-title"
      >
        <button
          type="button"
          class="absolute inset-0 bg-black/35 backdrop-blur-[1px]"
          aria-label="Close log detail"
          @click="closeDrawer"
        />

        <Transition name="drawer-panel">
          <aside
            v-if="open"
            class="absolute inset-y-0 right-0 flex w-full max-w-[720px] flex-col border-l border-border/70 bg-surface shadow-[0_18px_60px_rgba(0,0,0,0.28)]"
          >
        <header class="flex items-start justify-between gap-4 border-b border-border/70 px-5 py-4 md:px-6">
          <div class="space-y-1">
            <p class="text-[11px] font-semibold uppercase tracking-[0.24em] text-subtle-foreground">
              Log Explorer
            </p>
            <h2 id="log-detail-title" class="text-lg font-semibold text-foreground">
              {{ detail?.message || "Log detail" }}
            </h2>
            <p class="text-xs text-faint-foreground">
              Inspect tags, attributes, and captured prompt payloads for the selected log.
            </p>
          </div>

          <button
            type="button"
            class="shrink-0 rounded-full border border-border/70 px-3 py-1.5 text-xs font-semibold text-subtle-foreground transition hover:border-border hover:text-foreground"
            @click="closeDrawer"
          >
            Close
          </button>
        </header>

        <div class="min-h-0 flex-1 overflow-y-auto px-5 py-5 md:px-6">
          <div
            v-if="isLoading"
            class="rounded-2xl border border-border/70 bg-surface-muted/30 p-4 text-sm text-faint-foreground"
          >
            Loading log details…
          </div>

          <div
            v-else-if="isError"
            class="rounded-2xl border border-danger/60 bg-danger/10 p-4 text-sm text-danger"
          >
            Failed to load log details.
          </div>

          <div
            v-else-if="!detail"
            class="rounded-2xl border border-border/70 bg-surface-muted/30 p-4 text-sm text-faint-foreground"
          >
            No log detail is available for this selection.
          </div>

          <div v-else class="space-y-6">
            <section class="space-y-3">
              <div class="flex flex-wrap gap-2">
                <span :class="['rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.18em]', levelBadgeClass]">
                  {{ detail.level || "info" }}
                </span>
                <span
                  v-if="detail.service"
                  class="rounded-full border border-border/70 bg-muted/20 px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.18em] text-subtle-foreground"
                >
                  {{ detail.service }}
                </span>
              </div>

              <dl class="grid gap-3 sm:grid-cols-2">
                <div class="rounded-2xl border border-border/70 bg-surface-muted/20 p-3">
                  <dt class="text-[10px] font-semibold uppercase tracking-[0.2em] text-subtle-foreground">Timestamp</dt>
                  <dd class="mt-1 text-sm text-foreground">{{ formattedTimestamp }}</dd>
                </div>
                <div class="rounded-2xl border border-border/70 bg-surface-muted/20 p-3">
                  <dt class="text-[10px] font-semibold uppercase tracking-[0.2em] text-subtle-foreground">Log ID</dt>
                  <dd class="mt-1 break-all font-mono text-xs text-foreground">{{ detail.id }}</dd>
                </div>
                <div class="rounded-2xl border border-border/70 bg-surface-muted/20 p-3">
                  <dt class="text-[10px] font-semibold uppercase tracking-[0.2em] text-subtle-foreground">Trace ID</dt>
                  <dd class="mt-1 break-all font-mono text-xs text-foreground">{{ detail.traceId || "—" }}</dd>
                </div>
                <div class="rounded-2xl border border-border/70 bg-surface-muted/20 p-3">
                  <dt class="text-[10px] font-semibold uppercase tracking-[0.2em] text-subtle-foreground">Span ID</dt>
                  <dd class="mt-1 break-all font-mono text-xs text-foreground">{{ detail.spanId || "—" }}</dd>
                </div>
              </dl>
            </section>

            <section class="space-y-2">
              <h3 class="text-sm font-semibold text-foreground">Message</h3>
              <div class="rounded-2xl border border-border/70 bg-surface-muted/20 p-4">
                <pre class="whitespace-pre-wrap break-words font-sans text-sm leading-6 text-foreground">{{ detail.message }}</pre>
              </div>
            </section>

            <section v-if="detail.tags?.length" class="space-y-2">
              <h3 class="text-sm font-semibold text-foreground">Tags</h3>
              <div class="flex flex-wrap gap-2">
                <span
                  v-for="tag in detail.tags"
                  :key="tag"
                  class="rounded-full border border-border/70 bg-muted/20 px-2.5 py-1 font-mono text-[11px] text-subtle-foreground"
                >
                  {{ tag }}
                </span>
              </div>
            </section>

            <section v-if="promptPayload" class="space-y-3">
              <div class="flex items-center justify-between gap-3">
                <h3 class="text-sm font-semibold text-foreground">Prompt Payload</h3>
                <span
                  class="rounded-full border border-border/70 bg-muted/20 px-2 py-0.5 text-[11px] font-medium uppercase tracking-wide text-subtle-foreground"
                >
                  {{ promptPayloadLabel }}
                </span>
              </div>

              <div v-if="promptMessages.length" class="space-y-3">
                <article
                  v-for="(message, index) in promptMessages"
                  :key="`${message.role}-${index}`"
                  class="rounded-2xl border border-border/70 bg-surface-muted/20 p-4"
                >
                  <div class="flex items-center justify-between gap-3">
                    <span class="text-[11px] font-semibold uppercase tracking-[0.2em] text-subtle-foreground">
                      {{ message.role || 'message' }}
                    </span>
                  </div>
                  <pre class="mt-3 whitespace-pre-wrap break-words font-mono text-xs leading-6 text-foreground">{{ message.content }}</pre>
                </article>
              </div>

              <div v-else class="rounded-2xl border border-border/70 bg-surface-muted/20 p-4">
                <pre class="whitespace-pre-wrap break-words font-mono text-xs leading-6 text-foreground">{{ promptPayload }}</pre>
              </div>
            </section>

            <section class="space-y-2">
              <h3 class="text-sm font-semibold text-foreground">Attributes</h3>
              <div v-if="attributeEntries.length" class="space-y-2">
                <div
                  v-for="entry in attributeEntries"
                  :key="entry.key"
                  class="rounded-2xl border border-border/70 bg-surface-muted/20 p-4"
                >
                  <div class="text-[11px] font-semibold uppercase tracking-[0.2em] text-subtle-foreground">
                    {{ entry.key }}
                  </div>
                  <pre class="mt-2 whitespace-pre-wrap break-words font-mono text-xs leading-6 text-foreground">{{ entry.value }}</pre>
                </div>
              </div>
              <div
                v-else
                class="rounded-2xl border border-border/70 bg-surface-muted/20 p-4 text-sm text-faint-foreground"
              >
                No log attributes captured for this entry.
              </div>
            </section>

            <section class="space-y-2">
              <h3 class="text-sm font-semibold text-foreground">Resource Attributes</h3>
              <div v-if="resourceEntries.length" class="space-y-2">
                <div
                  v-for="entry in resourceEntries"
                  :key="entry.key"
                  class="rounded-2xl border border-border/70 bg-surface-muted/20 p-4"
                >
                  <div class="text-[11px] font-semibold uppercase tracking-[0.2em] text-subtle-foreground">
                    {{ entry.key }}
                  </div>
                  <pre class="mt-2 whitespace-pre-wrap break-words font-mono text-xs leading-6 text-foreground">{{ entry.value }}</pre>
                </div>
              </div>
              <div
                v-else
                class="rounded-2xl border border-border/70 bg-surface-muted/20 p-4 text-sm text-faint-foreground"
              >
                No resource attributes captured for this entry.
              </div>
            </section>
          </div>
        </div>
        </aside>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted } from "vue";
import { useQuery } from "@tanstack/vue-query";
import {
  fetchLogDetail,
  type LogDetail,
} from "@/api/client";
import type { MetricsTimeRangeValue } from "@/composables/observability/useTokenMetrics";

const props = defineProps<{
  open: boolean;
  logId?: string | null;
  window?: MetricsTimeRangeValue;
}>();

const emit = defineEmits<{
  close: [];
}>();

const { data, isLoading, isError } = useQuery({
  queryKey: computed(() => ["log-detail", props.logId, props.window]),
  queryFn: () => fetchLogDetail(props.logId || "", { window: props.window }),
  enabled: computed(() => props.open && Boolean(props.logId)),
  staleTime: 15_000,
});

const detail = computed(() => data.value?.log || null);

const formattedTimestamp = computed(() => {
  if (!detail.value) return "—";
  const raw = detail.value.timestamp;
  const millis = raw < 1_000_000_000_000 ? raw * 1000 : raw;
  return new Date(millis).toLocaleString();
});

const levelBadgeClass = computed(() => {
  const level = String(detail.value?.level || "info").toLowerCase();
  if (level === "error" || level === "fatal") {
    return "bg-danger/10 text-danger";
  }
  if (level === "warn" || level === "warning") {
    return "bg-warning/10 text-warning";
  }
  if (level === "info") {
    return "bg-info/10 text-info";
  }
  return "bg-muted/30 text-subtle-foreground";
});

const promptPayloadSource = computed(() => {
  const attrs = detail.value?.attributes || {};
  if (attrs.prompt_raw) return { key: "prompt_raw", value: attrs.prompt_raw };
  if (attrs.prompt) return { key: "prompt", value: attrs.prompt };
  return null;
});

const promptPayloadLabel = computed(() => {
  if (promptPayloadSource.value?.key === "prompt_raw") return "Raw";
  if (promptPayloadSource.value?.key === "prompt") return "Redacted";
  return "Prompt";
});

const promptPayload = computed(() => promptPayloadSource.value?.value || "");

const promptMessages = computed(() => parsePromptMessages(promptPayload.value));

const attributeEntries = computed(() =>
  toEntries(detail.value?.attributes).filter(
    (entry) => entry.key !== "prompt" && entry.key !== "prompt_raw",
  ),
);

const resourceEntries = computed(() => toEntries(detail.value?.resourceAttributes));

function closeDrawer() {
  emit("close");
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === "Escape" && props.open) {
    event.preventDefault();
    closeDrawer();
  }
}

onMounted(() => {
  window.addEventListener("keydown", handleKeydown);
});

onBeforeUnmount(() => {
  window.removeEventListener("keydown", handleKeydown);
});

function toEntries(values?: Record<string, string>) {
  return Object.entries(values || {})
    .map(([key, value]) => ({ key, value }))
    .sort((left, right) => left.key.localeCompare(right.key));
}

function parsePromptMessages(raw: string) {
  const trimmed = raw.trim();
  if (!trimmed) return [] as Array<{ role: string; content: string }>;

  try {
    const parsed = JSON.parse(trimmed);
    if (Array.isArray(parsed)) {
      return parsed
        .map((item) => normalizePromptMessage(item))
        .filter((item): item is { role: string; content: string } => Boolean(item));
    }
    if (
      parsed &&
      typeof parsed === "object" &&
      typeof parsed.preview === "string"
    ) {
      return [];
    }
  } catch {
    return [];
  }

  return [];
}

function normalizePromptMessage(value: unknown) {
  if (!value || typeof value !== "object") return null;
  const record = value as Record<string, unknown>;
  const role = typeof record.role === "string" ? record.role : "message";
  const content = formatPromptContent(record.content);
  if (!content) return null;
  return { role, content };
}

function formatPromptContent(value: unknown): string {
  if (typeof value === "string") return value;
  if (Array.isArray(value)) {
    return value
      .map((part) => {
        if (typeof part === "string") return part;
        if (part && typeof part === "object") {
          const text = (part as Record<string, unknown>).text;
          if (typeof text === "string") return text;
          return JSON.stringify(part, null, 2);
        }
        return "";
      })
      .filter(Boolean)
      .join("\n\n");
  }
  if (value && typeof value === "object") {
    return JSON.stringify(value, null, 2);
  }
  return value == null ? "" : String(value);
}
</script>

<style scoped>
/* Overlay fade */
.drawer-overlay-enter-active,
.drawer-overlay-leave-active {
  transition: opacity 0.25s ease;
}
.drawer-overlay-enter-from,
.drawer-overlay-leave-to {
  opacity: 0;
}

/* Panel slide from right */
.drawer-panel-enter-active,
.drawer-panel-leave-active {
  transition: transform 0.28s cubic-bezier(0.4, 0, 0.2, 1);
}
.drawer-panel-enter-from,
.drawer-panel-leave-to {
  transform: translateX(100%);
}
.drawer-panel-enter-to,
.drawer-panel-leave-from {
  transform: translateX(0);
}
</style>