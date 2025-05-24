<template>
  <div
    v-if="show"
    class="absolute right-0 mt-2 w-52 bg-gray-800 rounded shadow-lg flex flex-col divide-y divide-gray-700 overflow-hidden z-20"
  >
    <slot />
  </div>
</template>

<script setup lang="ts">
const props = defineProps({
  show: { type: Boolean, default: false }
});
</script>
