<template>
  <div :style="data.style" class="node-container tool-node">
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <!-- Query Input -->
    <div class="input-field">
      <label :for="`${data.id}-query`" class="input-label">Query:</label>
      <input :id="`${data.id}-query`" type="text" v-model="query" @change="updateNodeData" class="input-text" />
    </div>

    <!-- Result Size Input -->
    <div class="input-field">
      <label :for="`${data.id}-result_size`" class="input-label">
        Result Size:
      </label>
      <input :id="`${data.id}-result_size`" type="number" v-model.number="resultSize" @change="updateNodeData"
        class="input-number" />
    </div>

    <!-- Search Backend Selection -->
    <div class="input-field">
      <label :for="`${data.id}-search_backend`" class="input-label">
        Search Backend:
      </label>
      <select :id="`${data.id}-search_backend`" v-model="searchBackend" @change="updateNodeData" class="input-select">
        <option value="ddg">DuckDuckGo</option>
        <option value="sxng">SearXNG</option>
      </select>
    </div>

    <!-- SXNG URL Input (Conditional) -->
    <div v-if="searchBackend === 'sxng'" class="input-field">
      <label :for="`${data.id}-sxng_url`" class="input-label">
        SearXNG URL:
      </label>
      <input :id="`${data.id}-sxng_url`" type="text" v-model="sxngUrl" @change="updateNodeData" class="input-text" />
    </div>

    <!-- Input Handle (Optional) -->
    <Handle style="width:10px; height:10px" v-if="data.hasInputs" type="target" :position="Position.Left" id="input" />

    <!-- Output Handle -->
    <Handle style="width:10px; height:10px" v-if="data.hasOutputs" type="source" :position="Position.Right" id="output" />
  </div>
</template>

<script setup>
import { onMounted, watch, computed, ref } from 'vue'
import { Handle, useVueFlow, Position } from '@vue-flow/core'
const { findNode } = useVueFlow()

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'WebSearch_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'WebSearchNode',
      labelStyle: {},
      style: {},
      inputs: {
        query: 'ai news',
        result_size: 1,
        search_backend: 'ddg',
        sxng_url: 'https://searx.be',
      },
      outputs: {
        urls: [], // Now stores an array of URLs
      },
      hasInputs: true,
      hasOutputs: true,
      inputHandleColor: '#777',
      inputHandleShape: '50%',
      handleColor: '#777',
      outputHandleShape: '50%',
    }),
  },
})

const emit = defineEmits(['update:data'])

const query = ref(props.data.inputs?.query || '')
const resultSize = ref(props.data.inputs?.result_size || 1)
const searchBackend = ref(props.data.inputs?.search_backend || 'ddg')
const sxngUrl = ref(props.data.inputs?.sxng_url || 'https://searx.be')

const { getEdges, getNodes } = useVueFlow()

onMounted(() => {
  if (!props.data.run) {
    props.data.run = run
  }
})

async function run() {
  console.log('Running WebSearchNode:', props.id)


  const connectedTargetEdges = getEdges.value.filter(
    (edge) => edge.target === props.id
  );

  if (connectedTargetEdges.length > 0) {
    // Get the first connected edge
    const targetEdge = connectedTargetEdges[0];

    console.log('Connected target edge:', targetEdge);

    // Get the source node of the connected edge
    const sourceNode = findNode(targetEdge.source);

    console.log('Source node:', sourceNode);

    // Get the response value from the source node's outputs
    const response = sourceNode.data.outputs.result.output;

    console.log('Response:', response);

    // Update the query with the response value
    query.value = response;

    // Update the node data with the new input text
    updateNodeData();
  }

  // Update the node data with the new input text
  updateNodeData();

  console.log('Query value:', props.data.inputs.query) // Check if query is populated

  // Perform Web Search using your provided logic
  let webUrls = []
  let searchQuery = '' // Variable to store the search query
  try {
    const { query, result_size, search_backend, sxng_url } = props.data.inputs
    searchQuery = query // Capture the search query
    let apiURL = `http://localhost:8080/api/web-search?query=${encodeURIComponent(
      query
    )}&result_size=${result_size}&search_backend=${search_backend}`
    if (search_backend === 'sxng') {
      apiURL += `&sxng_url=${encodeURIComponent(sxng_url)}`
    }

    const response = await fetch(apiURL)

    if (!response.ok) {
      const errorText = await response.text()
      throw new Error(
        `Web Search API error (${response.status}): ${errorText}`
      )
    }

    const searchResults = await response.json()
    webUrls = searchResults // Assuming the API returns an array of URLs

    // Update Web Search Node outputs
    props.data.outputs = { urls: webUrls }
    //updateNodeData({ id: props.id, data: props.data })

    console.log('WebSearchNode run result:', props.data.outputs)
    return { response, result: props.data.outputs.result }
  } catch (error) {
    console.error('Error in WebSearchNode run:', error)
    props.data.error = error.message
    return { error: error.message }
  }
}

// ----- Reactivity for inputs -----
watch(
  [query, resultSize, searchBackend, sxngUrl],
  ([newQuery, newResultSize, newSearchBackend, newSxngUrl]) => {
    // Update internal props.data.inputs
    props.data.inputs.query = newQuery;
    props.data.inputs.result_size = newResultSize;
    props.data.inputs.search_backend = newSearchBackend;
    props.data.inputs.sxng_url = newSxngUrl;

    updateNodeData();
  },
  { deep: true }
);

watch(
  () => props.data,
  (newData) => {
    emit('update:data', { id: props.id, data: newData });
  },
  { deep: true }
);

// ----- Emit updates to parent -----
function updateNodeData() {
  emit('update:data', {
    id: props.id,
    data: {
      ...props.data,
      inputs: {
        query: query.value,
        result_size: resultSize.value,
        search_backend: searchBackend.value,
        sxng_url: sxngUrl.value,
      },
    },
  })
}
</script>

<style scoped>
/* Same styles as before, no changes needed */
.node-container {
  border: 3px solid var(--node-border-color) !important;
  background-color: var(--node-bg-color) !important;
  box-shadow: 0 2px 5px rgba(0, 0, 0, 0.2);
  padding: 15px;
  border-radius: 8px;
  color: var(--node-text-color);
  font-family: 'Roboto', sans-serif;
}

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

.input-text,
.input-number,
.input-select {
  background-color: #333;
  border: 1px solid #666;
  color: #eee;
  padding: 4px;
  font-size: 12px;
  width: calc(100% - 8px);
  box-sizing: border-box;
}

.input-number {
  width: 60px;
  /* Adjust as needed */
}

.handle-output {
  width: 12px;
  height: 12px;
  border: none;
  background-color: #777;
}
</style>