<template>
  <header
    class="chat-pane-header flex flex-wrap items-center justify-between gap-3 px-4 pb-4 pt-1"
  >
    <div class="min-w-0 flex-1">
      <p class="chat-panel-kicker">Conversation</p>
      <h1 class="truncate text-base font-semibold text-foreground">
        {{ model.activeSession?.name ?? "Conversation" }}
      </h1>
    </div>
    <div class="flex items-center gap-2 text-xs text-subtle-foreground">
      <span
        v-if="model.activeSummaryEvent"
        class="flex items-center gap-1.5 rounded-full bg-warning/10 border border-warning/30 px-2.5 py-1 text-warning transition-all duration-300 dark:bg-warning/20 dark:text-warning-foreground"
        :title="`Summarized ${model.activeSummaryEvent.summarizedCount} of ${model.activeSummaryEvent.messageCount} messages (${model.activeSummaryEvent.inputTokens.toLocaleString()} tokens exceeded ${model.activeSummaryEvent.tokenBudget.toLocaleString()} budget)`"
      >
        <svg
          xmlns="http://www.w3.org/2000/svg"
          class="h-3 w-3"
          viewBox="0 0 20 20"
          fill="currentColor"
        >
          <path
            fill-rule="evenodd"
            d="M4 4a2 2 0 012-2h4.586A2 2 0 0112 2.586L15.414 6A2 2 0 0116 7.414V16a2 2 0 01-2 2H6a2 2 0 01-2-2V4zm2 6a1 1 0 011-1h6a1 1 0 110 2H7a1 1 0 01-1-1zm1 3a1 1 0 100 2h6a1 1 0 100-2H7z"
            clip-rule="evenodd"
          />
        </svg>
        <span class="font-medium">Context summarized</span>
        <button
          type="button"
          class="ml-0.5 rounded-full p-0.5 transition hover:bg-warning/20 dark:hover:bg-warning/30"
          title="Dismiss"
          @click.stop="model.clearSummaryEvent()"
        >
          <svg
            xmlns="http://www.w3.org/2000/svg"
            class="h-3 w-3"
            viewBox="0 0 20 20"
            fill="currentColor"
          >
            <path
              fill-rule="evenodd"
              d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
              clip-rule="evenodd"
            />
          </svg>
        </button>
      </span>
      <span
        v-if="model.commandPolicyAllowAllActive"
        class="flex items-center gap-1.5 rounded-full border border-warning/40 bg-warning/10 px-2.5 py-1 text-warning"
        title="Command approvals are skipped for this session unless policy explicitly denies a command."
      >
        <span class="h-1.5 w-1.5 rounded-full bg-warning"></span>
        <span class="font-medium">All commands allowed</span>
        <button
          type="button"
          class="ml-0.5 rounded-full border border-warning/40 px-1.5 py-0.5 text-[10px] font-semibold text-warning transition hover:bg-warning/15 disabled:cursor-not-allowed disabled:opacity-60"
          :disabled="model.commandPolicyDisablePending"
          @click.stop="model.disableSessionCommandPolicyAllowAll"
        >
          {{ model.commandPolicyDisablePending ? "Disabling..." : "Disable" }}
        </button>
      </span>
      <span
        v-if="model.commandPolicyDisableError"
        class="rounded-full border border-danger/40 bg-danger/10 px-2.5 py-1 text-danger"
      >
        {{ model.commandPolicyDisableError }}
      </span>
      <div class="flex items-center gap-3" aria-label="Conversation settings">
        <div class="flex items-center gap-1.5">
          <span class="text-[11px] font-medium text-subtle-foreground">
            Project
          </span>
          <DropdownSelect
            :model-value="model.selectedProjectId"
            :options="model.projectOptions"
            size="xs"
            title="Project for this conversation"
            aria-label="Project"
            class="min-w-[160px]"
            @update:model-value="model.setSelectedProjectId"
          />
        </div>
        <span class="h-4 w-px bg-border/60" aria-hidden="true"></span>
        <div class="flex items-center gap-3" aria-label="Memory controls">
          <label
            class="inline-flex cursor-pointer items-center gap-1.5 text-[11px] font-medium leading-none text-subtle-foreground"
            title="Enable unified evolving, belief, and MAGMA memory for this conversation"
          >
            <input
              type="checkbox"
              class="sr-only"
              :checked="model.memoryEnabled"
              :disabled="
                !model.activeSession?.id ||
                model.isStreaming ||
                model.activeMemorySettingsSaving
              "
              aria-label="Memory"
              @change="model.setSessionMemorySetting($event)"
            />
            <span
              class="relative h-4 w-7 rounded-full border transition-colors"
              :class="
                model.memoryEnabled
                  ? 'border-accent bg-accent'
                  : 'border-border bg-surface'
              "
            >
              <span
                class="absolute top-0.5 h-2.5 w-2.5 rounded-full bg-background shadow-1 transition-transform"
                :class="
                  model.memoryEnabled ? 'translate-x-3.5' : 'translate-x-0.5'
                "
              ></span>
            </span>
            <span>Memory</span>
          </label>
        </div>
      </div>
    </div>
  </header>
</template>

<script setup lang="ts">
import DropdownSelect from "@/components/DropdownSelect.vue";
import type { ChatHeaderPanelModel } from "@/composables/chat/useChatViewController";

defineProps<{
  model: ChatHeaderPanelModel;
}>();
</script>
