<template>
  <BaseNode
    :id="id"
    :data="data"
    :min-height="140"
    @resize="onResize"
  >
    <template #header>
      <div :style="data.labelStyle" class="node-label">
        {{ data.type }}
      </div>
    </template>

    <BaseInput
      :id="`${data.id}-filename`"
      label="Filename"
      v-model="filename"
      class="mb-2"
    />

    <Handle
      v-if="data.hasInputs"
      type="target"
      position="left"
      id="input"
      style="width:12px;height:12px"
    />
    <Handle
      v-if="data.hasOutputs"
      type="source"
      position="right"
      id="output"
      style="width:12px;height:12px"
    />
  </BaseNode>
</template>

<script setup>
import { Handle } from '@vue-flow/core'
import BaseNode from '@/components/base/BaseNode.vue'
import BaseInput from '@/components/base/BaseInput.vue'
import { useSaveTextNode } from '@/composables/useSaveTextNode'

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'SaveText_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'saveTextNode',
      labelStyle: {},
      style: {},
      inputs: {
        filename: 'output.md',
        text: '',
      },
      hasInputs: true,
      hasOutputs: false,
      inputHandleColor: '#777',
      outputHandleShape: '50%',
      handleColor: '#777',
    }),
  },
});

const emit = defineEmits(['update:data', 'resize', 'disable-zoom', 'enable-zoom'])

const {
  filename,
  onResize
} = useSaveTextNode(props, emit)
</script>
