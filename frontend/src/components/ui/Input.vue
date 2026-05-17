<template>
  <div class="flex flex-col gap-1">
    <label v-if="label" :for="id" class="text-xs font-medium text-white/60">{{ label }}<span v-if="required" class="ml-0.5 text-red-400">*</span></label>
    <input
      :id="id"
      v-bind="$attrs"
      :value="modelValue"
      :placeholder="placeholder"
      :disabled="disabled"
      :required="required"
      class="rounded-lg border border-border bg-black/20 px-3 py-2 text-sm text-foreground placeholder:text-white/30 focus:border-accent/50 focus:outline-none focus:ring-1 focus:ring-accent/30 disabled:opacity-40"
      @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
    />
    <p v-if="error" role="alert" class="text-xs text-red-400">{{ error }}</p>
    <p v-else-if="hint" class="text-xs text-white/40">{{ hint }}</p>
  </div>
</template>

<script setup lang="ts">
defineOptions({ inheritAttrs: false });
const id = `input-${Math.random().toString(36).slice(2)}`;
defineProps<{ modelValue?: string; label?: string; placeholder?: string; disabled?: boolean; required?: boolean; error?: string; hint?: string }>();
defineEmits(["update:modelValue"]);
</script>
