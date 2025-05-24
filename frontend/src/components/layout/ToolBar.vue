<template>
  <div class="absolute bottom-0 left-0 right-0 h-10 flex justify-center items-center z-10">
    <div class="flex justify-evenly items-center"></div>
    <div class="flex justify-center items-center bg-zinc-900 rounded-xl w-[33vw] h-full p-1 border border-gray-600 mb-10">
      <div class="flex-1 flex justify-center">
        <div class="relative flex items-center group">
            <label class="inline-flex relative items-center cursor-pointer">
            <input
              type="checkbox"
              :checked="modelValue"
              @change="$emit('update:modelValue', $event.target.checked)"
              class="sr-only peer"
            />
            <div class="w-10 h-5 bg-zinc-700 rounded-full peer-checked:bg-teal-600 transition-colors"></div>
            <div class="absolute left-1 top-1/2 -translate-y-1/2 bg-white w-4 h-4 rounded-full peer-checked:translate-x-5 transition-transform"></div>
            </label>
          <span class="text-white ml-2 text-sm">Auto-Pan</span>
          <div class="invisible absolute bottom-full left-1/2 transform -translate-x-1/2 mb-1 w-48 bg-orange-500/90 text-white px-3 py-2 rounded-xl text-xs font-bold z-50 text-center whitespace-normal group-hover:visible">
            When enabled, the view will automatically pan to follow node execution
          </div>
        </div>
      </div>
      <div class="flex-1 flex justify-center items-center">
        <button @click="$emit('run')" class="px-4 py-1 bg-teal-600 text-white rounded-xl text-base font-bold hover:bg-teal-500 transition">
          Run
        </button>
      </div>
      <div class="flex-1 flex justify-center">
        <LayoutControls
          ref="layoutControls"
          @update-nodes="$emit('update-layout', $event)"
          @update-edge-type="$emit('update-edge-type', $event)"
          :style="{ zIndex: 1000 }"
        />
      </div>
    </div>
    <div class="flex justify-evenly items-center"></div>
  </div>
</template>

<script setup>
import LayoutControls from '@/components/layout/LayoutControls.vue'

defineOptions({ inheritAttrs: false })
const props = defineProps({
  modelValue: Boolean
})
const emit = defineEmits(['update:modelValue', 'run', 'update-layout', 'update-edge-type'])
</script>
