<template>
  <div class="base-input">
    <label v-if="label" :for="id" class="input-label">{{ label }}:</label>
    <div class="input-wrapper">
      <input 
        :id="id" 
        :type="type" 
        class="input-text" 
        :value="modelValue"
        @input="handleInput"
        v-bind="$attrs"
      />
      <slot name="suffix"></slot>
    </div>
  </div>
</template>

<script setup>
defineOptions({
  inheritAttrs: false
})

const props = defineProps({
  modelValue: {
    type: [String, Number],
    default: ''
  },
  label: {
    type: String,
    default: ''
  },
  id: {
    type: String,
    default: () => `input-${Math.random().toString(36).substring(2, 9)}`
  },
  type: {
    type: String,
    default: 'text'
  }
})

const emit = defineEmits(['update:modelValue'])

function handleInput(event) {
  const value = props.type === 'number' 
    ? event.target.valueAsNumber 
    : event.target.value;
    
  emit('update:modelValue', value)
}
</script>

<style scoped>
.base-input {
  margin-bottom: 8px;
}

.input-label {
  display: block;
  margin-bottom: 4px;
  font-size: 14px;
  color: var(--input-label-color, #eee);
}

.input-wrapper {
  position: relative;
  display: flex;
}

.input-text {
  width: 100%;
  padding: 6px 8px;
  font-size: 14px;
  border: 1px solid var(--input-border-color, #666);
  border-radius: 4px;
  background-color: var(--input-bg-color, #333);
  color: var(--input-text-color, #eee);
}

/* Safari fix to hide spinner arrows */
.input-text::-webkit-inner-spin-button,
.input-text::-webkit-outer-spin-button {
  -webkit-appearance: none;
  margin: 0;
}
/* Firefox fix */
.input-text[type=number] {
  -moz-appearance: textfield;
  appearance: textfield;
}
</style>