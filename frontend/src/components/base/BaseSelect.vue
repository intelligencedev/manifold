<template>
  <div class="base-select">
    <label v-if="label" :for="id" class="input-label">{{ label }}:</label>
    <select :id="id" class="input-select" v-model="internalValue" @change="handleChange">
      <option v-for="option in normalizedOptions" :key="option.value" :value="option.value">
        {{ option.label }}
      </option>
    </select>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'

const props = defineProps({
  modelValue: {
    type: [String, Number],
    required: true
  },
  label: {
    type: String,
    default: ''
  },
  id: {
    type: String,
    default: () => `select-${Math.random().toString(36).substring(2, 9)}`
  },
  options: {
    type: Array,
    required: true,
    default: () => []
  }
})

const emit = defineEmits(['update:modelValue'])

// Handle both formats of options: ['option1', 'option2'] or [{ value: 'option1', label: 'Option 1' }]
const normalizedOptions = computed(() => {
  return props.options.map(option => {
    if (typeof option === 'object' && option !== null) {
      return { value: option.value, label: option.label || option.value }
    }
    return { value: option, label: option }
  })
})

const internalValue = ref(props.modelValue)

watch(() => props.modelValue, (newValue) => {
  internalValue.value = newValue
})

watch(internalValue, (newValue) => {
  emit('update:modelValue', newValue)
})

function handleChange() {
  emit('update:modelValue', internalValue.value)
}
</script>

<style scoped>
.base-select {
  margin-bottom: 8px;
}

.input-label {
  display: block;
  margin-bottom: 4px;
  font-size: 14px;
  color: var(--input-label-color, #eee);
}

.input-select {
  width: 100%;
  padding: 6px 8px;
  font-size: 14px;
  border: 1px solid var(--input-border-color, #666);
  border-radius: 4px;
  background-color: var(--input-bg-color, #333);
  color: var(--input-text-color, #eee);
}
</style>