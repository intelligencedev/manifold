<template>
  <details class="w-full mb-2 border border-slate-600 rounded bg-zinc-900 text-gray-200 box-border" :open="initiallyOpen">
    <summary class="cursor-pointer p-2 font-bold select-none">{{ title }}</summary>
    <div class="px-2 pb-2">
      <slot></slot>
    </div>
  </details>
</template>

<script setup>
defineProps({
  title: {
    type: String,
    required: true
  },
  initiallyOpen: {
    type: Boolean,
    default: true
  }
})
</script>