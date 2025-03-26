<template>
  <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
    class="node-container mcp-client-node tool-node"
    @mouseenter="isHovered = true" 
    @mouseleave="isHovered = false">
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <!-- Configuration Form -->
    <BaseAccordion title="Configuration" :initiallyOpen="true">
      <BaseInput
        label="Server URL"
        v-model="serverUrl"
        placeholder="ws://localhost:8000/ws"
      />
      <BaseInput
        label="Customer ID"
        v-model="customerId"
        placeholder="customer123"
      />
      <BaseInput
        label="Customer Name"
        v-model="customerName"
        placeholder="John Doe"
      />
    </BaseAccordion>

    <!-- Connection Status & Controls -->
    <div class="connection-controls">
      <div class="connection-status" :class="{ connected: connected }">
        Status: {{ wsStatus }}
      </div>
      <div class="connection-buttons">
        <button @click="connectWebSocket" :disabled="connected" class="connect-btn">
          Connect
        </button>
        <button @click="disconnectWebSocket" :disabled="!connected" class="disconnect-btn">
          Disconnect
        </button>
        <button @click="clearMessages" class="clear-btn">
          Clear Messages
        </button>
      </div>
    </div>

    <!-- Initial Message -->
    <BaseTextarea
      label="Initial Message"
      v-model="initialMessage"
      placeholder="Hello, how can I help you?"
    />

    <!-- Messages Display -->
    <div class="messages-container">
      <div v-for="(msg, idx) in socketMessages" :key="idx" class="message" :class="msg.data?.role || 'system'">
        <div class="message-role">{{ msg.data?.role || 'system' }}:</div>
        <div class="message-content">{{ msg.data?.content || JSON.stringify(msg) }}</div>
      </div>
      <div v-if="socketMessages.length === 0" class="no-messages">
        No messages yet. Connect to the server and send a message.
      </div>
    </div>

    <!-- Send Message Form -->
    <div class="send-message-form">
      <BaseTextarea
        ref="messageInput"
        v-model="newMessage"
        placeholder="Type a message..."
        class="message-input"
        @keydown.enter.prevent="sendMessage"
      />
      <button @click="sendMessage" :disabled="!connected || !newMessage" class="send-btn">
        Send
      </button>
    </div>

    <!-- Input/Output Handles -->
    <Handle
      style="width:12px; height:12px"
      v-if="data.hasInputs"
      type="target"
      position="left"
    />
    <Handle
      style="width:12px; height:12px"
      v-if="data.hasOutputs"
      type="source"
      position="right"
    />

    <!-- NodeResizer -->
    <NodeResizer 
      :is-resizable="true" 
      :color="'#666'" 
      :handle-style="resizeHandleStyle"
      :line-style="resizeHandleStyle" 
      :min-width="400" 
      :min-height="500"
      :node-id="id" 
      @resize="onResize" 
    />
  </div>
</template>

<script setup>
import { ref, onUnmounted } from "vue";
import { Handle, useVueFlow } from "@vue-flow/core";
import { NodeResizer } from "@vue-flow/node-resizer";
import BaseAccordion from '@/components/base/BaseAccordion.vue';
import BaseInput from '@/components/base/BaseInput.vue';
import BaseTextarea from '@/components/base/BaseTextarea.vue';
import { useMCPClient } from "@/composables/useMCPClient";

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: "MCPClient_0",
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: "MCPClient",
      labelStyle: { fontWeight: "normal" },
      hasInputs: true,
      hasOutputs: true,
      inputs: {
        serverUrl: "ws://localhost:8000/ws",
        customerId: "customer_123",
        customerName: "Customer",
        initialMessage: "",
      },
      outputs: {},
      style: {
        border: "1px solid #666",
        borderRadius: "12px",
        backgroundColor: "#333",
        color: "#eee",
        width: "500px",
        height: "700px",
      },
    }),
  }
});

const emit = defineEmits(["update:data", "resize"]);
const vueFlowInstance = useVueFlow();

// New message input
const messageInput = ref(null);
const newMessage = ref("");

// Send a new message from the input field
function sendMessage() {
  if (newMessage.value.trim() && connected.value) {
    sendUserMessage(newMessage.value);
    newMessage.value = "";
    // Focus back on input after sending
    if (messageInput.value) {
      messageInput.value.focus();
    }
  }
}

// Use the composable to manage state and functionality
const {
  // State refs
  isHovered,
  customStyle,
  connected,
  socketMessages,
  
  // Computed properties
  serverUrl,
  customerId,
  customerName,
  initialMessage,
  wsStatus,
  resizeHandleStyle,
  
  // Methods
  connectWebSocket,
  disconnectWebSocket,
  sendUserMessage,
  onResize,
  clearMessages,
  cleanup
} = useMCPClient(props, emit, vueFlowInstance);

// Clean up on component unmount
onUnmounted(() => {
  cleanup();
});
</script>

<style scoped>
.mcp-client-node {
  --accent-color: #1e88e5;
  --connected-color: #4caf50;
  --disconnected-color: #f44336;
  display: flex;
  flex-direction: column;
  padding: 10px;
}

.connection-controls {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}

.connection-status {
  padding: 5px 10px;
  border-radius: 4px;
  background-color: var(--disconnected-color);
  color: white;
}

.connection-status.connected {
  background-color: var(--connected-color);
}

.connection-buttons {
  display: flex;
  gap: 5px;
}

button {
  padding: 6px 12px;
  border-radius: 4px;
  border: none;
  cursor: pointer;
  font-size: 14px;
}

button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.connect-btn {
  background-color: var(--connected-color);
  color: white;
}

.disconnect-btn {
  background-color: var(--disconnected-color);
  color: white;
}

.clear-btn {
  background-color: #607d8b;
  color: white;
}

.send-btn {
  background-color: var(--accent-color);
  color: white;
}

.messages-container {
  flex: 1;
  overflow-y: auto;
  border: 1px solid #555;
  border-radius: 4px;
  padding: 10px;
  margin-bottom: 10px;
  background-color: #222;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.no-messages {
  color: #888;
  text-align: center;
  padding: 20px;
}

.message {
  padding: 8px 12px;
  border-radius: 4px;
  max-width: 90%;
}

.message.user {
  align-self: flex-end;
  background-color: var(--accent-color);
  color: white;
}

.message.assistant, .message.agent {
  align-self: flex-start;
  background-color: #424242;
  color: white;
}

.message.system {
  align-self: center;
  background-color: #333;
  color: #aaa;
  font-style: italic;
  font-size: 0.9em;
}

.message-role {
  font-weight: bold;
  margin-bottom: 4px;
  font-size: 0.8em;
  text-transform: uppercase;
}

.message-content {
  white-space: pre-wrap;
}

.send-message-form {
  display: flex;
  gap: 10px;
  margin-top: 10px;
}

.message-input {
  flex: 1;
  height: 40px;
}
</style>
