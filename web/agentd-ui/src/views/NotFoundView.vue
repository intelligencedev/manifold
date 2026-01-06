<template>
  <section
    class="flex min-h-[60vh] flex-col items-center justify-center gap-6 text-center"
  >
    <div
      class="rounded-full border border-border/70 bg-surface px-4 py-2 text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
    >
      404
    </div>
    <h1 class="text-4xl font-semibold text-foreground">We lost that route</h1>
    <p class="max-w-md text-sm text-subtle-foreground">
      The page you are looking for might have been moved or renamed. Use the
      navigation to get back on track.
    </p>
    <RouterLink
      to="/"
      class="rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-foreground transition hover:bg-accent/90"
    >
      Go home
    </RouterLink>
  </section>
</template>

<script setup lang="ts">
import { RouterLink } from "vue-router";
</script>
