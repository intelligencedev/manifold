<template>
  <Transition name="command-approval">
    <section
      v-if="activeItem"
      class="command-approval"
      role="alertdialog"
      aria-live="assertive"
      :aria-label="activeItem.request.question"
    >
      <div class="command-approval__heading">
        <span class="command-approval__signal" aria-hidden="true"></span>
        <span>Approve command</span>
        <span v-if="items.length > 1" class="command-approval__count">
          1 / {{ items.length }}
        </span>
        <span class="command-approval__agent">
          {{
            activeItem.request.agent || model.agentNameFor(activeItem.message)
          }}
        </span>
      </div>

      <p class="command-approval__question">
        {{ commandText }}
      </p>
      <p v-if="activeItem.request.reason" class="command-approval__reason">
        {{ activeItem.request.reason }}
      </p>

      <form @submit.prevent="submit">
        <div class="command-approval__choices">
          <label
            v-for="(choice, index) in activeItem.request.choices"
            :key="choice.id"
            class="command-approval__choice"
            :class="{
              'command-approval__choice--selected': selected(choice.id),
              'command-approval__choice--danger':
                choice.id === 'allow_all_session',
            }"
          >
            <input
              :type="activeItem.request.multiple ? 'checkbox' : 'radio'"
              :name="
                model.inputRequestFieldName(
                  activeItem.message,
                  activeItem.request,
                )
              "
              :checked="selected(choice.id)"
              :disabled="submitting"
              @change="choose(choice.id)"
            />
            <kbd>{{ index + 1 }}</kbd>
            <span>
              <strong>{{ choice.label }}</strong>
              <small v-if="choice.description">{{ choice.description }}</small>
            </span>
          </label>
        </div>

        <textarea
          v-if="activeItem.request.allowFreeText"
          class="command-approval__instructions"
          :value="
            model.inputRequestDraft(activeItem.message, activeItem.request)
          "
          rows="2"
          placeholder="Optional instructions…"
          :disabled="submitting"
          @input="
            model.setInputRequestDraft(
              activeItem.message,
              activeItem.request,
              ($event.target as HTMLTextAreaElement | null)?.value || '',
            )
          "
        ></textarea>

        <p v-if="errorMessage" class="command-approval__error">
          {{ errorMessage }}
        </p>

        <footer class="command-approval__footer">
          <span>
            Number to select
            <span aria-hidden="true">·</span>
            Enter to continue
          </span>
          <button type="submit" :disabled="!canSubmit || submitting">
            {{ submitting ? "Submitting…" : "Continue" }}
          </button>
        </footer>
      </form>
    </section>
  </Transition>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted } from "vue";
import type { ChatTranscriptModel } from "@/composables/chat/useChatViewController";
import type { ChatInputRequest, ChatMessage } from "@/types/chat";

type ApprovalItem = {
  message: ChatMessage;
  request: ChatInputRequest;
};

const props = defineProps<{
  items: ApprovalItem[];
  model: ChatTranscriptModel;
}>();

const activeItem = computed(() => props.items[0]);
const commandText = computed(() => {
  const question = activeItem.value?.request.question ?? "";
  return question.replace(/^Approve command execution:\s*/i, "");
});
const submitting = computed(() =>
  activeItem.value
    ? props.model.isInputRequestSubmitting(
        activeItem.value.message,
        activeItem.value.request,
      )
    : false,
);
const canSubmit = computed(() =>
  activeItem.value
    ? props.model.canSubmitInputRequest(
        activeItem.value.message,
        activeItem.value.request,
      )
    : false,
);
const errorMessage = computed(() => {
  if (!activeItem.value) return "";
  return (
    props.model.inputRequestLocalError(
      activeItem.value.message,
      activeItem.value.request,
    ) ||
    activeItem.value.request.error ||
    ""
  );
});

function selected(choiceID: string) {
  return activeItem.value
    ? props.model.inputRequestChoiceSelected(
        activeItem.value.message,
        activeItem.value.request,
        choiceID,
      )
    : false;
}

function choose(choiceID: string) {
  if (!activeItem.value) return;
  props.model.toggleInputRequestChoice(
    activeItem.value.message,
    activeItem.value.request,
    choiceID,
  );
}

function submit() {
  if (!activeItem.value || !canSubmit.value || submitting.value) return;
  void props.model.submitInputRequest(
    activeItem.value.message,
    activeItem.value.request,
  );
}

function isEditing(target: EventTarget | null) {
  const element = target instanceof HTMLElement ? target : null;
  return Boolean(
    element &&
    (element.isContentEditable ||
      ["INPUT", "TEXTAREA", "SELECT"].includes(element.tagName)),
  );
}

function onKeydown(event: KeyboardEvent) {
  if (!activeItem.value || submitting.value) return;
  const editing = isEditing(event.target);
  if (event.key === "Enter") {
    const modifiedSubmit =
      event.target instanceof HTMLTextAreaElement &&
      (event.metaKey || event.ctrlKey);
    if (editing && !modifiedSubmit) return;
    if (!canSubmit.value) return;
    event.preventDefault();
    submit();
    return;
  }
  if (editing || event.metaKey || event.ctrlKey || event.altKey) return;
  const choiceIndex = Number(event.key) - 1;
  if (!Number.isInteger(choiceIndex) || choiceIndex < 0) return;
  const choice = activeItem.value.request.choices[choiceIndex];
  if (!choice) return;
  event.preventDefault();
  choose(choice.id);
}

