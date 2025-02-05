<template>
  <svg
    v-if="name === 'vertical'"
    width="24"
    height="24"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="2"
    stroke-linecap="round"
    stroke-linejoin="round"
  >
    <path d="M8 6h8" />
    <path d="M8 10h8" />
    <path d="M8 14h8" />
    <path d="M8 18h8" />
    <path d="M4 4l4 4-4 4" />
    <path d="M20 20l-4-4 4-4" />
  </svg>
  <svg
    v-else-if="name === 'horizontal'"
    width="24"
    height="24"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="2"
    stroke-linecap="round"
    stroke-linejoin="round"
  >
    <path d="M6 8v8" />
    <path d="M10 8v8" />
    <path d="M14 8v8" />
    <path d="M18 8v8" />
    <path d="M4 4l4 4-4 4" />
    <path d="M20 20l-4-4 4-4" />
  </svg>
</template>

<script setup>
defineProps({
  name: {
    type: String,
    required: true,
  },
});
</script>