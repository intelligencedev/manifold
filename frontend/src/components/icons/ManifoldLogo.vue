<template>
  <!-- Manifold logo – three connected nodes forming an “M” -->
  <svg
    xmlns="http://www.w3.org/2000/svg"
    viewBox="0 0 48 48"
    role="img"
    aria-label="Manifold logo"
    class="w-32 h-6"
    v-bind="$attrs"
  >
    <defs>
      <!-- subtle blue-cyan gradient that pops on dark mode -->
      <linearGradient id="manifoldGradient" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0%" stop-color="#3b82f6" />
        <stop offset="100%" stop-color="#06b6d4" />
      </linearGradient>
    </defs>

    <!-- left, top, right nodes -->
    <circle cx="6"  cy="42" r="5" stroke="url(#manifoldGradient)" stroke-width="4" fill="none" />
    <circle cx="24" cy="6"  r="5" stroke="url(#manifoldGradient)" stroke-width="4" fill="none" />
    <circle cx="42" cy="42" r="5" stroke="url(#manifoldGradient)" stroke-width="4" fill="none" />

    <!-- connecting strokes (form the M-shape) -->
    <path d="M6 42 L24 6 L42 42" stroke="url(#manifoldGradient)" stroke-width="4" stroke-linecap="round" fill="none" />
  </svg>
</template>

<script setup lang="ts">
/*
 * Accept any native <svg> attributes via v-bind="$attrs"
 * so parent components can size or animate the logo freely.
 */
defineOptions({ name: 'ManifoldLogo' });
</script>
