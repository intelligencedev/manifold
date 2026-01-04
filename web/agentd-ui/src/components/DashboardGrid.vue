<template>
  <grid-layout
    v-model:layout="currentLayout"
    :col-num="12"
    :row-height="80"
    :is-draggable="true"
    :is-resizable="true"
    :vertical-compact="true"
    :use-css-transforms="true"
    :margin="[16, 16]"
    class="dashboard-grid"
    @layout-updated="onLayoutUpdated"
  >
    <grid-item
      v-for="item in currentLayout"
      :key="item.i"
      :x="item.x"
      :y="item.y"
      :w="item.w"
      :h="item.h"
      :i="item.i"
      :static="item.static"
      :min-w="item.minW || 2"
      :min-h="item.minH || 2"
      class="dashboard-grid-item"
    >
      <div class="grid-item-content">
        <slot :name="`item-${item.i}`" :item="item" />
      </div>
    </grid-item>
  </grid-layout>
</template>

<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { GridLayout, GridItem } from 'vue3-grid-layout-next'
import 'vue3-grid-layout-next/dist/style.css'

export interface GridItemConfig {
  i: string
  x: number
  y: number
  w: number
  h: number
  minW?: number
  minH?: number
  static?: boolean
}

interface Props {
  layout: GridItemConfig[]
  storageKey?: string
}

const props = withDefaults(defineProps<Props>(), {
  storageKey: 'dashboard-layout',
})

const emit = defineEmits<{
  'layout-change': [layout: GridItemConfig[]]
}>()

const currentLayout = ref<GridItemConfig[]>([...props.layout])

// Load saved layout from localStorage
onMounted(() => {
  if (props.storageKey) {
    const saved = localStorage.getItem(props.storageKey)
    if (saved) {
      try {
        const savedLayout = JSON.parse(saved) as GridItemConfig[]
        // Merge saved layout with default props, preserving IDs from props
        const merged = props.layout.map((item) => {
          const savedItem = savedLayout.find((s) => s.i === item.i)
          return savedItem ? { ...item, ...savedItem } : item
        })
        currentLayout.value = merged
      } catch (e) {
        console.warn('Failed to load saved layout:', e)
      }
    }
  }
})

function onLayoutUpdated(newLayout: GridItemConfig[]) {
  currentLayout.value = newLayout
  emit('layout-change', newLayout)
  
  // Save to localStorage
  if (props.storageKey) {
    try {
      localStorage.setItem(props.storageKey, JSON.stringify(newLayout))
    } catch (e) {
      console.warn('Failed to save layout:', e)
    }
  }
}

// Watch for external layout changes
watch(() => props.layout, (newLayout) => {
  // Only update if the structure (number of items) has changed
  if (newLayout.length !== currentLayout.value.length) {
    currentLayout.value = [...newLayout]
  }
}, { deep: true })

// Expose method to reset layout
defineExpose({
  resetLayout: () => {
    currentLayout.value = [...props.layout]
    if (props.storageKey) {
      localStorage.removeItem(props.storageKey)
    }
  },
})
</script>

<style scoped>
.dashboard-grid {
  @apply min-h-screen w-full;
  max-width: 100%;
  overflow-x: hidden;
}

.dashboard-grid-item {
  @apply rounded-2xl;
  transition: all 0.2s ease;
  touch-action: none;
}

.dashboard-grid-item:hover {
  @apply ring-2 ring-accent/30;
}

.grid-item-content {
  @apply h-full w-full overflow-auto rounded-2xl bg-surface;
}

/* Override library styles for better appearance */
:deep(.vue-grid-item) {
  @apply transition-all duration-200;
}

:deep(.vue-grid-item.vue-grid-placeholder) {
  @apply rounded-2xl bg-accent/20 backdrop-blur-sm;
  border: 2px dashed rgba(59, 130, 246, 0.5) !important;
}

/* Make resize handles more visible and easier to use */
:deep(.vue-resizable-handle) {
  @apply opacity-40 transition-opacity;
  width: 20px !important;
  height: 20px !important;
  bottom: 2px !important;
  right: 2px !important;
  /* Clip the grip so it doesn't overlap the rounded card corner */
  border-bottom-right-radius: 1rem !important; /* matches Tailwind rounded-2xl */
  overflow: hidden !important;
  background-image: url('data:image/svg+xml;utf8,<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 20 20"><path d="M 20 0 L 20 20 L 0 20 Z" fill="%23888" opacity="0.3"/><path d="M 14 20 L 20 14 L 20 20 Z M 8 20 L 20 8 L 20 12 L 12 20 Z" fill="%23fff" opacity="0.8"/></svg>') !important;
  background-position: bottom right !important;
  background-repeat: no-repeat !important;
  cursor: nwse-resize !important;
}

.dashboard-grid-item:hover :deep(.vue-resizable-handle) {
  @apply opacity-100;
}
</style>
