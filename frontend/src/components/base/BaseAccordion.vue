<template>
  <details class="accordion" :open="initiallyOpen">
    <summary class="accordion-summary">{{ title }}</summary>
    <div class="accordion-content">
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

<style scoped>
.accordion {
  width: 100%;
  margin-bottom: 10px;
  border: 1px solid var(--accordion-border-color, #666);
  border-radius: 4px;
  background-color: var(--accordion-bg-color, #444);
  color: var(--accordion-text-color, #eee);
  box-sizing: border-box;
}

.accordion-summary {
  cursor: pointer;
  padding: 8px;
  font-weight: bold;
  user-select: none;
}

.accordion-content {
  padding: 0 8px 8px 8px;
}
</style>