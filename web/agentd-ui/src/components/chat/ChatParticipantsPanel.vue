<template>
  <aside
    class="chat-participants-panel flex h-full min-h-0 flex-col text-sm text-subtle-foreground chat-side"
  >
    <div class="flex min-h-0 flex-1 flex-col">
      <GlassCard
        flat
        :padded="false"
        class="flex min-h-0 flex-1 flex-col overflow-hidden"
      >
        <div class="flex min-h-0 flex-1 flex-col">
          <div class="participant-team-select">
            <DropdownSelect
              :model-value="model.selectedTeam"
              :options="model.teamOptions"
              size="xs"
              title="Team for this conversation"
              aria-label="Specialist team"
              class="w-full"
              @update:model-value="model.setSelectedTeamValue"
            />
          </div>
          <div class="participant-list-scroll min-h-0 flex-1 overflow-y-auto">
            <div
              v-if="!model.participantList.length"
              class="rounded-4 border border-dashed border-border bg-surface p-3 text-xs text-subtle-foreground"
            >
              No specialists available
            </div>
            <ul v-else class="participant-list">
              <li
                v-for="participant in model.participantList"
                :key="participant.id"
                class="participant-list-item"
              >
                <button
                  type="button"
                  class="participant-row"
                  :class="model.participantRowClasses(participant)"
                  :aria-label="`Open activity for ${participant.name}`"
                  @click="model.openParticipantActivity(participant)"
                >
                  <span
                    class="participant-dot"
                    :class="model.participantDotClasses(participant)"
                  ></span>
                  <span class="participant-body">
                    <span class="participant-name">{{ participant.name }}</span>
                    <span class="participant-model">
                      {{
                        participant.model ? `${participant.model}` : "Model pending"
                      }}
                    </span>
                  </span>
                  <span class="participant-status">
                    {{ model.participantStatusLabel(participant) }}
                  </span>
                </button>
              </li>
            </ul>
          </div>
        </div>
      </GlassCard>
    </div>
  </aside>
</template>

<script setup lang="ts">
import DropdownSelect from "@/components/DropdownSelect.vue";
import type { ChatParticipantsPanelModel } from "@/composables/chat/useChatViewController";
import GlassCard from "@/components/ui/GlassCard.vue";

defineProps<{
  model: ChatParticipantsPanelModel;
}>();
</script>
