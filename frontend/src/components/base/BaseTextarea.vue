<template>
  <div v-bind="$attrs" class="mb-2 w-full h-full flex flex-col">
    <label v-if="label" :for="id" class="mb-1 block text-sm text-gray-200">{{ label }}:</label>
    <div class="relative flex flex-1 min-h-0">
      <textarea 
        :id="id" 
        class="w-full px-2 py-1.5 text-sm border border-gray-600 rounded-md bg-gray-800 text-gray-200 resize-none overflow-y-auto flex-1 h-full min-h-0" 
        v-model="internalValue" 
        @input="handleInput"
      ></textarea>
      <slot name="suffix"></slot>
    </div>
  </div>
</template>

<script setup>
import { ref, watch } from 'vue'

defineOptions({
  inheritAttrs: false
})

const props = defineProps({
  modelValue: {
    type: String,
    default: ''
  },
  label: {
    type: String,
    default: ''
  },
  id: {
    type: String,
    default: () => `textarea-${Math.random().toString(36).substring(2, 9)}`
  }
})

const emit = defineEmits(['update:modelValue'])

const internalValue = ref(props.modelValue)

watch(() => props.modelValue, (newValue) => {
  internalValue.value = newValue
})

function handleInput() {
  emit('update:modelValue', internalValue.value)
}
</script>