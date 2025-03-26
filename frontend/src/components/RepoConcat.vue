<template>
  <div :style="data.style" class="node-container tool-node">
    <!-- Node label -->
    <div class="node-label">
      <input
        v-model="label"
        @change="updateNodeData"
        class="label-input"
        :style="data.labelStyle"
      />
    </div>

    <!-- Checkbox to enable/disable updating input from a connected source -->
    <div class="input-field">
      <input
        type="checkbox"
        :id="`${data.id}-update-from-source`"
        v-model="updateFromSource"
        @change="updateNodeData"
      />
      <label :for="`${data.id}-update-from-source`" class="input-label">
        Update Input from Source
      </label>
    </div>

    <!-- Input for Paths (comma separated) -->
    <div class="input-field">
      <label :for="`${data.id}-paths`" class="input-label">
        Paths (comma separated):
      </label>
      <input
        type="text"
        :id="`${data.id}-paths`"
        v-model="paths"
        @change="updateNodeData"
        class="input-text"
      />
    </div>

    <!-- Input for Types (comma separated) -->
    <div class="input-field">
      <label :for="`${data.id}-types`" class="input-label">
        Types (comma separated):
      </label>
      <input
        type="text"
        :id="`${data.id}-types`"
        v-model="types"
        @change="updateNodeData"
        class="input-text"
      />
    </div>

    <!-- Checkbox for Recursive -->
    <div class="input-field">
      <input
        type="checkbox"
        :id="`${data.id}-recursive`"
        v-model="recursive"
        @change="updateNodeData"
      />
      <label :for="`${data.id}-recursive`" class="input-label">
        Recursive
      </label>
    </div>

    <!-- Input for Ignore Pattern -->
    <div class="input-field">
      <label :for="`${data.id}-ignore`" class="input-label">
        Ignore Pattern:
      </label>
      <input
        type="text"
        :id="`${data.id}-ignore`"
        v-model="ignorePattern"
        @change="updateNodeData"
        class="input-text"
      />
    </div>

    <!-- Node connection handles -->
    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" id="input" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" id="output" />
  </div>
</template>

<script setup>
import { onMounted } from 'vue'
import { Handle } from '@vue-flow/core'
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

const emit = defineEmits(['update:data'])

// Use the repo concat composable
const {
  updateFromSource,
  label,
  paths,
  types,
  recursive,
  ignorePattern,
  updateNodeData,
  run
} = useRepoConcat(props, emit)

// Mount the run method on the node data so that VueFlow can invoke it.
onMounted(() => {
  if (!props.data.run) {
    props.data.run = run
  }
})
</script>

<style scoped>
.tool-node {
  --node-border-color: #777 !important;
  --node-bg-color: #1e1e1e !important;
  --node-text-color: #eee;
}

.node-label {
  color: var(--node-text-color);
  font-size: 16px;
  text-align: center;
  margin-bottom: 10px;
  font-weight: bold;
}

.input-field {
  margin-bottom: 8px;
}

/* Styling for standard text inputs */
.input-text {
  background-color: #333;
  border: 1px solid #666;
  color: #eee;
  padding: 4px;
  font-size: 12px;
  width: calc(100% - 8px);
  box-sizing: border-box;
}

/* You can keep using your existing label-input styling from your other components */
.label-input {
  background-color: #333;
  border: 1px solid #666;
  color: #eee;
  padding: 4px;
  font-size: 16px;
  width: 100%;
  box-sizing: border-box;
}
</style>
