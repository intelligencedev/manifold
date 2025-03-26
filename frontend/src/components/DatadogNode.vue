<template>
  <div :style="data.style" class="node-container tool-node datadog-node">
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <!-- API Key Input with Toggle -->
    <div class="input-field">
      <BaseInput
        :id="`${data.id}-api_key`"
        label="API Key"
        v-model="apiKey"
        :type="showApiKey ? 'text' : 'password'"
      >
        <template #suffix>
          <BaseTogglePassword v-model="showApiKey" />
        </template>
      </BaseInput>
    </div>

    <!-- Application Key Input with Toggle -->
    <div class="input-field">
      <BaseInput
        :id="`${data.id}-app_key`"
        label="Application Key"
        v-model="appKey"
        :type="showAppKey ? 'text' : 'password'"
      >
        <template #suffix>
          <BaseTogglePassword v-model="showAppKey" />
        </template>
      </BaseInput>
    </div>

    <!-- Datadog Site Selection -->
    <BaseSelect
      :id="`${data.id}-site`"
      label="Datadog Site"
      v-model="site"
      :options="[
        { value: 'datadoghq.com', label: 'US1 (datadoghq.com)' },
        { value: 'datadoghq.eu', label: 'EU (datadoghq.eu)' },
        { value: 'us3.datadoghq.com', label: 'US3 (us3.datadoghq.com)' },
        { value: 'us5.datadoghq.com', label: 'US5 (us5.datadoghq.com)' },
        { value: 'ap1.datadoghq.com', label: 'AP1 (ap1.datadoghq.com)' },
        { value: 'ddog-gov.com', label: 'US1-FED (ddog-gov.com)' }
      ]"
    />

    <!-- Operation Selection -->
    <BaseSelect
      :id="`${data.id}-operation`"
      label="Operation"
      v-model="operation"
      :options="[
        { value: 'getLogs', label: 'Get Logs' },
        { value: 'getMetrics', label: 'Get Metrics' },
        { value: 'listMonitors', label: 'List Monitors' },
        { value: 'listIncidents', label: 'List Incidents' },
        { value: 'getEvents', label: 'Get Events' }
      ]"
    />

    <!-- Query Input (for logs, metrics, events) -->
    <BaseTextarea
      v-if="operation === 'getLogs' || operation === 'getMetrics' || operation === 'getEvents'"
      :id="`${data.id}-query`"
      label="Query"
      v-model="query"
    />

    <!-- Time Range Inputs -->
    <div v-if="operation === 'getLogs' || operation === 'getMetrics' || operation === 'getEvents'">
      <BaseInput
        :id="`${data.id}-fromTime`"
        label="From Time"
        v-model="fromTime"
        placeholder="e.g., now-15m"
      />
      <BaseInput
        :id="`${data.id}-toTime`"
        label="To Time"
        v-model="toTime"
        placeholder="e.g., now"
      />
    </div>

    <!-- Input/Output Handles -->
    <Handle 
      style="width:12px; height:12px"
      v-if="data.hasInputs"
      type="target"
      position="left"
      id="input"
    />

    <Handle 
      style="width:12px; height:12px"
      v-if="data.hasOutputs"
      type="source"
      position="right"
      id="output"
    />
  </div>
</template>

<script setup>
import { Handle, useVueFlow } from '@vue-flow/core';
import { watch } from 'vue';
import BaseInput from '@/components/base/BaseInput.vue';
import BaseSelect from '@/components/base/BaseSelect.vue';
import BaseTextarea from '@/components/base/BaseTextarea.vue';
import BaseTogglePassword from '@/components/base/BaseTogglePassword.vue';
import { useDatadogNode } from '@/composables/useDatadogNode';

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

// Get Vue Flow instance
const vueFlowInstance = useVueFlow();

// Use the composable to manage state and functionality
const {
  // State refs
  showApiKey,
  showAppKey,
  
  // Computed properties
  apiKey,
  appKey,
  site,
  operation,
  query,
  fromTime,
  toTime
} = useDatadogNode(props, vueFlowInstance);

// Watch for changes in props.data and emit update events
watch(
  () => props.data,
  (newData) => {
    emit('update:data', { id: props.id, data: newData });
  },
  { deep: true }
);
</script>

<style scoped>
@import '@/assets/css/nodes.css';

.datadog-node {
  --node-border-color: #fd7702 !important;
  --node-bg-color: #1e1e1e !important;
  --node-text-color: #eee;
  padding: 15px;
  box-shadow: 0 2px 5px rgba(0, 0, 0, 0.2);
  border: 3px solid var(--node-border-color) !important;
}

.input-field {
  position: relative; /* For positioning the toggle button */
}
</style>