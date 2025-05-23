<template>
  <div class="w-full mb-2 border border-slate-600 rounded bg-zinc-900 text-gray-200 box-border">
    <!-- Summary/Header -->
    <div 
      class="cursor-pointer p-2 font-bold select-none flex justify-between items-center" 
      @click="isOpen = !isOpen"
    >
      <span class="flex-grow text-center">{{ title }}</span>
      <span class="transition-transform duration-300" :class="{ 'rotate-180': isOpen }">
        ▼
      </span>
    </div>
    
    <!-- Content -->
    <div 
      class="overflow-hidden transition-all duration-300 ease-in-out" 
      :style="{ maxHeight: contentHeight, opacity: isOpen ? 1 : 0 }"
      ref="contentRef"
    >
      <div class="px-2 pb-2 text-left space-y-2" ref="innerContentRef">
        <slot></slot>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, watch, nextTick } from 'vue'

const props = defineProps({
  title: {
    type: String,
    required: true
  },
  initiallyOpen: {
    type: Boolean,
    default: true
  }
})

const isOpen = ref(props.initiallyOpen)
const contentRef = ref(null)
const innerContentRef = ref(null)
const contentHeight = ref(isOpen.value ? 'auto' : '0px')

// Update height when content changes
const updateContentHeight = async () => {
  if (!contentRef.value || !innerContentRef.value) return
  
  if (isOpen.value) {
    // First set a fixed height for animation
    contentHeight.value = `${innerContentRef.value.offsetHeight}px`
    
    // Then switch to auto after animation completes
    setTimeout(() => {
      if (isOpen.value) contentHeight.value = 'auto'
    }, 300) // Match the duration in the transition
  } else {
    // Set a fixed height first (for animation from auto)
    contentHeight.value = `${innerContentRef.value.offsetHeight}px`
    
    // Force a reflow to ensure the browser registers the change
    contentRef.value.offsetHeight
    
    // Then animate to zero
    contentHeight.value = '0px'
  }
}

// Initialize on mount
onMounted(async () => {
  await nextTick()
  updateContentHeight()
})

// Watch for toggle changes
watch(isOpen, updateContentHeight)
</script>