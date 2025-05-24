<template>
  <button
    :type="type"
    :disabled="disabled"
    class="inline-flex items-center justify-center px-3 py-1 bg-blue-600 text-white rounded text-sm transition-colors hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
    @click="$emit('click', $event)"
    v-bind="$attrs"
  >
    <slot>{{ label }}</slot>
  </button>
</template>

<script setup>
// Prevent automatic inheritance of attributes onto root element
defineOptions({ inheritAttrs: false })

const props = defineProps({
  label: { type: String, default: '' },
  type: { type: String, default: 'button' },
  disabled: { type: Boolean, default: false }
})

const emit = defineEmits(['click'])
</script>