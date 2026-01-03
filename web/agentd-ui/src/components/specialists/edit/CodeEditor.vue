<template>
  <div class="flex h-full min-h-0 flex-1 flex-col gap-2">
    <div v-if="showToolbar" class="flex flex-wrap items-center justify-between gap-2">
      <div class="text-xs text-subtle-foreground">
        <slot name="left" />
      </div>
      <div class="flex items-center gap-2">
        <label class="inline-flex items-center gap-2 text-xs text-subtle-foreground">
          <input v-model="wrap" type="checkbox" class="h-4 w-4" />
          <span>Wrap</span>
        </label>
        <button
          v-if="formatAction"
          type="button"
          class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-xs font-semibold text-subtle-foreground hover:border-border"
          @click="formatAction"
        >
          Format
        </button>
        <slot name="right" />
      </div>
    </div>

    <textarea
      :id="id"
      v-model="model"
      class="min-h-0 w-full flex-1 resize-none rounded border border-border/60 bg-surface px-3 py-2 text-sm text-foreground font-mono"
      :class="wrap ? 'whitespace-pre-wrap' : 'whitespace-pre'"
      :wrap="wrap ? 'soft' : 'off'"
      :placeholder="placeholder"
      @blur="$emit('blur')"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'

const props = withDefaults(
  defineProps<{ id?: string; placeholder?: string; showToolbar?: boolean; defaultWrap?: boolean; formatAction?: null | (() => void) }>(),
  { showToolbar: true, defaultWrap: true, formatAction: null },
)

defineEmits<{ blur: [] }>()

const model = defineModel<string>({ required: true })

const wrap = ref(!!props.defaultWrap)

watch(
  () => props.defaultWrap,
  (v) => {
    wrap.value = !!v
  },
)
</script>
