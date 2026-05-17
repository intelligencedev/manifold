<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition duration-150"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition duration-100"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div
        v-if="open"
        class="fixed inset-0 z-50 flex items-end justify-center bg-black/60 backdrop-blur-sm sm:items-center"
        @click.self="$emit('close')"
        @keydown.escape="$emit('close')"
      >
        <div
          role="dialog"
          :aria-label="title"
          aria-modal="true"
          class="w-full max-w-lg rounded-2xl border border-border bg-surface p-6 shadow-2xl sm:mx-4"
        >
          <div v-if="title" class="mb-4 flex items-center justify-between">
            <h2 class="text-base font-semibold">{{ title }}</h2>
            <button class="rounded p-1 text-white/40 hover:text-white" @click="$emit('close')" aria-label="Close">✕</button>
          </div>
          <slot />
          <div v-if="$slots.footer" class="mt-6 flex justify-end gap-3">
            <slot name="footer" />
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
defineProps<{ open: boolean; title?: string }>();
defineEmits(["close"]);
</script>
