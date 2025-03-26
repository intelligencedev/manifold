import { ref, computed, onMounted, watch } from 'vue'

/**
 * Composable for managing MCPClient state and functionality
 */
export function useMCPClient(props, emit, vueFlow) {
  const { getEdges, findNode } = vueFlow
  
  // State variables
  const isHovered = ref(false)
  const customStyle = ref({})
  const socket = ref(null)
  const connected = ref(false)
  const socketMessages = ref([])
  
  // Computed properties for form binding
  const serverUrl = computed({
    get: () => props.data.inputs.serverUrl,
    set: (value) => { props.data.inputs.serverUrl = value }
  })
  
  const customerId = computed({
    get: () => props.data.inputs.customerId,
    set: (value) => { props.data.inputs.customerId = value }
  })
  
  const customerName = computed({
    get: () => props.data.inputs.customerName,
    set: (value) => { props.data.inputs.customerName = value }
  })
  
  const initialMessage = computed({
    get: () => props.data.inputs.initialMessage,
    set: (value) => { props.data.inputs.initialMessage = value }
  })
  
  const wsStatus = computed(() => {
    if (connected.value) return 'Connected'
    if (socket.value) return 'Connecting...'
    return 'Disconnected'
  })
  
  // UI state
  const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
    width: '12px',
    height: '12px'
  }))
  
  // Connect to websocket
  function connectWebSocket() {
    // Close existing socket if it exists
    disconnectWebSocket()
    
    try {
      console.log(`[MCPClient] Connecting to ${serverUrl.value}`)
      socket.value = new WebSocket(serverUrl.value)
      
      socket.value.onopen = (event) => {
        console.log('[MCPClient] WebSocket connection established')
        connected.value = true
        
        // Register client with the server
        const registrationMessage = {
          type: 'client_registration',
          data: {
            id: customerId.value,
            name: customerName.value,
          }
        }
        
        sendMessage(registrationMessage)
        
        // Send initial message if provided
        if (initialMessage.value && initialMessage.value.trim() !== '') {
          setTimeout(() => {
            const initialUserMessage = {
              type: 'user_message',
              data: {
                content: initialMessage.value,
                role: 'user'
              }
            }
            sendMessage(initialUserMessage)
          }, 1000)
        }
      }
      
      socket.value.onmessage = (event) => {
        console.log('[MCPClient] Message received:', event.data)
        try {
          const message = JSON.parse(event.data)
          socketMessages.value.push(message)
          
          // Also update the output for downstream nodes
          if (message.data && message.data.content) {
            props.data.outputs.result = {
              output: message.data.content
            }
          }
        } catch (error) {
          console.error('[MCPClient] Error parsing message:', error)
        }
      }
      
      socket.value.onclose = (event) => {
        console.log('[MCPClient] WebSocket connection closed')
        connected.value = false
      }
      
      socket.value.onerror = (error) => {
        console.error('[MCPClient] WebSocket error:', error)
        connected.value = false
      }
    } catch (error) {
      console.error('[MCPClient] Error connecting to WebSocket:', error)
      connected.value = false
    }
  }
  
  // Disconnect from websocket
  function disconnectWebSocket() {
    if (socket.value) {
      socket.value.close()
      socket.value = null
      connected.value = false
    }
  }
  
  // Send message to websocket
  function sendMessage(message) {
    if (socket.value && connected.value) {
      if (typeof message === 'object') {
        socket.value.send(JSON.stringify(message))
      } else {
        socket.value.send(message)
      }
    } else {
      console.error('[MCPClient] Cannot send message: WebSocket not connected')
    }
  }
  
  // Send a user message to the server
  function sendUserMessage(content) {
    const message = {
      type: 'user_message',
      data: {
        content,
        role: 'user'
      }
    }
    sendMessage(message)
    
    // Add to local messages for display
    socketMessages.value.push(message)
  }
  
  // Main run function
  async function run() {
    console.log('Running MCPClient:', props.id)
    
    try {
      // Check for connected source nodes
      const connectedSources = getEdges.value
        .filter((edge) => edge.target === props.id)
        .map((edge) => edge.source)
      
      if (connectedSources.length > 0) {
        const sourceNode = findNode(connectedSources[0])
        if (sourceNode && sourceNode.data.outputs?.result?.output) {
          const inputText = sourceNode.data.outputs.result.output
          console.log('[MCPClient] Got input from source node:', inputText)
          
          // If connected, send the message
          if (connected.value && socket.value) {
            sendUserMessage(inputText)
          } else {
            // Store it as initialMessage and connect
            initialMessage.value = inputText
            connectWebSocket()
          }
        }
      }
      
      return { status: wsStatus.value }
    } catch (error) {
      console.error('[MCPClient] Error in run:', error)
      return { error: error.message }
    }
  }
  
  // Event handlers
  function onResize(event) {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
    emit('resize', event)
  }
  
  // Clear messages
  function clearMessages() {
    socketMessages.value = []
  }
  
  // Lifecycle hooks
  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run
    }
  })
  
  // Cleanup on unmount
  function cleanup() {
    disconnectWebSocket()
  }
  
  // Watch for server URL changes to reconnect
  watch(serverUrl, (newUrl, oldUrl) => {
    if (newUrl !== oldUrl && connected.value) {
      console.log('[MCPClient] Server URL changed, reconnecting...')
      connectWebSocket()
    }
  })
  
  return {
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
    run,
    onResize,
    clearMessages,
    cleanup
  }
}