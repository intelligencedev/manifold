<template>
  <div class="mb-2">
    <label v-if="label" :for="id" class="block mb-1 text-md text-gray-200">{{ label }}:</label>
    <div class="relative flex">
      <input 
        :id="id" 
        :type="type" 
        class="w-full px-2 py-1.5 text-md border border-slate-600 rounded bg-zinc-700 text-gray-200" 
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

<style>
/* Safari fix to hide spinner arrows */
input::-webkit-inner-spin-button,
input::-webkit-outer-spin-button {
  -webkit-appearance: none;
  margin: 0;
}
/* Firefox fix */
input[type=number] {
  -moz-appearance: textfield;
  appearance: textfield;
}
</style>