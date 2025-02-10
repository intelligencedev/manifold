<template>
  <div :style="data.style" class="node-container tool-node datadog-node">
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <div class="input-field">
      <label :for="`${data.id}-api_key`" class="input-label">API Key:</label>
      <input
        :id="`${data.id}-api_key`"
        :type="showApiKey ? 'text' : 'password'"
        v-model="apiKey"
        class="input-text"
      />
      <button @click="showApiKey = !showApiKey" class="toggle-password">
        <span v-if="showApiKey">👁️</span>
        <span v-else>🙈</span>
      </button>
    </div>

    <div class="input-field">
      <label :for="`${data.id}-app_key`" class="input-label">Application Key:</label>
      <input
        :id="`${data.id}-app_key`"
        :type="showAppKey ? 'text' : 'password'"
        v-model="appKey"
        class="input-text"
      />
      <button @click="showAppKey = !showAppKey" class="toggle-password">
        <span v-if="showAppKey">👁️</span>
        <span v-else>🙈</span>
      </button>
    </div>

    <div class="input-field">
      <label :for="`${data.id}-site`" class="input-label">Datadog Site:</label>
      <select
        :id="`${data.id}-site`"
        v-model="site"
        class="input-select"
      >
        <option value="datadoghq.com">US1 (datadoghq.com)</option>
        <option value="datadoghq.eu">EU (datadoghq.eu)</option>
        <option value="us3.datadoghq.com">US3 (us3.datadoghq.com)</option>
        <option value="us5.datadoghq.com">US5 (us5.datadoghq.com)</option>
        <option value="ap1.datadoghq.com">AP1 (ap1.datadoghq.com)</option>
        <option value="ddog-gov.com">US1-FED (ddog-gov.com)</option>
      </select>
    </div>

    <div class="input-field">
      <label :for="`${data.id}-operation`" class="input-label">Operation:</label>
      <select
        :id="`${data.id}-operation`"
        v-model="operation"
        class="input-select"
      >
        <option value="getLogs">Get Logs</option>
        <option value="getMetrics">Get Metrics</option>
        <option value="listMonitors">List Monitors</option>
        <option value="listIncidents">List Incidents</option>
        <option value="getEvents">Get Events</option>
      </select>
    </div>

    <div
      class="input-field"
      v-if="operation === 'getLogs' || operation === 'getMetrics' || operation === 'getEvents'"
    >
      <label :for="`${data.id}-query`" class="input-label">Query:</label>
      <textarea
        :id="`${data.id}-query`"
        v-model="query"
        class="input-textarea"
      ></textarea>
    </div>

    <div
      class="input-field"
      v-if="operation === 'getLogs' || operation === 'getMetrics' || operation === 'getEvents'"
    >
      <label :for="`${data.id}-fromTime`" class="input-label">From Time (e.g., 'now-15m'):</label>
      <input
        :id="`${data.id}-fromTime`"
        type="text"
        v-model="fromTime"
        class="input-text"
        placeholder="e.g., now-15m"
      />
    </div>

    <div
      class="input-field"
      v-if="operation === 'getLogs' || operation === 'getMetrics' || operation === 'getEvents'"
    >
      <label :for="`${data.id}-toTime`" class="input-label">To Time (e.g., 'now'):</label>
      <input
        :id="`${data.id}-toTime`"
        type="text"
        v-model="toTime"
        class="input-text"
        placeholder="e.g., now"
      />
    </div>

    <Handle style="width:10px; height:10px"
      v-if="data.hasInputs"
      type="target"
      position="left"
      id="input"
    />

    <Handle style="width:10px; height:10px"
      v-if="data.hasOutputs"
      type="source"
      position="right"
      id="output"
    />
  </div>
</template>

<script setup>
import { Handle, useVueFlow } from '@vue-flow/core';
import { watch, ref, onMounted, computed } from 'vue';
const { getEdges, findNode, zoomIn, zoomOut, updateNodeData } = useVueFlow()

onMounted(() => {
  console.log("[Node Type: " + props.data.type + "] onMounted - Assigning run function");
  if (!props.data.run) {
    props.data.run = run;
  }
});

