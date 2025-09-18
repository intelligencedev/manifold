<template>
  <section class="flex min-h-[60vh] flex-col items-center justify-center gap-6 text-center">
    <div class="rounded-full border border-slate-800 bg-slate-900 px-4 py-2 text-xs font-semibold uppercase tracking-wide text-slate-500">
      404
    </div>
    <h1 class="text-4xl font-semibold text-white">We lost that route</h1>
    <p class="max-w-md text-sm text-slate-400">
      The page you are looking for might have been moved or renamed. Use the navigation to get back
      on track.
    </p>
    <RouterLink
      to="/"
      class="rounded-lg bg-emerald-500 px-4 py-2 text-sm font-semibold text-slate-950 transition hover:bg-emerald-400"
    >
      Go home
    </RouterLink>
  </section>
</template>

<script setup lang="ts">
import { RouterLink } from 'vue-router'
</script>
