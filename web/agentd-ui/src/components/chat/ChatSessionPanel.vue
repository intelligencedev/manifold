<template>
  <aside
    class="chat-session-panel flex h-full min-h-0 flex-col text-sm text-subtle-foreground chat-side"
    aria-label="Session"
  >
    <div class="flex min-h-0 flex-1 flex-col">
      <GlassCard
        flat
        :padded="false"
        class="flex min-h-0 flex-1 flex-col overflow-hidden"
      >
        <div class="session-column-content flex min-h-0 flex-1 flex-col gap-2.5">
          <header class="session-column-header">
            <p class="chat-panel-kicker">Session</p>
            <div
              class="session-conversation-controls"
              aria-label="Conversation controls"
            >
              <div
                v-if="model.renamingSessionId === model.activeSessionId && model.activeSession"
                class="w-full"
              >
                <input
                  :ref="model.setRenameInput"
                  :value="model.renamingName"
                  type="text"
                  class="session-conversation-input"
                  aria-label="Conversation name"
                  @input="
                    model.setRenamingName(
                      (($event.target as HTMLInputElement | null)?.value || ''),
                    )
                  "
                  @keyup.enter.prevent="model.commitRename(model.activeSession.id)"
                  @keyup.esc.prevent="model.cancelRename"
                  @blur="model.commitRename(model.activeSession.id)"
                />
              </div>
              <DropdownSelect
                v-else
                :model-value="model.activeSessionId || ''"
                :options="model.conversationOptions"
                :disabled="model.sessionsLoading || !model.sessions.length"
                size="sm"
                title="Select conversation"
                aria-label="Select conversation"
                class="w-full"
                @update:model-value="model.selectSession"
              />
              <div class="session-conversation-actions">
                <button
                  type="button"
                  class="session-action-button session-action-button--primary"
                  @click="model.createSession()"
                >
                  New
                </button>
                <template v-if="model.activeSession">
                  <button
                    type="button"
                    class="session-action-button"
                    @click="model.startRename(model.activeSession)"
                  >
                    Rename
                  </button>
                  <button
                    type="button"
                    class="session-icon-button"
                    :title="
                      model.activeSession.pinned
                        ? `Unpin conversation ${model.activeSession.name}`
                        : `Pin conversation ${model.activeSession.name}`
                    "
                    :aria-label="
                      model.activeSession.pinned
                        ? `Unpin conversation ${model.activeSession.name}`
                        : `Pin conversation ${model.activeSession.name}`
                    "
                    :aria-pressed="Boolean(model.activeSession.pinned)"
                    :disabled="model.sessionPinPending(model.activeSession.id)"
                    @click="model.toggleSessionPinned(model.activeSession)"
                  >
                    <SolarPinBold class="h-3.5 w-3.5" />
                  </button>
                  <button
                    type="button"
                    class="session-icon-button"
                    title="Export conversation"
                    aria-label="Export conversation"
                    @click="model.exportSession(model.activeSession.id)"
                  >
                    <SolarDownloadIcon class="h-3.5 w-3.5" />
                  </button>
                  <button
                    type="button"
                    class="session-icon-button session-icon-button--danger"
                    :title="`Delete conversation ${model.activeSession.name}`"
                    :aria-label="`Delete conversation ${model.activeSession.name}`"
                    @click="model.openDeleteSessionDialog(model.activeSession)"
                  >
                    <SolarTrashIcon class="h-3.5 w-3.5" />
                  </button>
                </template>
              </div>
            </div>
          </header>
          <div class="cockpit-inspector-stack">
            <section class="cockpit-inspector-card" aria-label="Context">
              <div
                class="cockpit-context-ring"
                :style="{ '--context-used': model.cockpitContextDegrees }"
              >
                <div>
                  <strong>{{ model.cockpitContextPercent }}%</strong>
                  <span>Context used</span>
                </div>
              </div>
              <div class="cockpit-readout-list">
                <div class="cockpit-readout-row">
                  <span>Context window</span>
                  <strong>{{ model.cockpitContextLabel }}</strong>
                </div>
              </div>
            </section>
          </div>

          <section class="session-tool-invocations" aria-label="Tool invocations">
            <header class="session-tool-invocations-header">
              <div>
                <p class="chat-panel-kicker">Tool Invocations</p>
                <p class="session-tool-invocations-summary">
                  {{ model.cockpitToolCount.toLocaleString() }} total
                </p>
              </div>
            </header>
            <div
              v-if="model.cockpitToolRows.length"
              class="session-tool-invocation-list"
            >
              <details
                v-for="row in model.cockpitToolRows"
                :key="row.id"
                class="session-tool-invocation"
              >
                <summary class="session-tool-invocation-summary-row">
                  <span
                    class="cockpit-tool-glyph"
                    :class="`cockpit-tool-glyph--${row.statusTone}`"
                    >⌘</span
                  >
                  <span class="session-tool-invocation-title-block">
                    <span class="cockpit-tool-title">{{ row.name }}</span>
                  </span>
                  <span class="cockpit-tool-state">
                    <span
                      class="cockpit-status-dot"
                      :class="`cockpit-status-dot--${row.statusTone}`"
                    ></span>
                    {{ row.status }}
                  </span>
                </summary>
                <div class="session-tool-invocation-details">
                  <div v-if="row.args" class="session-tool-detail-block">
                    <span class="session-tool-detail-label">Arguments</span>
                    <pre>{{ row.args }}</pre>
                  </div>
                  <div v-if="row.output" class="session-tool-detail-block">
                    <span class="session-tool-detail-label">Result</span>
                    <pre>{{ row.output }}</pre>
                  </div>
                  <p v-if="!row.args && !row.output" class="cockpit-empty-text">
                    No details recorded for this invocation.
                  </p>
                </div>
              </details>
            </div>
            <p v-else class="cockpit-empty-text">
              No tool calls recorded for this conversation.
            </p>
          </section>
        </div>
      </GlassCard>
    </div>
  </aside>
</template>

<script setup lang="ts">
import DropdownSelect from "@/components/DropdownSelect.vue";
import SolarDownloadIcon from "@/components/icons/SolarDownload.vue";
import SolarPinBold from "@/components/icons/SolarPinBold.vue";
import SolarTrashIcon from "@/components/icons/SolarTrash.vue";
import GlassCard from "@/components/ui/GlassCard.vue";
import type { ChatSessionPanelModel } from "@/composables/chat/useChatViewController";

defineProps<{
  model: ChatSessionPanelModel;
}>();
</script>
