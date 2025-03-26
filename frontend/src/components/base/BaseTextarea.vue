<template>
  <div class="base-textarea" :class="{ 'full-height': fullHeight }">
    <label v-if="label" :for="id" class="input-label">{{ label }}:</label>
    <div class="textarea-wrapper">
      <textarea 
        :id="id" 
        class="input-textarea" 
        v-model="internalValue" 
        @input="handleInput"
        v-bind="$attrs"
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
  },
  fullHeight: {
    type: Boolean,
    default: false
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

<style scoped>
.base-textarea {
  margin-bottom: 8px;
  width: 100%;
}

.base-textarea.full-height {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

.input-label {
  display: block;
  margin-bottom: 4px;
  font-size: 14px;
  color: var(--input-label-color, #eee);
}

.textarea-wrapper {
  position: relative;
  display: flex;
  flex: 1;
  min-height: 0;
}

.input-textarea {
  width: 100%;
  padding: 6px 8px;
  font-size: 14px;
  border: 1px solid var(--input-border-color, #666);
  border-radius: 4px;
  background-color: var(--input-bg-color, #333);
  color: var(--input-text-color, #eee);
  resize: none;
  overflow-y: auto;
  flex: 1;
  min-height: 0;
}

.full-height .input-textarea {
  height: 100%;
}
</style>