<template>
  <div class="overflow-hidden rounded-lg border border-border bg-surface">
    <table class="w-full border-collapse text-left text-sm">
      <thead>
        <tr>
          <slot name="head" />
        </tr>
      </thead>
      <tbody>
        <slot />
      </tbody>
    </table>
  </div>
</template>

<style scoped>
:deep(th) {
  border-bottom: 1px solid rgb(var(--color-border));
  color: rgb(var(--color-faint-foreground));
  font-family: var(--font-mono);
  font-size: 11px;
  font-weight: 500;
  letter-spacing: 0.1em;
  padding: 9px 12px;
  text-transform: uppercase;
}

:deep(td) {
  border-bottom: 1px solid rgb(var(--line-soft));
  color: rgb(var(--color-muted-foreground));
  padding: 11px 12px;
}

:deep(tbody tr:last-child td) {
  border-bottom: 0;
}

:deep(tbody tr:hover td) {
  background: rgb(var(--color-surface-muted));
  color: rgb(var(--color-foreground));
}
</style>
