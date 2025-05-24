<template>
  <BaseNode :id="id" :data="data" :min-height="200">
    <template #header>
      <div :style="data.labelStyle" class="text-center mb-2 font-bold text-gray-200">{{ data.type }}</div>
    </template>

    <!-- Filename Input -->
    <BaseInput
      :id="`${data.id}-filepath`"
      label="Filepath"
      v-model="filepath"
      @update:modelValue="updateNodeData"
    />

    <!-- Checkbox to enable/disable updating input from a connected source -->
    <div class="mb-2 flex items-center">
      <input
        type="checkbox"
        :id="`${data.id}-update-from-source`"
        v-model="updateFromSource"
        @change="updateNodeData"
        class="mr-2 form-checkbox text-blue-500"
      />
      <label :for="`${data.id}-update-from-source`" class="text-sm text-gray-200">
        Update Input from Source
      </label>
    </div>

    <!-- Image Preview -->
    <div v-if="isImage || data.isImage" class="mt-2 mb-2 border border-gray-600 p-2 bg-gray-800 rounded overflow-hidden min-h-[100px] flex items-center justify-center">
      <!-- Pan-and-scan slices -->
      <template v-if="data.outputs?.result?.slices">
        <div
          v-for="(slice, index) in data.outputs.result.slices"
          :key="index"
          class="w-full text-center"
        >
          <img :src="slice.dataUrl" :alt="`Image slice ${index + 1}`" class="max-w-full max-h-52 object-contain mx-auto block" />
        </div>
      </template>
      <!-- Single image -->
      <template v-else-if="data.outputs?.result?.dataUrl">
        <div class="w-full text-center">
          <img :src="data.outputs.result.dataUrl" alt="Image preview" class="max-w-full max-h-52 object-contain mx-auto block" />
        </div>
      </template>
      <template v-else>
        <div class="text-gray-400 italic text-center">
          Image will appear here when loaded
        </div>
      </template>
    </div>
    <Handle v-if="data.hasInputs" type="target" position="left" style="width:12px;height:12px" />
    <Handle v-if="data.hasOutputs" type="source" position="right" style="width:12px;height:12px" />
  </BaseNode>
</template>

<script setup>
import { watch, onMounted } from 'vue'
import { Handle } from '@vue-flow/core'
import { useOpenFileNode } from '@/composables/useOpenFileNode'
import BaseNode from '@/components/base/BaseNode.vue'
import BaseInput from '@/components/base/BaseInput.vue'

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'OpenFile_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'openFileNode',
      labelStyle: {},
      style: {},
      inputs: {
        filepath: 'input.txt',
        text: '',
      },
      outputs: {
        result: { output: '' }  // Initialize with empty result
      },
      hasInputs: true,
      hasOutputs: true,
      inputHandleColor: '#777',
      outputHandleShape: '50%',
      handleColor: '#777',
      updateFromSource: true,
      isImage: false,
    }),
  },
});

const emit = defineEmits(['update:data']);

// Use the open file node composable
const { filepath, updateFromSource, updateNodeData, run, isImage } = useOpenFileNode(props, emit)

watch(
  () => props.data,
  (newData) => {
    emit('update:data', { id: props.id, data: newData });
  },
  { deep: true }
);

onMounted(() => {
  if (!props.data.run) {
    props.data.run = run;
  }
});
</script>
