<template>
  <div
    class="inline-flex items-center gap-0.5 rounded-md border border-border bg-surface p-[3px]"
  >
    <button
      v-for="option in options"
      :key="option.value"
      type="button"
      :class="[
        'halo-focus rounded-sm border border-transparent px-3 py-1.5 font-sans text-[13px] font-medium text-muted-foreground transition-colors',
        option.value === modelValue
          ? 'bg-input text-foreground shadow-[inset_0_0_0_1px_rgb(var(--line-strong))]'
          : 'hover:bg-surface-muted hover:text-foreground',
      ]"
      @click="$emit('update:modelValue', option.value)"
    >
      {{ option.label }}
    </button>
  </div>
</template>

<script setup lang="ts">
defineEmits<{
  "update:modelValue": [value: string];
}>();

defineProps<{
  modelValue: string;
  options: Array<{ value: string; label: string }>;
}>();
</script>
