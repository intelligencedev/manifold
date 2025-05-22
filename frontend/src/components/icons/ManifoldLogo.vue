<template>
  <img 
    src="/assets/logo.png" 
    alt="Manifold logo"
    width="56"
    height="56"
    v-bind="$attrs"
  />
</template>

<script setup lang="ts">
/*
 * Accept any native <img> attributes via v-bind="$attrs"
 * so parent components can size or animate the logo freely.
 */
defineOptions({ name: 'ManifoldLogo' });
</script>
