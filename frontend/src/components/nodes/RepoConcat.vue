<template>
  <BaseNode :id="id" :data="data" :min-height="350" @resize="onResize">
    <template #header>
      <div :style="data.labelStyle" class="node-label text-base font-semibold mb-2">{{ data.type }}</div>
    </template>

    <BaseCheckbox
      :id="`${data.id}-update-from-source`"
      v-model="updateFromSource"
      label="Update Input from Source"
    />

    <BaseInput
      :id="`${data.id}-paths`"
      label="Paths (comma separated)"
      v-model="paths"
    />

    <BaseInput
      :id="`${data.id}-types`"
      label="Types (comma separated)"
      v-model="types"
    />

    <BaseCheckbox
      :id="`${data.id}-recursive`"
      v-model="recursive"
      label="Recursive"
    />

    <BaseInput
      :id="`${data.id}-ignore`"
      label="Ignore Pattern"
      v-model="ignorePattern"
    />

    <Handle style="width:12px;height:12px" v-if="data.hasInputs" type="target" position="left" id="input" />
    <Handle style="width:12px;height:12px" v-if="data.hasOutputs" type="source" position="right" id="output" />
  </BaseNode>
</template>

<script setup>
import { onMounted } from 'vue'
import { Handle } from '@vue-flow/core'
import BaseNode from '@/components/base/BaseNode.vue'
import BaseInput from '@/components/base/BaseInput.vue'
import BaseCheckbox from '@/components/base/BaseCheckbox.vue'
import { useRepoConcat } from '@/composables/useRepoConcat'

// Define the component props; note that we set default node data values for our inputs.
const props = defineProps({
  id: {
    type: String,
    required: false,
    default: 'RepoConcat_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      style: {},
      labelStyle: {},
      type: 'RepoConcatNode',
      inputs: {
        // Defaults: a single path and a comma‐separated list of file types
        paths: ".",
        types: ".go, .md, .vue, .js, .css, .html",
        recursive: false,
        ignorePattern: ""
      },
      outputs: {},
      hasInputs: true,
      hasOutputs: true,
      updateFromSource: true,
    }),
  },
})

const emit = defineEmits(['update:data', 'resize'])

// Use the repo concat composable
const {
  updateFromSource,
  label,
  paths,
  types,
  recursive,
  ignorePattern,
  updateNodeData,
  run,
  onResize
} = useRepoConcat(props, emit)

// Mount the run method on the node data so that VueFlow can invoke it.
onMounted(() => {
  if (!props.data.run) {
    props.data.run = run
  }
})
</script>