onMounted(() => window.addEventListener("keydown", onKeydown));
onBeforeUnmount(() => window.removeEventListener("keydown", onKeydown));
</script>

<style scoped>
.command-approval {
  width: min(760px, calc(100% - 2rem));
  margin: 0.5rem auto 0;
  overflow: hidden;
  border: 1px solid rgb(var(--color-accent) / 0.6);
  border-left-width: 3px;
  border-radius: 6px;
  background: rgb(var(--color-surface));
}

.command-approval__heading {
  display: flex;
  min-height: 32px;
  align-items: center;
  gap: 0.5rem;
  border-bottom: 1px solid rgb(var(--color-border));
  padding: 0 0.75rem;
  color: rgb(var(--color-accent));
  font-family: var(--font-mono);
  font-size: 0.62rem;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.command-approval__signal {
  width: 0.45rem;
  height: 0.45rem;
  border-radius: 50%;
  background: rgb(var(--color-accent));
}

.command-approval__count {
  border: 1px solid rgb(var(--color-border));
  border-radius: 999px;
  padding: 0.08rem 0.4rem;
  color: rgb(var(--color-muted-foreground));
}

.command-approval__agent {
  margin-left: auto;
  color: rgb(var(--color-faint-foreground));
}

.command-approval__question {
  margin: 0;
  overflow-x: auto;
  padding: 0.8rem 0.85rem 0.35rem;
  color: rgb(var(--color-foreground));
  font-family: var(--font-mono);
  font-size: 0.78rem;
  white-space: pre-wrap;
}

.command-approval__reason {
  padding: 0 0.85rem 0.55rem;
  color: rgb(var(--color-subtle-foreground));
  font-size: 0.72rem;
}

.command-approval__choices {
  display: grid;
  gap: 1px;
  margin: 0.35rem 0.75rem;
  overflow: hidden;
  border: 1px solid rgb(var(--color-border));
  border-radius: 4px;
  background: rgb(var(--color-border));
}

.command-approval__choice {
  display: grid;
  grid-template-columns: auto auto minmax(0, 1fr);
  align-items: center;
  gap: 0.55rem;
  min-height: 38px;
  cursor: pointer;
  background: rgb(var(--color-muted));
  padding: 0.45rem 0.65rem;
}

.command-approval__choice:hover,
.command-approval__choice--selected {
  background: rgb(var(--color-accent) / 0.1);
}

.command-approval__choice--danger {
  box-shadow: inset 2px 0 rgb(var(--color-danger));
}

.command-approval__choice input {
  accent-color: rgb(var(--color-accent));
}

.command-approval__choice kbd {
  min-width: 1.2rem;
  border: 1px solid rgb(var(--color-border));
  border-radius: 3px;
  color: rgb(var(--color-muted-foreground));
  font: 0.62rem var(--font-mono);
  text-align: center;
}

.command-approval__choice strong,
.command-approval__choice small {
  display: block;
}

.command-approval__choice strong {
  color: rgb(var(--color-foreground));
  font-size: 0.72rem;
}

.command-approval__choice small {
  margin-top: 0.12rem;
  color: rgb(var(--color-subtle-foreground));
  font-size: 0.64rem;
}

.command-approval__instructions {
  display: block;
  width: calc(100% - 1.5rem);
  margin: 0.55rem 0.75rem;
  resize: vertical;
  border: 1px solid rgb(var(--color-border));
  border-radius: 4px;
  background: rgb(var(--color-input));
  padding: 0.5rem 0.6rem;
  color: rgb(var(--color-foreground));
  font-size: 0.72rem;
  outline: none;
}

.command-approval__instructions:focus {
  border-color: rgb(var(--color-ring));
}

.command-approval__error {
  padding: 0 0.75rem;
  color: rgb(var(--color-danger));
  font-size: 0.7rem;
}

.command-approval__footer {
  display: flex;
  min-height: 38px;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  border-top: 1px solid rgb(var(--color-border));
  padding: 0.4rem 0.75rem;
  color: rgb(var(--color-faint-foreground));
  font: 0.62rem var(--font-mono);
}

.command-approval__footer button {
  height: 28px;
  border-radius: 4px;
  background: rgb(var(--color-accent));
  padding: 0 0.85rem;
  color: rgb(var(--color-accent-foreground));
  font-family: var(--font-text);
  font-weight: 700;
}

.command-approval__footer button:disabled {
  opacity: 0.45;
}

.command-approval-enter-active,
.command-approval-leave-active {
  transition:
    opacity 150ms ease,
    transform 150ms ease;
}

.command-approval-enter-from,
.command-approval-leave-to {
  opacity: 0;
  transform: translateY(5px);
}
</style>
