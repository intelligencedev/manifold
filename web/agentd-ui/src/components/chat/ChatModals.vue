<template>
  <div
    v-if="model.showImageModal && model.modalImage"
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4"
    @click.self="model.closeImageModal"
  >
    <div
      class="relative max-h-[90vh] max-w-[90vw] rounded-5 bg-surface p-4 ring-1 ring-border/60"
    >
      <button
        type="button"
        class="absolute right-3 top-3 rounded-full bg-surface-muted px-2 py-1 text-sm text-foreground shadow hover:bg-surface"
        @click="model.closeImageModal"
      >
        ×
      </button>
      <div class="flex flex-col items-center gap-3">
        <img
          :src="model.modalImageSrc"
          :alt="model.modalImage.name"
          class="max-h-[70vh] max-w-[80vw] rounded border border-border object-contain"
        />
        <div class="text-center text-xs text-subtle-foreground">
          <p class="font-semibold text-foreground">{{ model.modalImage.name }}</p>
          <p v-if="model.modalImage.path">Saved at: {{ model.modalImage.path }}</p>
        </div>
      </div>
    </div>
  </div>

  <div
    v-if="model.showDeleteSessionDialog && model.deleteSessionTarget"
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4"
    role="dialog"
    aria-modal="true"
    aria-labelledby="delete-session-title"
    @click.self="model.closeDeleteSessionDialog"
    @keydown.esc.prevent="model.closeDeleteSessionDialog"
  >
    <div class="w-full max-w-md rounded-5 bg-surface p-5 ring-1 ring-border/60">
      <h2 id="delete-session-title" class="text-base font-semibold text-danger">
        Delete Conversation
      </h2>
      <p class="mt-2 text-sm text-subtle-foreground">
        This permanently removes
        <span class="font-semibold text-foreground">{{
          model.deleteSessionTarget.name
        }}</span>
        and all messages in it.
      </p>
      <form class="mt-4 space-y-3" @submit.prevent="model.confirmDeleteSession">
        <p class="text-xs text-faint-foreground">This action cannot be undone.</p>
        <p v-if="model.deleteSessionError" class="text-xs text-danger">
          {{ model.deleteSessionError }}
        </p>
        <div class="flex items-center justify-end gap-2">
          <button
            type="button"
            class="h-9 rounded-full border border-white/15 px-3 text-sm text-subtle-foreground transition hover:border-white/30 hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60"
            :disabled="model.deleteSessionPending"
            @click="model.closeDeleteSessionDialog"
          >
            Cancel
          </button>
          <button
            type="submit"
            class="h-9 rounded-full border border-danger/60 bg-danger/10 px-3 text-sm font-semibold text-danger transition hover:bg-danger/20 disabled:cursor-not-allowed disabled:opacity-60"
            :disabled="!model.canConfirmDeleteSession"
          >
            {{ model.deleteSessionPending ? "Deleting..." : "Delete Conversation" }}
          </button>
        </div>
      </form>
    </div>
  </div>

  <div
    v-if="model.showBulkDeleteSessionDialog"
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4"
    role="dialog"
    aria-modal="true"
    aria-labelledby="bulk-delete-session-title"
    @click.self="model.closeBulkDeleteSessionDialog"
    @keydown.esc.prevent="model.closeBulkDeleteSessionDialog"
  >
    <div class="w-full max-w-md rounded-5 bg-surface p-5 ring-1 ring-border/60">
      <h2 id="bulk-delete-session-title" class="text-base font-semibold text-danger">
        Delete Conversations
      </h2>
      <p class="mt-2 text-sm text-subtle-foreground">
        This permanently removes
        <span class="font-semibold text-foreground">{{ model.bulkDeleteSessionCount }}</span>
        conversation{{ model.bulkDeleteSessionCount === 1 ? '' : 's' }} and all messages in them.
      </p>
      <form class="mt-4 space-y-3" @submit.prevent="model.confirmBulkDeleteSession">
        <p class="text-xs text-faint-foreground">This action cannot be undone.</p>
        <p v-if="model.bulkDeleteSessionError" class="text-xs text-danger">
          {{ model.bulkDeleteSessionError }}
        </p>
        <div class="flex items-center justify-end gap-2">
          <button
            type="button"
            class="h-9 rounded-full border border-white/15 px-3 text-sm text-subtle-foreground transition hover:border-white/30 hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60"
            :disabled="model.bulkDeleteSessionPending"
            @click="model.closeBulkDeleteSessionDialog"
          >
            Cancel
          </button>
          <button
            type="submit"
            class="h-9 rounded-full border border-danger/60 bg-danger/10 px-3 text-sm font-semibold text-danger transition hover:bg-danger/20 disabled:cursor-not-allowed disabled:opacity-60"
            :disabled="!model.canConfirmBulkDeleteSession"
          >
            {{ model.bulkDeleteSessionPending ? "Deleting..." : `Delete ${model.bulkDeleteSessionCount} Conversation${model.bulkDeleteSessionCount === 1 ? '' : 's'}` }}
          </button>
        </div>
      </form>
    </div>
  </div>

  <div
    v-if="model.selectedParticipantActivityName"
    class="activity-modal-backdrop"
    role="dialog"
    aria-modal="true"
    :aria-label="`${model.selectedParticipantActivity?.name || 'Specialist'} activity`"
    @click.self="model.closeParticipantActivity"
  >
    <div class="activity-modal">
      <header class="activity-modal-header">
        <div class="min-w-0">
          <p class="activity-modal-title">
            {{ model.selectedParticipantActivity?.name || "Specialist activity" }}
          </p>
          <p class="activity-modal-model">
            {{ model.selectedParticipantActivity?.model || "Model pending" }}
          </p>
        </div>
        <button
          type="button"
          class="activity-modal-close"
          aria-label="Close specialist activity"
          @click="model.closeParticipantActivity"
        >
          Close
        </button>
      </header>

      <div
        :ref="model.setParticipantActivityPaneRef"
        class="activity-detail-scroll activity-modal-scroll"
        @scroll="model.handleActivityPaneScroll"
      >
        <section
          v-for="item in model.selectedParticipantActivityItems"
          :key="item.id"
          class="activity-detail-section"
        >
          <div v-if="item.toolEntries.length">
            <h3 class="activity-detail-section-title">Tool activity</h3>
            <ul class="activity-tool-list">
              <li
                v-for="entry in item.toolEntries"
                :key="entry.id"
                class="activity-tool-item"
              >
                <p class="activity-tool-title">
                  {{ entry.title || "Tool" }}
                </p>
              </li>
            </ul>
          </div>

          <div v-if="item.thoughtSummaries.length" class="activity-detail-subsection">
            <h3 class="activity-detail-section-title">Thought summaries</h3>
            <ul class="activity-thought-list text-foreground">
              <li
                v-for="(summary, idx) in item.thoughtSummaries"
                :key="`${item.id}:summary:${idx}:${summary}`"
                class="activity-thought-item"
              >
                <div
                  class="chat-markdown min-w-0 flex-1 break-words"
                  v-html="model.renderMarkdownOrHtml(summary)"
                ></div>
              </li>
            </ul>
          </div>

          <div v-if="item.response" class="activity-detail-subsection">
            <h3 class="activity-detail-section-title">Response stream</h3>
            <div
              class="chat-markdown activity-response"
              v-html="model.renderMarkdownOrHtml(item.response)"
            ></div>
          </div>

          <div v-if="item.error" class="activity-detail-subsection">
            <h3 class="activity-detail-section-title">Error</h3>
            <p class="activity-error-text">
              {{ item.error }}
            </p>
          </div>

          <div
            v-if="
              !item.toolEntries.length &&
              !item.thoughtSummaries.length &&
              !item.response &&
              !item.error
            "
            class="activity-detail-empty"
          >
            No activity details yet.
          </div>
        </section>

        <div
          v-if="!model.selectedParticipantActivityItems.length"
          class="activity-detail-empty"
        >
          No specialist activity yet.
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ChatModalsModel } from "@/composables/chat/useChatViewController";

defineProps<{
  model: ChatModalsModel;
}>();
</script>
