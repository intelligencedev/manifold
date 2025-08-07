<template>
  <BaseNode
    :id="id"
    :data="data"
    :min-height="650"
    @resize="onResize"
  >
    <template #header>
      <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>
    </template>

    <BaseInput label="Retrieve Endpoint" v-model="retrieve_endpoint" />

    <BaseSelect
      label="Retrieval Mode"
      v-model="retrieval_mode"
      :options="[
        'combined',
        'contextual', 
        'summary',
        'neighbors'
      ]"
    />

    <div class="mode-description text-xs text-gray-400 mb-3">
      <span v-if="retrieval_mode === 'combined'">Hybrid vector + keyword search with document reconstruction</span>
      <span v-else-if="retrieval_mode === 'contextual'">Search with automatic neighboring chunk context</span>
      <span v-else-if="retrieval_mode === 'summary'">Search prioritizing chunk summaries for high-level concepts</span>
      <span v-else-if="retrieval_mode === 'neighbors'">Retrieve context around a specific chunk ID</span>
    </div>

    <BaseCheckbox
      :id="`${data.id}-update-from-source`"
      label="Update Input from Source"
      v-model="updateFromSource"
    />

    <!-- Query input for all modes except neighbors -->
    <BaseTextarea 
      v-if="retrieval_mode !== 'neighbors'"
      label="Query Text" 
      v-model="prompt" 
      rows="3" 
    />

    <!-- Chunk ID input for neighbors mode -->
    <BaseInput 
      v-if="retrieval_mode === 'neighbors'"
      label="Chunk ID" 
      type="number" 
      v-model.number="chunk_id" 
    />
    <div v-if="retrieval_mode === 'neighbors'" class="text-xs text-gray-400 mb-2">
      Enter the ID of a specific chunk to retrieve its surrounding context
    </div>

    <BaseInput label="Limit" type="number" v-model.number="limit" />

    <!-- Combined mode specific options -->
    <template v-if="retrieval_mode === 'combined'">
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
        label="Return Full Documents"
        v-model="return_full_docs"
      />
    </template>

    <!-- Contextual and neighbors mode options -->
    <template v-if="retrieval_mode === 'contextual' || retrieval_mode === 'neighbors'">
      <BaseInput
        label="Context Window"
        type="number"
        min="0"
        max="10"
        v-model.number="context_window"
        placeholder="Number of chunks before/after"
      />
      <div class="text-xs text-gray-400 mb-2">
        Context window of {{ context_window }} includes {{ context_window }} chunks before and {{ context_window }} chunks after the match
      </div>

      <BaseCheckbox
        :id="`${data.id}-include-full-doc`"
        label="Include Full Document"
        v-model="include_full_doc"
      />
    </template>

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
import { useConfigStore } from '@/stores/configStore';
import { getApiEndpoint, API_PATHS } from '@/utils/endpoints';

const configStore = useConfigStore();
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
        retrieve_endpoint: 'http://localhost:8080/api/sefii/combined-retrieve', // Will be updated dynamically
        text: 'Enter prompt text here...',
        limit: 1,
        merge_mode: 'intersect',
        return_full_docs: true,
        alpha: 0.7,
        beta: 0.3,
        retrieval_mode: 'combined',
        context_window: 2,
        include_full_doc: false,
        chunk_id: null
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
        width: '280px',
        height: '400px'
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
  retrieval_mode,
  context_window,
  include_full_doc,
  chunk_id,
  onResize
} = useDocumentsRetrieveNode(props, emit)
</script>
