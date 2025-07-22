<template>
  <BaseNode :id="id" :data="data" :min-height="220" @resize="onResize">
    <template #header>
      <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>
    </template>


    <div class="relative mb-2">
      <BaseInput
        :id="`${data.id}-conn`"
        label="Connection String"
        v-model="connString"
        :type="showConnString ? 'text' : 'password'"
      >
        <template #suffix>
          <BaseTogglePassword v-model="showConnString" />
        </template>
      </BaseInput>
    </div>

    <BaseTextarea
      :id="`${data.id}-query`"
      label="SQL Query"
      v-model="query"
      rows="4"
      class="mb-2"
      autocomplete="off"
    />

    <Handle v-if="data.hasInputs" type="target" position="left" id="input" style="width:12px;height:12px" />
    <Handle v-if="data.hasOutputs" type="source" position="right" id="output" style="width:12px;height:12px" />
  </BaseNode>
</template>


<script setup>
import { ref, onMounted } from 'vue'
import { Handle } from '@vue-flow/core'
import BaseNode from '@/components/base/BaseNode.vue'
import BaseInput from '@/components/base/BaseInput.vue'
import BaseTextarea from '@/components/base/BaseTextarea.vue'
import BaseTogglePassword from '@/components/base/BaseTogglePassword.vue'
import { usePostgresNode } from '@/composables/usePostgresNode'

const props = defineProps({
  id: { type: String, default: 'PostgresNode_0' },
  data: {
    type: Object,
    default: () => ({
      type: 'PostgresNode',
      labelStyle: {},
      style: {},
      inputs: { conn_string: '', query: '' },
      outputs: { result: { output: '' } },
      hasInputs: true,
      hasOutputs: true,
      inputHandleColor: '#777',
      outputHandleShape: '50%',
      handleColor: '#777'
    })
  }
})

const emit = defineEmits(['update:data', 'resize'])


const { connString, query, onResize, run } = usePostgresNode(props, emit)
const showConnString = ref(false)

onMounted(() => {
  if (!props.data.run) props.data.run = run
})
</script>
