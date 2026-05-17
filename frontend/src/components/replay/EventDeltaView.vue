<template>
  <pre
    class="overflow-auto rounded-xl border border-border bg-black/20 p-4 text-xs text-white/75 leading-relaxed"
    style="max-height: 420px; white-space: pre-wrap; word-break: break-all;"
  >{{ formatted }}</pre>
</template>

<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{ event: unknown }>();

const formatted = computed(() => {
  try { return JSON.stringify(props.event, null, 2); }
  catch { return String(props.event); }
});
</script>
