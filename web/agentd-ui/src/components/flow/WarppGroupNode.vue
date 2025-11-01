<template>
  <div class="group-node relative h-full w-full">
    <NodeResizer
      v-if="isDesignMode"
      :min-width="GROUP_MIN_WIDTH"
      :min-height="GROUP_MIN_HEIGHT"
      :handle-style="RESIZER_HANDLE_STYLE"
      :line-style="RESIZER_LINE_STYLE"
      @resize-end="onResizeEnd"
    />
    <div class="group-surface" />
    <div class="group-header" :class="{ 'pointer-events-none': !isDesignMode }">
      <input
        v-model="labelText"
        :disabled="!isDesignMode"
        class="group-title"
        type="text"
        :placeholder="defaultLabel"
        spellcheck="false"
      />
      <button
        v-if="isDesignMode"
        type="button"
        class="group-ungroup"
        @click.stop.prevent="requestUngroup"
      >
        Ungroup
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, ref, watch, type CSSProperties, type Ref } from 'vue'
import { NodeResizer, type OnResizeEnd } from '@vue-flow/node-resizer'
import { useVueFlow, type NodeProps } from '@vue-flow/core'

import type { GroupNodeData } from '@/types/flow'
import { WARPP_GROUP_NODE_DIMENSIONS } from '@/constants/warppNodes'

defineOptions({ name: 'WarppGroupNode', inheritAttrs: false })

const props = defineProps<NodeProps<GroupNodeData>>()
const emit = defineEmits<{ (event: 'request-ungroup', id: string): void }>()

const { updateNode, updateNodeData } = useVueFlow()

const GROUP_MIN_WIDTH = WARPP_GROUP_NODE_DIMENSIONS.minWidth
const GROUP_MIN_HEIGHT = WARPP_GROUP_NODE_DIMENSIONS.minHeight
const defaultLabel = 'Group'

const modeRef = inject<Ref<'design' | 'run'>>('warppMode', ref<'design' | 'run'>('design'))
const isDesignMode = computed(() => modeRef.value === 'design')
const ungroupHandler = inject<((id: string) => void) | null>('warppRequestUngroup', null)

const RESIZER_HANDLE_STYLE = Object.freeze({
  width: '14px',
  height: '14px',
  opacity: '0',
  border: 'none',
  background: 'transparent',
})

const RESIZER_LINE_STYLE = Object.freeze({ opacity: '0' })

const labelText = ref(defaultLabel)
let suppressCommit = false

watch(
  () => props.data?.label,
  (next) => {
    suppressCommit = true
    labelText.value = next?.trim() || defaultLabel
    suppressCommit = false
  },
  { immediate: true },
)

watch(labelText, (next) => {
  if (suppressCommit) return
  const label = next.trim() || defaultLabel
  updateNodeData(props.id, {
    ...(props.data ?? { kind: 'group', label }),
    label,
  })
})

function onResizeEnd(event: OnResizeEnd) {
  if (!isDesignMode.value) return
  const widthPx = `${Math.max(Math.round(event.params.width), GROUP_MIN_WIDTH)}px`
  const heightPx = `${Math.max(Math.round(event.params.height), GROUP_MIN_HEIGHT)}px`
  updateNode(props.id, (node) => {
    const baseStyle: CSSProperties =
      typeof node.style === 'function' ? ((node.style(node) as CSSProperties) ?? {}) : { ...(node.style ?? {}) }
    return {
      style: {
        ...baseStyle,
        width: widthPx,
        height: heightPx,
      },
    }
  })
}

function requestUngroup() {
  ungroupHandler?.(props.id)
  emit('request-ungroup', props.id)
}
</script>

<style scoped>
.group-node {
  border-radius: 12px;
}
.group-surface {
  position: absolute;
  inset: 0;
  border-radius: 12px;
  background: color-mix(in srgb, rgb(var(--color-surface) / 1) 75%, transparent 25%);
  border: 1px dashed rgb(var(--color-border) / 0.65);
}
.group-header {
  position: absolute;
  top: 0.35rem;
  left: 0.5rem;
  right: 0.5rem;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  pointer-events: auto;
}
.group-title {
  flex: 1;
  min-width: 0;
  border: 1px solid rgb(var(--color-border) / 0.75);
  border-radius: 0.5rem;
  background: rgb(var(--color-surface) / 0.9);
  padding: 0.25rem 0.5rem;
  font-size: 0.7rem;
  color: rgb(var(--color-foreground));
}
.group-title:disabled {
  opacity: 0.7;
  cursor: default;
}
.group-ungroup {
  border-radius: 0.5rem;
  border: 1px solid rgb(var(--color-border) / 0.7);
  background: rgb(var(--color-surface-muted) / 0.9);
  padding: 0.25rem 0.45rem;
  font-size: 0.65rem;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: rgb(var(--color-subtle-foreground));
}
.group-ungroup:hover {
  color: rgb(var(--color-foreground));
  background: rgb(var(--color-surface-muted));
}
</style>
