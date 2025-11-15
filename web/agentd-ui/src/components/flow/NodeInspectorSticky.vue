<template>
  <div class="space-y-3">
    <div class="text-xs text-subtle-foreground">Sticky Note</div>
    <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
      Note
      <textarea v-model="noteText" rows="6" class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground overflow-auto w-full resize-none whitespace-pre-wrap break-words" :disabled="!isDesignMode || hydratingRef" />
    </label>
    <div class="pt-1 flex items-center justify-end gap-2">
      <span v-if="isDirty" class="text-[10px] italic text-warning-foreground">Unsaved</span>
      <button class="rounded bg-accent px-2 py-1 text-[11px] font-medium text-accent-foreground transition disabled:opacity-40" :disabled="!isDirty || !isDesignMode" @click="applyChanges">Apply</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, ref, watch, type Ref } from 'vue'
import { useVueFlow } from '@vue-flow/core'
import type { StickyNoteNodeData } from '@/types/flow'

const props = defineProps<{ nodeId: string; data: StickyNoteNodeData }>()
const { updateNodeData } = useVueFlow()
const modeRef = inject<Ref<'design' | 'run'>>('warppMode', ref<'design' | 'run'>('design'))
const hydratingRef = inject<Ref<boolean>>('warppHydrating', ref(false))

const isDesignMode = computed(() => modeRef.value === 'design')
const noteText = ref('')
const isDirty = ref(false)
let suppress = false

watch(
  () => props.data?.note,
  (next) => {
    suppress = true
    noteText.value = next ?? ''
    isDirty.value = false
    suppress = false
  },
  { immediate: true },
)

watch(noteText, () => {
  if (suppress || hydratingRef.value || !isDesignMode.value) return
  isDirty.value = true
})

function applyChanges() {
  if (!isDesignMode.value || !isDirty.value) return
  updateNodeData(props.nodeId, { ...(props.data ?? { kind: 'utility' }), note: noteText.value })
  isDirty.value = false
}
</script>

