<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="fixed inset-0 z-50 flex justify-end bg-black/40"
      @click.self="$emit('close')"
    >
      <aside
        class="flex h-full w-full max-w-3xl flex-col border-l border-border bg-surface shadow-2xl"
        aria-label="Context Inspector"
      >
        <header class="flex items-start justify-between gap-4 border-b border-border px-5 py-4">
          <div>
            <h2 class="text-lg font-semibold text-foreground">Context Inspector</h2>
            <p class="mt-1 text-xs text-subtle-foreground">
              Redacted snapshot of the context sent to the LLM provider.
            </p>
          </div>
          <button
            type="button"
            class="rounded-4 border border-border px-2 py-1 text-xs text-subtle-foreground hover:text-foreground"
            @click="$emit('close')"
          >
            Close
          </button>
        </header>

        <div class="grid min-h-0 flex-1 grid-cols-[220px_minmax(0,1fr)]">
          <section class="min-h-0 border-r border-border p-3">
            <p v-if="loading" class="text-xs text-subtle-foreground">Loading requests…</p>
            <p v-else-if="!requests.length" class="text-xs text-subtle-foreground">
              No LLM request snapshots are available for this message.
            </p>
            <div v-else class="space-y-2">
              <button
                v-for="request in requests"
                :key="request.id"
                type="button"
                class="w-full rounded-4 border px-3 py-2 text-left text-xs transition"
                :class="request.id === selectedRequestId ? 'border-accent/70 bg-accent/10 text-foreground' : 'border-border bg-surface-muted/40 text-subtle-foreground hover:text-foreground'"
                @click="$emit('select-request', request.id)"
              >
                <span class="block truncate font-medium text-foreground">
                  {{ request.specialistId || 'orchestrator' }}
                </span>
                <span class="block truncate font-mono text-[11px]">{{ request.model }}</span>
                <span v-if="request.inputTokens" class="block text-[11px]">
                  {{ request.inputTokens.toLocaleString() }} input tokens
                </span>
              </button>
            </div>
          </section>

          <section class="min-h-0 overflow-y-auto p-5">
            <p v-if="error" class="mb-3 rounded border border-danger/50 bg-danger/10 px-3 py-2 text-xs text-danger">
              {{ error }}
            </p>
            <p v-if="contextLoading" class="text-xs text-subtle-foreground">Loading context…</p>
            <div v-else-if="selectedContext" class="space-y-5">
              <div class="grid grid-cols-2 gap-3 text-xs">
                <div class="rounded-4 border border-border bg-surface-muted/40 p-3">
                  <p class="text-faint-foreground">Model</p>
                  <p class="font-mono text-foreground">{{ selectedContext.model }}</p>
                </div>
                <div class="rounded-4 border border-border bg-surface-muted/40 p-3">
                  <p class="text-faint-foreground">Provider</p>
                  <p class="font-mono text-foreground">{{ selectedContext.provider || 'unknown' }}</p>
                </div>
                <div class="rounded-4 border border-border bg-surface-muted/40 p-3">
                  <p class="text-faint-foreground">Specialist</p>
                  <p class="font-mono text-foreground">{{ selectedContext.specialistId || 'orchestrator' }}</p>
                </div>
                <div class="rounded-4 border border-border bg-surface-muted/40 p-3">
                  <p class="text-faint-foreground">Input tokens</p>
                  <p class="font-mono text-foreground">{{ formatNumber(selectedContext.inputTokens) }}</p>
                </div>
              </div>

              <div class="flex flex-wrap gap-2">
                <button class="rounded-4 border border-border px-3 py-1.5 text-xs hover:text-accent" type="button" @click="copyRawJson">
                  {{ copied ? 'Copied' : 'Copy raw JSON' }}
                </button>
                <button class="rounded-4 border border-border px-3 py-1.5 text-xs hover:text-accent" type="button" @click="downloadRawJson">
                  Download JSON
                </button>
              </div>

              <div>
                <h3 class="mb-2 font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground">Messages</h3>
                <div class="space-y-2">
                  <details
                    v-for="(message, index) in messages"
                    :key="index"
                    class="rounded-4 border border-border bg-surface-muted/40 p-3 text-xs"
                    open
                  >
                    <summary class="cursor-pointer font-mono text-[11px] uppercase text-subtle-foreground">
                      {{ roleFor(message) }} · {{ contentLength(message).toLocaleString() }} chars
                    </summary>
                    <pre class="mt-2 max-h-72 overflow-auto whitespace-pre-wrap break-words text-[11px] leading-relaxed text-foreground">{{ contentFor(message) }}</pre>
                  </details>
                </div>
              </div>

              <div>
                <h3 class="mb-2 font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground">Raw JSON</h3>
                <pre class="max-h-[420px] overflow-auto rounded-4 border border-border bg-surface-muted/40 p-3 text-[11px] text-foreground">{{ rawJson }}</pre>
              </div>
            </div>
          </section>
        </div>
      </aside>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import type { ChatLLMRequestContext, ChatLLMRequestSummary } from "@/types/chat";

const props = defineProps<{
  open: boolean;
  loading: boolean;
  contextLoading: boolean;
  requests: ChatLLMRequestSummary[];
  selectedRequestId: string;
  selectedContext: ChatLLMRequestContext | null;
  error: string;
}>();

defineEmits<{
  close: [];
  "select-request": [requestId: string];
}>();

const copied = ref(false);
const rawJson = computed(() => JSON.stringify(props.selectedContext?.payload ?? {}, null, 2));
const messages = computed(() => {
  const payload = props.selectedContext?.payload as { messages?: unknown } | undefined;
  return Array.isArray(payload?.messages) ? payload.messages : [];
});

function formatNumber(value?: number) {
  return value ? value.toLocaleString() : "—";
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" ? (value as Record<string, unknown>) : {};
}

function roleFor(value: unknown) {
  return String(asRecord(value).role || "message");
}

function contentFor(value: unknown) {
  const record = asRecord(value);
  if (typeof record.content === "string") return record.content;
  return JSON.stringify(record, null, 2);
}

function contentLength(value: unknown) {
  return contentFor(value).length;
}

async function copyRawJson() {
  await navigator.clipboard?.writeText(rawJson.value);
  copied.value = true;
  setTimeout(() => (copied.value = false), 1200);
}

function downloadRawJson() {
  const blob = new Blob([rawJson.value], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `${props.selectedRequestId || "llm-request"}.json`;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 0);
}
</script>
