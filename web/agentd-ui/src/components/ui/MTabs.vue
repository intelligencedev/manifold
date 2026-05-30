<template>
  <div class="flex border-b border-border">
    <button
      v-for="tab in tabs"
      :key="tab.value"
      type="button"
      :class="[
        'halo-focus border-b-2 px-4 py-3 text-sm font-medium transition-colors',
        tab.value === modelValue
          ? 'border-accent text-foreground'
          : 'border-transparent text-muted-foreground hover:text-foreground',
      ]"
      @click="$emit('update:modelValue', tab.value)"
    >
      {{ tab.label }}
    </button>
  </div>
</template>

<script setup lang="ts">
defineEmits<{
  "update:modelValue": [value: string];
}>();

defineProps<{
  modelValue: string;
  tabs: Array<{ value: string; label: string }>;
}>();
</script>
