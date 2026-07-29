<template>
  <div class="chat-modern flex h-full min-h-0 flex-1 overflow-hidden">
    <section
      class="chat-grid grid h-full min-h-0 flex-1 grid-cols-[236px_minmax(0,1fr)_236px] gap-0 overflow-hidden"
    >
      <ChatSessionPanel :model="sessionPanel" />

      <section
        class="chat-pane relative flex h-full min-h-0 flex-col overflow-hidden px-5"
      >
        <ChatHeaderPanel :model="headerPanel" />
        <ChatTimelinePanel :model="timelinePanel" />
        <ChatTranscript :model="transcript" />
        <ApproveCommand :items="pendingApprovals" :model="transcript" />
        <ChatComposerPanel :model="composerPanel" />
        <ContextInspectorDrawer
          :open="contextInspector.open.value"
          :loading="contextInspector.loading.value"
          :context-loading="contextInspector.contextLoading.value"
          :requests="contextInspector.requests.value"
          :selected-request-id="contextInspector.selectedRequestId.value"
          :selected-context="contextInspector.selectedContext.value"
          :error="contextInspector.error.value"
          @close="contextInspector.close"
          @select-request="contextInspector.selectRequest"
        />
      </section>

      <ChatParticipantsPanel :model="participantsPanel" />
    </section>

    <ChatModals :model="modals" />
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import "highlight.js/styles/github-dark-dimmed.css";
import "@/components/chat/chat.css";
import ApproveCommand from "@/components/chat/ApproveCommand.vue";
import ChatComposerPanel from "@/components/chat/ChatComposerPanel.vue";
import ContextInspectorDrawer from "@/components/chat/ContextInspectorDrawer.vue";
import ChatHeaderPanel from "@/components/chat/ChatHeaderPanel.vue";
import ChatModals from "@/components/chat/ChatModals.vue";
import ChatParticipantsPanel from "@/components/chat/ChatParticipantsPanel";
import ChatSessionPanel from "@/components/chat/ChatSessionPanel.vue";
import ChatTimelinePanel from "@/components/chat/ChatTimelinePanel.vue";
import ChatTranscript from "@/components/chat/ChatTranscript.vue";
import { useChatViewController } from "@/composables/chat/useChatViewController";

const {
  sessionPanel,
  headerPanel,
  timelinePanel,
  transcript,
  contextInspector,
  composerPanel,
  participantsPanel,
  modals,
} = useChatViewController();

const pendingApprovals = computed(() =>
  transcript.value.chatMessages.flatMap((message) =>
    (message.inputRequests || [])
      .filter((request) => transcript.value.isInputRequestRespondable(request))
      .map((request) => ({ message, request })),
  ),
);
</script>
