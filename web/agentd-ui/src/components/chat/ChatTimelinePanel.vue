<template>
  <section class="cockpit-center-panels" aria-label="Orchestration summaries">
    <article
      class="cockpit-center-card cockpit-timeline-card"
      aria-label="Execution Timeline"
    >
      <header class="cockpit-card-header">
        <div>
          <p class="chat-panel-kicker">Specialists Execution Timeline</p>
        </div>
        <div class="cockpit-card-actions">
          <span class="cockpit-card-pill">1x</span>
          <span class="cockpit-card-pill cockpit-card-pill--live">{{
            model.runActivitySidebarLabel
          }}</span>
        </div>
      </header>
      <div v-if="model.cockpitTimelineLanes.length" class="cockpit-timeline-grid">
        <div class="cockpit-timeline-scale" aria-hidden="true">
          <span class="cockpit-timeline-scale-label"></span>
          <div class="cockpit-timeline-tick-track">
            <span
              v-for="tick in model.cockpitTimelineTicks"
              :key="tick.id"
              class="cockpit-timeline-tick"
              :style="{ left: tick.position }"
            >
              {{ tick.label }}
            </span>
          </div>
        </div>
        <div
          v-for="lane in model.cockpitTimelineLanes"
          :key="lane.id"
          class="cockpit-timeline-lane"
        >
          <div class="cockpit-timeline-agent">
            <span
              class="cockpit-status-dot"
              :class="`cockpit-status-dot--${lane.status}`"
            ></span>
            <span class="cockpit-timeline-agent-name">{{ lane.name }}</span>
            <span class="cockpit-timeline-agent-state">{{
              lane.statusLabel
            }}</span>
          </div>
          <div class="cockpit-timeline-track">
            <span
              v-for="tick in model.cockpitTimelineTicks.slice(1)"
              :key="`${lane.id}:${tick.id}`"
              class="cockpit-timeline-gridline"
              :style="{ left: tick.position }"
            ></span>
            <span
              v-for="segment in lane.segments"
              :key="segment.id"
              class="cockpit-timeline-bar"
              :class="`cockpit-timeline-bar--${segment.status}`"
              :style="{ left: segment.left, width: segment.width }"
              :aria-label="segment.label"
              :title="segment.label"
            >
              <span>{{ segment.durationLabel }}</span>
            </span>
          </div>
        </div>
      </div>
      <p v-else class="cockpit-empty-text">
        No active timeline for this conversation.
      </p>
    </article>
  </section>
</template>

<script setup lang="ts">
import type { ChatTimelinePanelModel } from "@/composables/chat/useChatViewController";

defineProps<{
  model: ChatTimelinePanelModel;
}>();
</script>
