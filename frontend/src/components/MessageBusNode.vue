<template>
  <BaseNode
    :id="id"
    :data="data"
    :min-width="320" 
    :min-height="190" 
    @resize="onResize"
  >
    <template #header>
      <div :style="data.labelStyle" class="node-label">
        Message Bus
      </div>
    </template>

    <!-- Mode selector -->
    <BaseDropdown label="Mode" v-model="mode" :options="modeOptions" />

    <!-- Topic entry -->
    <BaseInput label="Topic" :id="`${data.id}-topic`" type="text" v-model="topic" placeholder="e.g. updates" />

    <!-- Publish-mode usage hint -->
    <div v-if="mode === 'publish'" class="mt-2 p-2 text-xs bg-zinc-800 border border-dashed border-zinc-600 rounded-md text-zinc-400">
      <strong>Tip:</strong>  
      Upstream text may include  
      <code class="px-1 py-0.5 bg-zinc-700 rounded text-xs">TOPIC:</code> and <code class="px-1 py-0.5 bg-zinc-700 rounded text-xs">MESSAGE:</code> lines<br>
      to auto-populate topic &amp; payload, e.g.:<br>
      <pre class="mt-1 p-0 text-xs bg-transparent text-zinc-500">TOPIC: alerts\nMESSAGE: service restarted</pre>
    </div>

    <!-- Input / output handles -->
    <Handle style="width:12px;height:12px" v-if="data.hasInputs" type="target" :position="Position.Left" />
    <Handle style="width:12px;height:12px" v-if="data.hasOutputs" type="source" :position="Position.Right" />

  </BaseNode>
</template>

<script setup lang="ts">
import { Handle, Position } from '@vue-flow/core'
import BaseNode from '@/components/base/BaseNode.vue'
import BaseInput from '@/components/base/BaseInput.vue'
import BaseDropdown from '@/components/base/BaseDropdown.vue'
import { useMessageBus } from '@/composables/useMessageBus'

const props = defineProps({
  id: { type: String, required: true, default: 'MessageBus_0' },
  data: {
    type: Object,
    default: () => ({
      type: 'MessageBusNode',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: true,
      inputs: { mode: 'publish', topic: 'default' },
      outputs: { result: { output: '' } },
      style: {},
    })
  }
})

const emit = defineEmits(['resize', 'disable-zoom', 'enable-zoom'])

const {
  mode,
  topic,
  modeOptions,
  onResize
} = useMessageBus(props, emit)

</script>

<style scoped>
/* Scoped styles are removed as TailwindCSS is used via base components */
</style>
