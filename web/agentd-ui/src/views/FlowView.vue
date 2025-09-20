<template>
  <div class="h-[75vh] max-h-[75vh] rounded-xl border border-slate-800 overflow-hidden">
    <VueFlow
      v-model:nodes="nodes"
      v-model:edges="edges"
      :fit-view="true"
      :zoom-on-scroll="true"
      :zoom-on-double-click="false"
      class="bg-slate-900/60"
    >
      <Background />
      <Controls />
      <MiniMap />
    </VueFlow>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { VueFlow, Background, Controls, MiniMap, type Node, type Edge } from '@vue-flow/core'

const nodes = ref<Node[]>([
  { id: '1', type: 'input', position: { x: 50, y: 25 }, data: { label: 'Start' } },
  { id: '2', position: { x: 250, y: 150 }, data: { label: 'Task' } },
  { id: '3', type: 'output', position: { x: 500, y: 50 }, data: { label: 'Done' } }
])

const edges = ref<Edge[]>([
  { id: 'e1-2', source: '1', target: '2' },
  { id: 'e2-3', source: '2', target: '3' }
])
</script>

<style>
/* ensure flow canvas fills area */
.vue-flow__container { height: 100%; }
</style>
