<template>
  <form
    class="input-request-card"
    :class="model.inputRequestCardClasses(request)"
    @submit.prevent="model.submitInputRequest(message, request)"
  >
    <div class="input-request-header">
      <div class="min-w-0">
        <p class="input-request-kicker">
          {{ model.inputRequestStatusLabel(request) }}
        </p>
        <p class="input-request-agent">
          {{ request.agent || model.agentNameFor(message) }}
        </p>
      </div>
      <span
        v-if="request.status === 'pending'"
        class="input-request-live-dot"
      ></span>
    </div>
    <p class="input-request-question">
      {{ request.question }}
    </p>
    <p v-if="request.reason" class="input-request-reason">
      {{ request.reason }}
    </p>

    <div
      v-if="request.choices.length && model.isInputRequestRespondable(request)"
      class="input-request-choices"
    >
      <label
        v-for="choice in request.choices"
        :key="choice.id"
        class="input-request-choice"
      >
        <input
          :type="request.multiple ? 'checkbox' : 'radio'"
          :name="model.inputRequestFieldName(message, request)"
          :checked="
            model.inputRequestChoiceSelected(message, request, choice.id)
          "
          :disabled="model.isInputRequestSubmitting(message, request)"
          @change="model.toggleInputRequestChoice(message, request, choice.id)"
        />
        <span class="min-w-0">
          <span class="input-request-choice-label">
            {{ choice.label }}
          </span>
          <span
            v-if="choice.description"
            class="input-request-choice-description"
          >
            {{ choice.description }}
          </span>
        </span>
      </label>
    </div>

    <textarea
      v-if="request.allowFreeText && model.isInputRequestRespondable(request)"
      :value="model.inputRequestDraft(message, request)"
      class="input-request-textarea"
      rows="3"
      placeholder="Tell the model what to do..."
      :disabled="model.isInputRequestSubmitting(message, request)"
      @input="
        model.setInputRequestDraft(
          message,
          request,
          ($event.target as HTMLTextAreaElement | null)?.value || '',
        )
      "
    ></textarea>

    <p
      v-if="model.inputRequestLocalError(message, request) || request.error"
      class="input-request-error"
    >
      {{ model.inputRequestLocalError(message, request) || request.error }}
    </p>

    <div v-if="request.status === 'answered'" class="input-request-answer">
      <span class="input-request-answer-label">Answered</span>
      <span class="input-request-answer-text">
        {{ model.inputRequestAnswerSummary(request) }}
      </span>
    </div>

    <div
      v-if="model.isInputRequestRespondable(request)"
      class="input-request-actions"
    >
      <button
        type="submit"
        class="input-request-submit"
        :disabled="!model.canSubmitInputRequest(message, request)"
      >
        {{
          model.isInputRequestSubmitting(message, request)
            ? "Submitting..."
            : "Continue"
        }}
      </button>
    </div>
  </form>
</template>

<script setup lang="ts">
import type { ChatTranscriptModel } from "@/composables/chat/useChatViewController";
import type { ChatInputRequest, ChatMessage } from "@/types/chat";

defineProps<{
  message: ChatMessage;
  request: ChatInputRequest;
  model: ChatTranscriptModel;
}>();
</script>
