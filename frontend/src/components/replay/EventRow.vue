<!-- A single typed event row for the replay timeline -->
<template>
  <div
    class="group flex cursor-pointer items-start gap-3 rounded-lg px-3 py-2.5 transition-colors"
    :class="active ? 'bg-white/8 ring-1 ring-inset ring-accent/20' : 'hover:bg-white/4'"
    role="button"
    :aria-current="active ? 'true' : undefined"
    tabindex="0"
    @click="$emit('select')"
    @keydown.enter="$emit('select')"
  >
    <div class="mt-0.5 shrink-0">
      <Badge :variant="kindVariant" class="w-20 justify-center">{{ shortKind }}</Badge>
    </div>
    <div class="min-w-0 flex-1">
      <div class="flex items-center gap-2">
        <span v-if="event.specialist || event.agent" class="text-[11px] font-medium text-accent/80">{{ event.specialist || event.agent }}</span>
        <span v-if="event.run_id" class="font-mono text-[10px] text-white/30 truncate">{{ shortId(event.run_id) }}</span>
      </div>
      <p v-if="event.message || event.title" class="mt-0.5 text-xs text-white/65 leading-relaxed line-clamp-2">{{ event.message || event.title }}</p>
      <div v-if="event.at" class="mt-1 text-[10px] text-white/25">{{ new Date(event.at).toLocaleTimeString() }}</div>
    </div>
    <div v-if="index !== undefined" class="shrink-0 font-mono text-[10px] text-white/20">{{ index }}</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { FleetEvent } from "@/api/fleet";
import Badge from "@/components/ui/Badge.vue";

const props = defineProps<{ event: any; active?: boolean; index?: number }>();
defineEmits(["select"]);

const kindVariant = computed(() => {
  const k = props.event.kind ?? props.event.type ?? "";
  if (k.includes("started") || k.includes("start")) return "info" as const;
  if (k.includes("finished") || k.includes("completed") || k.includes("done")) return "success" as const;
  if (k.includes("failed") || k.includes("error")) return "danger" as const;
  if (k.includes("input")) return "warning" as const;
  if (k.includes("delegation") || k.includes("agent")) return "accent" as const;
  return "muted" as const;
});

const shortKind = computed(() => {
  const k = String(props.event.kind ?? props.event.type ?? "");
  const map: Record<string, string> = {
    run_started: "▶ start", run_finished: "✓ done", run_failed: "✗ fail",
    run_cancelled: "⊘ cancel", node_started: "▷ node", node_completed: "✓ node",
    node_failed: "✗ node", node_skipped: "⤳ skip", node_retrying: "↺ retry",
    tool_start: "↑ tool", tool_result: "↓ tool",
    delegation: "→ agent", error: "⚡ err",
    input_request: "? input", input_answered: "✓ input",
  };
  return map[k] ?? k.replace(/_/g, " ").slice(0, 10);
});

function shortId(id: string) {
  return id?.length > 12 ? id.slice(0, 5) + "…" + id.slice(-3) : id;
}
</script>
