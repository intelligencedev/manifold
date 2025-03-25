<template>
  <div class="base-checkbox">
    <label class="checkbox-label">
      <input 
        type="checkbox" 
        :id="id" 
        :checked="modelValue"
        class="checkbox-input" 
        @change="handleChange"
      />
      <span class="label-text">{{ label }}</span>
    </label>
  </div>
</template>

<script setup>
const props = defineProps({
  modelValue: {
    type: Boolean,
    default: false
  },
  label: {
    type: String,
    required: true
  },
  id: {
    type: String,
    default: () => `checkbox-${Math.random().toString(36).substring(2, 9)}`
  }
})

const emit = defineEmits(['update:modelValue'])

function handleChange(event) {
  emit('update:modelValue', event.target.checked)
}
</script>

<style scoped>
.base-checkbox {
  margin-bottom: 8px;
}

.checkbox-label {
  display: flex;
  align-items: center;
  cursor: pointer;
  color: var(--input-label-color, #eee);
  font-size: 14px;
}

.checkbox-input {
  margin-right: 8px;
  cursor: pointer;
}

.label-text {
  user-select: none;
}
</style>