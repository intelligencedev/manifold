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
              <div v-else class="session-dropdown-wrapper" ref="dropdownWrapperRef">
                <button
                  type="button"
                  class="session-dropdown-trigger"
                  :disabled="model.sessionsLoading || !model.sessions.length"
                  :title="model.activeSession?.name ?? 'Select conversation'"
                  aria-label="Select conversation"
                  @click="model.toggleSessionDropdown()"
                >
                  <span class="session-dropdown-trigger-label">
                    {{ model.activeSession?.name ?? 'Select conversation' }}
                  </span>
                  <span
                    v-if="model.checkedSessionCount > 0"
                    class="session-dropdown-trigger-count"
                  >
                    {{ model.checkedSessionCount }} selected
                  </span>
                  <span class="session-dropdown-trigger-chevron">
                    {{ model.sessionDropdownOpen ? '▲' : '▼' }}
                  </span>
                </button>

                <div
                  v-if="model.sessionDropdownOpen"
                  class="session-dropdown-menu"
                  @click.stop
                >
                  <div class="session-dropdown-header">
                    <label class="session-dropdown-select-all">
                      <input
                        type="checkbox"
                        class="session-checkbox"
                        :checked="model.allSessionsChecked"
                        @change="model.toggleSelectAll()"
                      />
                      <span>Select all</span>
                    </label>
                    <button
                      type="button"
                      class="session-dropdown-delete-btn"
                      :disabled="model.checkedSessionCount === 0"
                      :title="`Delete ${model.checkedSessionCount} selected conversation${model.checkedSessionCount === 1 ? '' : 's'}`"
                      @click="handleBulkDelete"
                    >
                      <SolarTrashIcon class="h-3.5 w-3.5" />
                      <span v-if="model.checkedSessionCount > 0">
                        Delete ({{ model.checkedSessionCount }})
                      </span>
                      <span v-else>Delete</span>
                    </button>
                  </div>
                  <div class="session-dropdown-list">
                    <div
                      v-for="session in model.sessions"
                      :key="session.id"
                      class="session-dropdown-item"
                      :class="{
                        'session-dropdown-item--active': session.id === model.activeSessionId,
                        'session-dropdown-item--checked': model.isSessionChecked(session.id),
                      }"
                    >
                      <input
                        type="checkbox"
                        class="session-checkbox"
                        :checked="model.isSessionChecked(session.id)"
                        @click.stop="model.toggleSessionChecked(session.id)"
                      />
                      <span
                        class="session-dropdown-item-label"
                        @click.stop="handleSessionClick(session.id)"
                      >
                        <span v-if="session.pinned" class="session-dropdown-pin">★</span>
                        {{ session.name }}
                      </span>
                      <span class="session-dropdown-item-count">
                        {{ model.messageCountFor?.(session.id) ?? 0 }}
                      </span>
                    </div>
                  </div>
                </div>
              </div>

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
                :style="{
                  '--context-used': model.cockpitContextDegrees,
                  '--context-gradient': model.cockpitContextGradient,
                }"
                :title="model.cockpitContextTitle"
                role="meter"
                :aria-valuenow="model.cockpitContextPercent"
                aria-valuemin="0"
                aria-valuemax="100"
                aria-label="Context window usage by context kind"
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
              <div
                v-if="model.cockpitContextLegend.length"
                class="cockpit-context-legend"
                aria-label="Context kind color legend"
              >
                <div
                  v-for="segment in model.cockpitContextLegend"
                  :key="segment.id"
                  class="cockpit-context-legend-row"
                >
                  <span
                    class="cockpit-context-legend-swatch"
                    :style="{ background: segment.color }"
                    aria-hidden="true"
                  ></span>
                  <span class="cockpit-context-legend-label">
                    {{ segment.label }}
                  </span>
                  <span class="cockpit-context-legend-value">
                    {{ segment.tokenLabel }} · {{ segment.percentLabel }}
                  </span>
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
                  >⌘</span>
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
import { onBeforeUnmount, onMounted, ref } from "vue";
import SolarDownloadIcon from "@/components/icons/SolarDownload.vue";
import SolarPinBold from "@/components/icons/SolarPinBold.vue";
import SolarTrashIcon from "@/components/icons/SolarTrash.vue";
import GlassCard from "@/components/ui/GlassCard.vue";
import type { ChatSessionPanelModel } from "@/composables/chat/useChatViewController";

const props = defineProps<{
  model: ChatSessionPanelModel;
}>();

const dropdownWrapperRef = ref<HTMLElement | null>(null);

function handleClickOutside(event: MouseEvent) {
  if (!dropdownWrapperRef.value) return;
  if (!dropdownWrapperRef.value.contains(event.target as Node)) {
    props.model.closeSessionDropdown();
  }
}

onMounted(() => {
  document.addEventListener("click", handleClickOutside, true);
});

onBeforeUnmount(() => {
  document.removeEventListener("click", handleClickOutside, true);
});

function handleSessionClick(sessionId: string) {
  props.model.selectSession(sessionId);
  props.model.closeSessionDropdown();
}

function handleBulkDelete() {
  const ids = Array.from(props.model.checkedSessionIds);
  if (!ids.length) return;
  props.model.openBulkDeleteSessionDialog(ids);
  props.model.closeSessionDropdown();
}
</script>
