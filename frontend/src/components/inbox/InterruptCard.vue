<template>
  <div
    class="group relative rounded-xl border bg-surface transition-colors"
    :class="active ? 'border-accent/40 bg-accent/5' : 'border-border hover:border-border/80'"
    role="article"
    :aria-label="`Input request: ${title}`"
  >
    <!-- Header -->
    <div class="flex items-start gap-3 p-4">
      <div class="flex-1 min-w-0">
        <div class="flex items-center gap-2 mb-1">
          <Badge variant="warning">input?</Badge>
          <span class="font-mono text-[10px] text-white/30 truncate">{{ requestId }}</span>
        </div>
        <h3 class="text-sm font-medium text-foreground leading-snug">{{ title }}</h3>
        <p v-if="reason" class="mt-1 text-xs text-white/55 leading-relaxed">{{ reason }}</p>
        <div v-if="agent" class="mt-1.5 flex items-center gap-1.5 text-[11px] text-white/35">
          <span>from</span>
          <Badge variant="accent">{{ agent }}</Badge>
          <span v-if="depth">depth {{ depth }}</span>
        </div>
      </div>
      <div class="shrink-0 text-[10px] text-white/25">{{ reltime }}</div>
    </div>

    <!-- Choices -->
    <div v-if="choices && choices.length" class="border-t border-border/40 px-4 py-3">
      <div class="mb-2 text-[10px] uppercase tracking-wider text-white/35">Choose one</div>
      <div class="flex flex-wrap gap-2">
        <button
          v-for="choice in choices"
          :key="choice.id ?? choice.label"
          class="rounded-lg border px-3 py-1.5 text-xs transition-colors"
          :class="selectedChoiceId === (choice.id ?? choice.label)
            ? 'border-accent bg-accent/20 text-accent'
            : 'border-border/60 bg-black/10 text-white/60 hover:border-border hover:text-white/80'"
          @click="selectChoice(choice.id ?? choice.label ?? '')"
        >
          {{ choice.label || choice.id }}
          <span v-if="choice.description" class="ml-1 text-white/35">({{ choice.description }})</span>
        </button>
      </div>
    </div>

    <!-- Free-text + actions -->
    <div v-if="allowFreeText || choices?.length" class="border-t border-border/40 px-4 py-3">
      <textarea
        v-if="allowFreeText"
        v-model="answer"
        class="mb-3 w-full resize-none rounded-lg border border-border bg-black/15 px-3 py-2 text-sm text-foreground placeholder:text-white/25 focus:border-accent/40 focus:outline-none focus:ring-1 focus:ring-accent/20"
        rows="2"
        placeholder="Type your answer…"
        @keydown.meta.enter="submit"
        @keydown.ctrl.enter="submit"
      />
      <div class="flex items-center justify-between">
        <div class="text-[10px] text-white/25">⌘↵ submit · Esc dismiss</div>
        <div class="flex gap-2">
          <Button size="sm" variant="ghost" :disabled="loading" @click="$emit('dismiss')">Dismiss</Button>
          <Button
            size="sm"
            variant="primary"
            :loading="loading"
            :disabled="!canSubmit"
            @click="submit"
          >Approve</Button>
        </div>
      </div>
    </div>

    <!-- Error -->
    <ErrorBanner v-if="error" :message="error" class="mx-4 mb-3" @dismiss="error = ''"/>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { answerInputRequest } from "@/api/chat";
import Badge from "@/components/ui/Badge.vue";
import Button from "@/components/ui/Button.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";

interface Choice { id?: string; label?: string; description?: string; }

const props = defineProps<{
  requestId: string;
  title: string;
  reason?: string;
  agent?: string;
  depth?: number;
  choices?: Choice[];
  allowFreeText?: boolean;
  multiple?: boolean;
  active?: boolean;
  createdAt?: string;
}>();

const emit = defineEmits<{
  answered: [requestId: string];
  dismiss: [];
}>();

const answer = ref("");
const selectedChoiceId = ref("");
const loading = ref(false);
const error = ref("");

const canSubmit = computed(() => {
  if (loading.value) return false;
  if (props.allowFreeText && answer.value.trim()) return true;
  if (props.choices?.length && selectedChoiceId.value) return true;
  if (!props.allowFreeText && !props.choices?.length) return true;
  return false;
});

function selectChoice(id: string) {
  selectedChoiceId.value = selectedChoiceId.value === id ? "" : id;
}

async function submit() {
  if (!canSubmit.value) return;
  loading.value = true;
  error.value = "";
  try {
    await answerInputRequest(props.requestId, {
      answer: answer.value.trim(),
      choice_ids: selectedChoiceId.value ? [selectedChoiceId.value] : [],
    });
    emit("answered", props.requestId);
  } catch (err: unknown) {
    error.value = err instanceof Error ? err.message : "Failed to submit answer";
  } finally {
    loading.value = false;
  }
}

const reltime = computed(() => {
  if (!props.createdAt) return "";
  const diff = Date.now() - new Date(props.createdAt).getTime();
  if (diff < 60_000) return `${Math.round(diff / 1000)}s ago`;
  if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m ago`;
  return new Date(props.createdAt).toLocaleTimeString();
});
</script>
