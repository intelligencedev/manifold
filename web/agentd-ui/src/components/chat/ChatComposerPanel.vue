<template>
  <footer class="px-4 pb-4 pt-2">
    <form
      class="space-y-3"
      @submit.prevent="model.sendCurrentPrompt"
      @dragover.prevent
      @drop.prevent="model.handleDrop"
    >
      <p
        v-if="model.requiresProjectSelection"
        class="rounded-4 border border-danger/40 bg-danger/10 px-3 py-2 text-xs text-danger"
      >
        Select a project to run the agent. If you don't see any projects,
        contact an administrator.
      </p>
      <div class="halo-surface chat-prompt-input relative p-3">
        <div
          v-if="model.mentionMenuOpen"
          class="absolute bottom-full left-3 z-20 mb-2 w-72 overflow-hidden rounded-4 border border-border bg-surface ring-1 ring-border/50"
        >
          <div class="max-h-60 overflow-y-auto py-1">
            <button
              v-for="(cand, i) in model.mentionCandidates"
              :key="cand.id"
              type="button"
              class="flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-xs transition"
              :class="
                i === model.mentionActiveIndex
                  ? 'bg-surface-muted/60 text-foreground'
                  : 'text-subtle-foreground hover:bg-surface-muted/40 hover:text-foreground'
              "
              @mousedown.prevent="model.selectMentionCandidate(cand)"
            >
              <span class="truncate font-medium">@{{ cand.mentionName }}</span>
              <span class="shrink-0 text-[10px] text-faint-foreground">
                {{
                  cand.kind === "team_orchestrator"
                    ? "Team orchestrator"
                    : cand.model
                      ? `Model ${cand.model}`
                      : ""
                }}
              </span>
            </button>

            <div
              v-if="!model.mentionCandidates.length"
              class="px-3 py-2 text-xs text-faint-foreground"
            >
              No matching participants
            </div>
          </div>
        </div>

        <div class="flex items-center gap-3">
          <textarea
            :ref="model.setComposerRef"
            :value="model.draft"
            rows="1"
            class="flex-1 min-w-0 resize-none bg-transparent py-1.5 text-sm leading-6 text-foreground outline-none placeholder:text-faint-foreground"
            :placeholder="model.composerPlaceholder"
            :disabled="!model.projectSelected || model.hasPendingInputRequest"
            @keydown="model.handleComposerKeydown"
            @input="
              model.handleComposerInput(
                (($event.target as HTMLTextAreaElement | null)?.value || ''),
              )
            "
            @keyup="model.handleComposerKeyup"
            @click="model.updateMentionState"
          ></textarea>

          <div class="shrink-0 flex items-end gap-1">
            <input
              :ref="model.setFileInputRef"
              type="file"
              multiple
              class="hidden"
              accept="image/png,image/jpeg,text/plain,text/markdown,text/*"
              @change="model.handleFileInputChange"
            />

            <button
              type="button"
              class="inline-flex h-8 w-8 items-center justify-center rounded-3 focus-visible:shadow-outline"
              title="Attach files"
              aria-label="Attach files"
              :disabled="!model.projectSelected || model.hasPendingInputRequest"
              :class="
                !model.projectSelected || model.hasPendingInputRequest
                  ? 'cursor-not-allowed text-foreground/40 opacity-50'
                  : 'text-foreground/80 hover:text-accent'
              "
              @click="
                model.projectSelected && !model.hasPendingInputRequest
                  ? model.triggerFilePicker()
                  : undefined
              "
            >
              <SolarPaperclip2Bold class="h-5 w-5" />
            </button>

            <button
              type="button"
              class="inline-flex h-8 w-8 items-center justify-center rounded-3 focus-visible:shadow-outline"
              :class="[
                model.isRecording
                  ? 'text-danger hover:text-danger/90'
                  : 'text-foreground/80 hover:text-accent',
                model.isStreaming ||
                !model.canUseMic ||
                !model.projectSelected ||
                model.hasPendingInputRequest
                  ? 'cursor-not-allowed opacity-50'
                  : '',
              ]"
              :disabled="
                model.isStreaming ||
                !model.canUseMic ||
                !model.projectSelected ||
                model.hasPendingInputRequest
              "
              :title="model.isRecording ? 'Stop recording' : 'Record voice prompt'"
              :aria-label="
                model.isRecording ? 'Stop recording' : 'Record voice prompt'
              "
              @click="model.isRecording ? model.stopRecording() : model.startRecording()"
            >
              <SolarMicrophone3Bold class="h-5 w-5" />
            </button>

            <button
              type="button"
              class="inline-flex h-8 w-8 items-center justify-center rounded-3 transition focus-visible:shadow-outline"
              :class="[
                model.imagePrompt
                  ? 'bg-accent/20 text-accent hover:bg-accent/30'
                  : 'text-foreground/80 hover:text-accent',
                model.isStreaming || !model.projectSelected || model.hasPendingInputRequest
                  ? 'cursor-not-allowed opacity-50'
                  : '',
              ]"
              :disabled="
                model.isStreaming || !model.projectSelected || model.hasPendingInputRequest
              "
              title="Generate image response"
              aria-label="Generate image response"
              @click="model.setImagePromptValue(!model.imagePrompt)"
            >
              <Camera class="h-5 w-5" />
            </button>

            <button
              type="button"
              :class="[
                'inline-flex h-8 w-8 items-center justify-center rounded-3 focus-visible:shadow-outline',
                model.isStreaming
                  ? 'border border-danger/60 text-foreground/80 hover:text-danger'
                  : 'bg-accent text-accent-foreground hover:bg-accent/90',
              ]"
              :title="
                model.isStreaming &&
                !model.hasPendingInputRequest &&
                (model.draft.trim() || model.pendingAttachments.length)
                  ? 'Send message'
                  : model.isStreaming
                    ? 'Stop generating'
                    : 'Send message'
              "
              :aria-label="
                model.isStreaming &&
                !model.hasPendingInputRequest &&
                (model.draft.trim() || model.pendingAttachments.length)
                  ? 'Send message'
                  : model.isStreaming
                    ? 'Stop generating'
                    : 'Send message'
              "
              :disabled="
                !model.isStreaming &&
                (!model.projectSelected ||
                  (!model.draft.trim() && !model.pendingAttachments.length))
              "
              @click="
                model.isStreaming &&
                !model.hasPendingInputRequest &&
                (model.draft.trim() || model.pendingAttachments.length)
                  ? model.sendCurrentPrompt()
                  : model.isStreaming
                    ? model.stopStreaming()
                    : model.sendCurrentPrompt()
              "
            >
              <SolarStopBold v-if="model.isStreaming" class="h-4 w-4" />
              <SolarArrowToTopLeftBold v-else class="h-4 w-4" />
            </button>
          </div>
        </div>
      </div>
      <div v-if="model.pendingAttachments.length" class="space-y-2">
        <div v-if="model.imageAttachments.length" class="flex gap-2 overflow-x-auto pb-1">
          <div
            v-for="img in model.imageAttachments"
            :key="img.id"
            class="relative shrink-0"
          >
            <img
              :src="img.previewUrl"
              :alt="img.name"
              class="h-16 w-16 rounded border border-border object-cover"
            />
            <button
              type="button"
              class="absolute -right-1 -top-1 rounded-full bg-surface px-1 text-[10px] shadow ring-1 ring-border hover:text-danger"
              @click="model.removeAttachment(img.id)"
            >
              ×
            </button>
          </div>
        </div>
        <div v-if="model.textAttachments.length" class="flex flex-wrap gap-2">
          <span
            v-for="t in model.textAttachments"
            :key="t.id"
            class="inline-flex items-center gap-1 rounded-full border border-border bg-surface px-2 py-1 text-[11px]"
          >
            <span class="max-w-[180px] truncate">{{ t.name }}</span>
            <button
              type="button"
              class="text-faint-foreground hover:text-danger"
              @click="model.removeAttachment(t.id)"
            >
              ×
            </button>
          </span>
        </div>
      </div>
    </form>
  </footer>
</template>

<script setup lang="ts">
import SolarArrowToTopLeftBold from "@/components/icons/SolarArrowToTopLeftBold.vue";
import Camera from "@/components/icons/Camera.vue";
import SolarMicrophone3Bold from "@/components/icons/SolarMicrophone3Bold.vue";
import SolarPaperclip2Bold from "@/components/icons/SolarPaperclip2Bold.vue";
import SolarStopBold from "@/components/icons/SolarStopBold.vue";
import type { ChatComposerPanelModel } from "@/composables/chat/useChatViewController";

defineProps<{
  model: ChatComposerPanelModel;
}>();
</script>