// The main function to execute the Datadog API call
async function run(queryOverride = null) {
  console.log('Running DatadogNode:', props.id);

  const connectedTargetEdges = getEdges.value.filter(
      (edge) => edge.target === props.id
  );

  let llmQuery = null;

  if (connectedTargetEdges.length > 0) {
    const targetEdge = connectedTargetEdges[0];
    const sourceNode = findNode(targetEdge.source);
    if (sourceNode && sourceNode.data.outputs.result.output) {
      llmQuery = sourceNode.data.outputs.result.output;
    }
  }

  queryOverride = queryOverride || llmQuery || query.value;

  // Update the query text in the node
  query.value = queryOverride;

  const requestBody = {
    apiKey: apiKey.value,
    appKey: appKey.value,
    site: site.value,
    operation: operation.value,
    // if llmQuery is defined, use it, otherwise use the query input
    query: queryOverride, 
    fromTime: fromTime.value,
    toTime: toTime.value,
  };

  try {
    // TODO: Make this backend endpoint configurable
    const response = await fetch('http://localhost:8080/api/datadog', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(requestBody),
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`Backend error (${response.status}): ${errorText}`);
    }

    const data = await response.json();
    console.log('Datadog API response:', data);

    props.data.outputs = {
      result: {
        output: JSON.stringify(data.result.output, null, 2),
      },
    };

    return { result: props.data.outputs.result };
  } catch (error) {
    console.error('Error in DatadogNode run:', error);
    props.data.error = error.message;
    return { error: error.message };
  }
}

// Define props for the component
const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'Datadog_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'datadogNode',
      labelStyle: {},
      style: {},
      inputs: {
        apiKey: '...',
        appKey: '...',
        site: 'datadoghq.com',
        operation: 'getMetrics',
        query: 'avg:system.cpu.user{*} by {host}',
        fromTime: 'now-15m',
        toTime: 'now',
      },
      outputs: {},
      hasInputs: true,
      hasOutputs: true,
      inputHandleColor: '#777',
      inputHandleShape: '50%',
      handleColor: '#777',
      outputHandleShape: '50%',
    }),
  },
});

// Define emitted events
const emit = defineEmits(['update:data']);

// Define computed properties for input fields
const apiKey = computed({
  get: () => props.data.inputs?.apiKey || '',
  set: (value) => {
    props.data.inputs.apiKey = value;
    updateNodeData();
  },
});

const appKey = computed({
  get: () => props.data.inputs?.appKey || '',
  set: (value) => {
    props.data.inputs.appKey = value;
    updateNodeData();
  },
});

const site = computed({
  get: () => props.data.inputs?.site || 'datadoghq.com',
  set: (value) => {
    props.data.inputs.site = value;
    updateNodeData();
  },
});

const operation = computed({
  get: () => props.data.inputs.operation || 'getLogs',
  set: (value) => {
    props.data.inputs.operation = value;
  },
});

const query = computed({
  get: () => props.data.inputs.query,
  set: (value) => {
    props.data.inputs.query = value;
  },
});

const fromTime = computed({
  get: () => props.data.inputs.fromTime,
  set: (value) => {
    props.data.inputs.fromTime = value;
  },
});

const toTime = computed({
  get: () => props.data.inputs.toTime,
  set: (value) => {
    props.data.inputs.toTime = value;
  },
});

const showApiKey = ref(false);
const showAppKey = ref(false);

// Watch for changes in props.data and update local state accordingly
watch(
  () => props.data,
  (newData) => {
    apiKey.value = newData.inputs?.apiKey || '';
    appKey.value = newData.inputs?.appKey || '';
    site.value = newData.inputs?.site || 'datadoghq.com';
    operation.value = newData.inputs?.operation || 'getLogs';
    query.value = newData.inputs?.query || 'source:kubernetes';
    fromTime.value = newData.inputs?.fromTime || 'now-15m';
    toTime.value = newData.inputs?.toTime || 'now';
    emit('update:data', { id: props.id, data: newData });
  },
  { deep: true }
);
</script>

<style scoped>
.node-container {
  border: 3px solid var(--node-border-color) !important;
  background-color: var(--node-bg-color) !important;
  box-shadow: 0 2px 5px rgba(0, 0, 0, 0.2);
  padding: 15px;
  border-radius: 8px;
  color: var(--node-text-color);
  font-family: 'Roboto', sans-serif;
}

.datadog-node {
  --node-border-color: #fd7702 !important;
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
  position: relative; /* For positioning the toggle button */
}

.input-text,
.input-select,
.input-textarea {
  background-color: #333;
  border: 1px solid #666;
  color: #eee;
  padding: 4px;
  font-size: 12px;
  width: calc(100% - 8px);
  box-sizing: border-box;
}

.input-textarea {
  min-height: 60px;
}

.handle-input,
.handle-output {
  width: 12px;
  height: 12px;
  border: none;
  background-color: #777;
}

.toggle-password {
  position: absolute;
  right: 10px;
  top: 50%;
  transform: translateY(-50%);
  background: none;
  border: none;
  padding: 0;
  cursor: pointer;
}
</style>