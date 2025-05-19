<template>
  <div :class="['fixed top-[62px] bottom-0 w-64 z-[1100] transition-transform duration-300 text-white dark:bg-neutral-800 flex flex-col space-y-2 shadow-[5px_0_10px_-5px_rgba(0,0,0,0.3)]', isOpen ? 'translate-x-0' : '-translate-x-full']">
    <div class="absolute top-1/2 right-0 translate-x-full w-[30px] h-[60px] dark:bg-neutral-800 rounded-r-md flex items-center justify-center cursor-pointer shadow-[5px_0_10px_-5px_rgba(0,0,0,0.3)]" @click="togglePalette">
      <svg v-if="isOpen" xmlns="http://www.w3.org/2000/svg" class="w-5 h-5" fill="currentColor" viewBox="0 0 16 16">
        <path fill-rule="evenodd" d="M11.354 1.646a.5.5 0 0 1 0 .708L5.707 8l5.647 5.646a.5.5 0 0 1-.708.708l-6-6a.5.5 0 0 1 0-.708l6-6a.5.5 0 0 1 .708 0z" />
      </svg>
      <svg v-else xmlns="http://www.w3.org/2000/svg" class="w-5 h-5" fill="currentColor" viewBox="0 0 16 16">
        <path fill-rule="evenodd" d="M4.646 1.646a.5.5 0 0 1 .708 0l6 6a.5.5 0 0 1 0 .708l-6 6a.5.5 0 0 1-.708-.708L10.293 8 4.646 2.354a.5.5 0 0 1 0-.708z" />
      </svg>
    </div>
    <div class="p-1 h-full box-border">
      <div class="overflow-y-auto h-full pr-2 space-y-4">
        <div v-for="(nodes, category) in nodeCategories" :key="category">
          <div class="text-base font-bold mb-2 uppercase text-gray-300 cursor-pointer flex justify-between items-center p-2 dark:bg-neutral-800 border border-white/20 rounded hover:bg-white/20" @click="toggleAccordion(category)">
        <span>{{ category }}</span>
        <svg v-if="isExpanded(category)" xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" fill="currentColor" viewBox="0 0 16 16">
          <path fill-rule="evenodd" d="M11.354 1.646a.5.5 0 0 1 0 .708L5.707 8l5.647 5.646a.5.5 0 0 1-.708.708l-6-6a.5.5 0 0 1 0-.708l6-6a.5.5 0 0 1 .708 0z" />
        </svg>
        <svg v-else xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" fill="currentColor" viewBox="0 0 16 16">
          <path fill-rule="evenodd" d="M4.646 1.646a.5.5 0 0 1 .708 0l6 6a.5.5 0 0 1 0 .708l-6 6a.5.5 0 0 1-.708-.708L10.293 8 4.646 2.354a.5.5 0 0 1 0-.708z" />
        </svg>
          </div>
          <div v-if="isExpanded(category)" class="pl-2 space-y-3">
        <div v-for="node in nodes" :key="node.type" class="p-3 mb-2 dark:bg-neutral-800 border border-white/20 rounded cursor-grab text-base font-medium flex items-center justify-center hover:bg-white/20" draggable="true" @dragstart="(event) => onDragStart(event, node.type)">
          {{ node.type }}
        </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import useDragAndDrop from '../composables/useDnD.js'
import useNodePalette from '../composables/useNodePalette.js'

const { onDragStart } = useDragAndDrop()
const { isOpen, togglePalette, nodeCategories, toggleAccordion, isExpanded } = useNodePalette()
</script>

<!-- No scoped styles; Tailwind classes are used -->
