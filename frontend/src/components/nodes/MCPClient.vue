<template>
  <BaseNode
    :id="id"
    :data="data"
    :min-height="420"
    :style="customStyle"
    @resize="onResize"
    @mouseenter="$emit('disable-zoom')"
    @mouseleave="$emit('enable-zoom')"
  >
    <template #header>
      <div class="node-label font-bold text-center text-base text-white">MCP Client</div>
    </template>

    <!-- Mode Selection -->
    <div class="flex gap-4 justify-center mb-3 border-b border-zinc-700 pb-2">
      <label class="flex items-center gap-2 text-xs text-zinc-200 cursor-pointer">
        <input type="radio" v-model="mode" value="list" :id="`${data.id}-mode-list`" class="accent-blue-500" />
        <span>List Tools</span>
      </label>
      <label class="flex items-center gap-2 text-xs text-zinc-200 cursor-pointer">
        <input type="radio" v-model="mode" value="execute" :id="`${data.id}-mode-execute`" class="accent-blue-500" />
        <span>Execute Tool</span>
      </label>
    </div>

    <!-- LIST MODE -->
    <div v-if="mode === 'list'" class="space-y-2">
      <BaseSelect
        :id="`${data.id}-server-list`"
        label="Server"
        v-model="selectedServer"
        :options="[{label: 'All Servers', value: 'all'}, ...servers.map(s => ({label: s, value: s}))]"
        :disabled="isLoadingServers"
      />
      <div v-if="isLoadingServers" class="text-xs text-zinc-400">Loading servers...</div>
      <div v-if="isLoadingToolsList" class="text-xs text-zinc-400">Loading tools list...</div>
      <div class="text-xs text-blue-300 text-center mt-2">
        Select a server to list its tools, or "All Servers" to list tools from all available servers.<br>
        The tools list will be sent to the output when this node is run.
      </div>
    </div>

    <!-- EXECUTE MODE -->
    <div v-if="mode === 'execute'" class="space-y-2">
      <BaseSelect
        :id="`${data.id}-server-exec`"
        label="Server"
        v-model="selectedServer"
        :options="servers.map(s => ({label: s, value: s}))"
        :disabled="isLoadingServers"
        placeholder="Select a server"
      />
      <div v-if="isLoadingServers" class="text-xs text-zinc-400">Loading servers...</div>
      <BaseSelect
        :id="`${data.id}-tool`"
        label="Tool"
        v-model="selectedTool"
        :options="toolsForServer.map(t => ({label: t.name, value: t.name}))"
        :disabled="isLoadingTools || !selectedServer"
        placeholder="Select a tool"
      />
      <div v-if="isLoadingTools" class="text-xs text-zinc-400">Loading tools...</div>
      <div v-if="selectedTool">
        <button @click="toggleToolSchema" class="w-full px-2 py-1 bg-slate-700 text-white rounded text-xs font-semibold mb-1 hover:bg-blue-700 transition">
          {{ showToolSchema ? 'Hide Schema' : 'Show Schema' }}
        </button>
        <div v-if="showToolSchema" class="bg-zinc-800 border border-zinc-600 rounded p-2 max-h-40 overflow-y-auto mt-1">
          <div class="font-bold text-green-400 text-xs mb-1">{{ currentToolSchema.name }} Schema</div>
          <div v-if="currentToolSchema.description" class="text-xs text-zinc-300 mb-1">{{ currentToolSchema.description }}</div>
          <pre class="text-xs text-zinc-200 whitespace-pre-wrap">{{ formattedToolSchema }}</pre>
        </div>
      </div>
      <BaseTextarea
        :id="`${data.id}-args`"
        label="Arguments (JSON)"
        v-model="argsInput"
        rows="5"
        placeholder="Enter JSON arguments for the selected tool"
      />
      <div class="text-xs text-blue-300 text-center mt-2">
        Connect an LLM node that outputs JSON with server, tool, and args properties to execute tools.
      </div>
    </div>

    <div v-if="errorMessage" class="text-xs text-red-400 bg-red-900/30 border-l-2 border-red-500 px-2 py-1 my-2">
      {{ errorMessage }}
    </div>

    <Handle v-if="data.hasInputs" type="target" position="left" style="width:12px;height:12px" />
    <Handle v-if="data.hasOutputs" type="source" position="right" style="width:12px;height:12px" />
  </BaseNode>
</template>

<script setup>
import { Handle } from '@vue-flow/core'
import BaseInput from '@/components/base/BaseInput.vue'
import BaseSelect from '@/components/base/BaseSelect.vue'
import BaseTextarea from '@/components/base/BaseTextarea.vue'
import BaseNode from '@/components/base/BaseNode.vue'
import { useMCPClient } from '@/composables/useMCPClient'

const props = defineProps({
  id: { type: String, default: 'MCP_Client_0' },
  data: {
    type: Object,
    default: () => ({
      style: {},
      type: 'MCPClientNode',
      inputs: {
        mode: 'list',
        command: '{"server":"","tool":"","args":{}}',
        selectedServer: '',
        selectedTool: '',
        argsInput: '{}'
      },
      outputs: {},
      hasInputs: true,
      hasOutputs: true,
      inputHandleColor: '#777',
      outputHandleColor: '#777'
    })
  }
})

const emit = defineEmits(['update:data','resize','disable-zoom','enable-zoom'])

const {
  mode,
  command,
  servers,
  selectedServer,
  toolsForServer,
  selectedTool,
  argsInput,
  isLoadingServers,
  isLoadingTools,
  isLoadingToolsList,
  errorMessage,
  showToolSchema,
  currentToolSchema,
  formattedToolSchema,
  customStyle,
  resizeHandleStyle,
  onResize,
  toggleToolSchema
} = useMCPClient(props, emit)
</script>

<!-- No scoped CSS: all styling is via Tailwind -->
