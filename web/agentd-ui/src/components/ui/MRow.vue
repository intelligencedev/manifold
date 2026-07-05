<template>
  <div
    class="flex items-center gap-3.5 border-b border-[rgb(var(--line-soft))] px-4 py-[13px] transition-colors last:border-b-0 hover:bg-surface-muted"
  >
    <div
      v-if="$slots.avatar"
      class="grid h-[30px] w-[30px] shrink-0 place-items-center rounded-md bg-[rgb(var(--accent-dim))] font-mono text-[11px] text-[rgb(var(--accent-hi))]"
    >
      <slot name="avatar" />
    </div>
    <div class="min-w-0 flex-1">
      <div
        v-if="$slots.title"
        class="min-w-0 text-sm font-medium text-foreground"
      >
        <slot name="title" />
      </div>
      <div
        v-if="$slots.meta"
        class="mt-1 min-w-0 text-xs leading-relaxed text-muted-foreground"
      >
        <slot name="meta" />
      </div>
    </div>
    <div v-if="$slots.status" class="shrink-0">
      <slot name="status" />
    </div>
    <div v-if="$slots.actions" class="shrink-0">
      <slot name="actions" />
    </div>
  </div>
</template>
