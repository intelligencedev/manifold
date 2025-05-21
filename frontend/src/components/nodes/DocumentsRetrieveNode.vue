<template>
  <BaseNode
    :id="id"
    :data="data"
    :min-height="200"
    @resize="onResize"
  >
    <template #header>
      <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>
    </template>

    <BaseInput label="Retrieve Endpoint" v-model="retrieve_endpoint" />

    <BaseCheckbox
      :id="`${data.id}-update-from-source`"
      label="Update Input from Source"
      v-model="updateFromSource"
    />

    <BaseTextarea label="Prompt Text" v-model="prompt" rows="3" />

    <BaseInput label="Limit" type="number" v-model.number="limit" />

    <BaseSelect
      label="Merge Mode"
      v-model="merge_mode"
      :options="['union', 'intersect', 'weighted']"
    />

    <template v-if="merge_mode === 'weighted'">
      <BaseInput
        label="Vector Weight (Alpha)"
        type="number"
        step="0.1"
        min="0"
        max="1"
        v-model.number="alpha"
      />
      <BaseInput
        label="Keyword Weight (Beta)"
        type="number"
        step="0.1"
        min="0"
        max="1"
        v-model.number="beta"
      />
    </template>

    <BaseCheckbox
      :id="`${data.id}-return-full-docs`"
      label="Return Full Docs"
      v-model="return_full_docs"
    />

    <Handle v-if="data.hasInputs" type="target" position="left" style="width:12px;height:12px" />
    <Handle v-if="data.hasOutputs" type="source" position="right" style="width:12px;height:12px" />
  </BaseNode>
</template>

<script setup>
import { Handle } from '@vue-flow/core'
import BaseNode from '@/components/base/BaseNode.vue'
import BaseInput from '@/components/base/BaseInput.vue'
import BaseSelect from '@/components/base/BaseSelect.vue'
import BaseTextarea from '@/components/base/BaseTextarea.vue'
import BaseCheckbox from '@/components/base/BaseCheckbox.vue'
import { useDocumentsRetrieveNode } from '@/composables/useDocumentsRetrieveNode'

const props = defineProps({
  id: { type: String, required: true, default: 'DocumentsRetrieve_0' },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'DocumentsRetrieveNode',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: true,
      inputs: {
        retrieve_endpoint: 'http://localhost:8080/api/sefii/combined-retrieve',
        text: 'Enter prompt text here...',
        limit: 1,
        merge_mode: 'intersect',
        return_full_docs: true,
        alpha: 0.7,
        beta: 0.3
      },
      outputs: {
        result: { output: '' }
      },
      updateFromSource: true,
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '200px',
        height: '150px'
      }
    })
  }
})

const emit = defineEmits(['update:data', 'resize'])

const {
  retrieve_endpoint,
  prompt,
  limit,
  merge_mode,
  return_full_docs,
  updateFromSource,
  alpha,
  beta,
  onResize
} = useDocumentsRetrieveNode(props, emit)
</script>

<!-- Styling is handled by Tailwind and Base components -->
