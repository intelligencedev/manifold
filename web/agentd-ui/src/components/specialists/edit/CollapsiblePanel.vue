<template>
  <div class="rounded-[14px] border border-border/60 bg-surface">
    <button
      type="button"
      class="flex w-full items-center justify-between gap-3 px-4 py-3 text-left"
      :aria-expanded="open ? 'true' : 'false'"
      @click="toggle"
    >
      <div class="min-w-0">
        <p class="text-sm font-semibold text-foreground">{{ title }}</p>
        <p v-if="helper" class="mt-0.5 text-xs text-subtle-foreground">{{ helper }}</p>
      </div>
      <svg viewBox="0 0 20 20" class="h-4 w-4 shrink-0 text-subtle-foreground" aria-hidden="true">
        <path
          d="M6 8l4 4 4-4"
          fill="none"
          stroke="currentColor"
          stroke-width="1.6"
          stroke-linecap="round"
          stroke-linejoin="round"
          :transform="open ? 'rotate(180 10 10)' : undefined"
        />
      </svg>
    </button>
    <div v-show="open" class="px-4 pb-4">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'

const props = withDefaults(defineProps<{ title: string; helper?: string; defaultOpen?: boolean }>(), {
  defaultOpen: false,
})

const model = defineModel<boolean>({ default: false })

onMounted(() => {
  if (props.defaultOpen) {
    model.value = true
  }
})

const open = computed(() => model.value)

function toggle() {
  model.value = !model.value
}
</script>
