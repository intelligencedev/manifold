<template>
  <div class="mb-2">
    <label v-if="label" :for="id" class="block mb-1 text-sm text-gray-200">{{ label }}:</label>
    <select
      :id="id"
      class="w-full px-2 py-1.5 text-md border border-slate-600 rounded bg-slate-700 text-gray-200"
      :value="modelValue"
      @change="handleChange"
      v-bind="$attrs"
    >
      <option v-for="opt in options" :key="opt.value" :value="opt.value">
        {{ opt.label }}
      </option>
    </select>
  </div>
</template>

<script setup>
defineOptions({ inheritAttrs: false })

const props = defineProps({
  options: { type: Array, required: true },
  modelValue: { type: [String, Number], default: '' },
  label: { type: String, default: '' },
  id: { type: String, default: () => `dropdown-${Math.random().toString(36).substring(2, 9)}` }
})

const emit = defineEmits(['update:modelValue'])

function handleChange(event) {
  emit('update:modelValue', event.target.value)
}
</script>