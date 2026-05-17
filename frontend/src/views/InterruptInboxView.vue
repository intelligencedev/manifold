<template>
  <section class="space-y-5">
    <!-- Page header -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-xl font-semibold">Interrupt Inbox</h1>
        <p class="mt-0.5 text-sm text-white/50">Pending operator approvals and clarification requests. <kbd class="rounded border border-border px-1 py-0.5 text-[10px]">j/k</kbd> navigate · <kbd class="rounded border border-border px-1 py-0.5 text-[10px]">a</kbd> approve · <kbd class="rounded border border-border px-1 py-0.5 text-[10px]">d</kbd> dismiss</p>
      </div>
      <Badge :variant="requests.length > 0 ? 'warning' : 'muted'" :dot="requests.length > 0">
        {{ requests.length }} open
      </Badge>
    </div>

    <!-- Empty state -->
    <EmptyState
      v-if="requests.length === 0"
      icon="✅"
      title="No pending requests"
      description="All agent input requests have been answered. The fleet is running autonomously."
    />

    <!-- Request grid -->
    <div v-else class="grid gap-4 lg:grid-cols-2">
      <InterruptCard
        v-for="(req, idx) in requests"
        :key="String(req.request?.id ?? idx)"
        :request-id="String(req.request?.id ?? req.run_id ?? '')"
        :title="String(req.request?.question ?? 'Input request')"
        :reason="String(req.request?.reason ?? '')"
        :agent="String(req.request?.agent ?? '')"
        :depth="Number(req.request?.depth ?? 0)"
        :choices="req.request?.choices ?? []"
        :allow-free-text="req.request?.allow_free_text ?? true"
        :multiple="req.request?.multiple ?? false"
        :created-at="String(req.request?.created_at ?? '')"
        :active="activeIndex === idx"
        @answered="onAnswered"
        @dismiss="onDismiss(idx)"
      />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import Badge from "@/components/ui/Badge.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import InterruptCard from "@/components/inbox/InterruptCard.vue";
import { useFleetEvents } from "@/composables/useFleetEvents";
import { useInboxStore } from "@/stores/inbox";
import { useHotkeys } from "@/composables/useHotkeys";

const { refresh: refreshFleet } = useFleetEvents();
const inbox = useInboxStore();
const requests = computed(() => inbox.requests);
const activeIndex = ref(0);

useHotkeys({
  j: () => { activeIndex.value = Math.min(activeIndex.value + 1, requests.value.length - 1); },
  k: () => { activeIndex.value = Math.max(activeIndex.value - 1, 0); },
  a: () => { /* trigger submit on active card via programmatic event in future */ },
  d: () => onDismiss(activeIndex.value),
});

function onAnswered(_requestId: string) {
  refreshFleet();
}

function onDismiss(idx: number) {
  activeIndex.value = Math.min(idx, Math.max(0, requests.value.length - 2));
}
</script>
