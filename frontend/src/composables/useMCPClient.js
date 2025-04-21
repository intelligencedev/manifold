import { ref, computed, onMounted, watch } from 'vue'

/**
 * Composable for managing MCPClient state and functionality.
 * It handles server and tool selection, fetching available tools,
 * and executing MCP tools either through UI or from connected nodes.
 */
export function useMCPClient(props, emit, vueFlow) {
  const { getEdges, findNode } = vueFlow

  // Local state for UI interactions
  const isHovered = ref(false)
  const customStyle = ref({})

  // State variables for MCP servers and tools
  const servers = ref([])
  const selectedServer = ref('')
  const toolsForServer = ref([])
  const selectedTool = ref('')
  const argsInput = ref('')
  const isLoadingServers = ref(false)
  const isLoadingTools = ref(false)
  const errorMessage = ref('')

  // Setup two-way binding for the primary inputs
  const command = computed({
    get: () => props.data.inputs.command || '',
    set: (value) => {
      props.data.inputs.command = value
    }
  })

  // (Optional) Computed style for resize handles
  const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
    width: '12px',
    height: '12px'
  }))

  // Fetch servers when component mounts
  onMounted(async () => {
    if (!props.data.run) {
      props.data.run = run
    }

    // Try to parse any existing command as input
    if (props.data.inputs.command) {
      try {
        const cmd = JSON.parse(props.data.inputs.command)
        if (cmd.server) {
          selectedServer.value = cmd.server
        }
        if (cmd.tool) {
          selectedTool.value = cmd.tool
        }
        if (cmd.args) {
          argsInput.value = JSON.stringify(cmd.args, null, 2)
        }
      } catch (err) {
        console.warn("Invalid JSON in initial command, ignoring:", err)
        argsInput.value = '{}'
      }
    } else {
      argsInput.value = '{}'
    }

    await fetchServers()
  })

  // Watch selectedServer and fetch tools when it changes
  watch(selectedServer, async (newServer) => {
    if (newServer) {
      await fetchToolsForServer(newServer)
    } else {
      toolsForServer.value = []
      selectedTool.value = ''
    }
  })

  // Update the command when server, tool, or args change
  watch([selectedServer, selectedTool, argsInput], () => {
    updateCommandFromInputs()
  })

  /**
   * fetchServers - Retrieves available MCP servers from the backend
   */
  async function fetchServers() {
    isLoadingServers.value = true
    errorMessage.value = ''
    try {
      const response = await fetch('http://localhost:8080/api/mcp/servers')
      if (!response.ok) {
        throw new Error(`Failed to fetch servers: ${response.statusText}`)
      }
      const data = await response.json()
      servers.value = data.servers || []
      
      // Select first server if none selected yet
      if (servers.value.length > 0 && !selectedServer.value) {
        selectedServer.value = servers.value[0]
      }
    } catch (error) {
      console.error('Error fetching servers:', error)
      errorMessage.value = error.message
    } finally {
      isLoadingServers.value = false
    }
  }

  /**
   * fetchToolsForServer - Retrieves available tools for a specific MCP server
   */
  async function fetchToolsForServer(serverName) {
    isLoadingTools.value = true
    errorMessage.value = ''
    try {
      const response = await fetch(`http://localhost:8080/api/mcp/servers/${serverName}/tools`)
      if (!response.ok) {
        throw new Error(`Failed to fetch tools: ${response.statusText}`)
      }
      const data = await response.json()
      toolsForServer.value = data.tools || []
      
      // If the currently selected tool is not available in this server, clear it
      if (selectedTool.value && !toolsForServer.value.some(t => t.name === selectedTool.value)) {
        selectedTool.value = ''
      }
    } catch (error) {
      console.error(`Error fetching tools for ${serverName}:`, error)
      errorMessage.value = error.message
    } finally {
      isLoadingTools.value = false
    }
  }

  /**
   * updateCommandFromInputs - Updates the command property based on the current inputs
   */
  function updateCommandFromInputs() {
    if (selectedServer.value && selectedTool.value) {
      let args = {}
      try {
        args = argsInput.value ? JSON.parse(argsInput.value) : {}
      } catch (err) {
        console.warn("Invalid JSON in arguments, using empty object:", err)
      }

      const commandObj = {
        server: selectedServer.value,
        tool: selectedTool.value,
        args
      }

      props.data.inputs.command = JSON.stringify(commandObj, null, 2)
    }
  }

  /**
   * processConnectedSource - Processes data from a connected source node (like an LLM)
   */
  function processConnectedSource(sourceData) {
    try {
      // Parse the JSON from the connected node
      const incomingData = typeof sourceData === 'string' 
        ? JSON.parse(sourceData) 
        : sourceData
        
      // Extract server, tool, and args from the incoming data
      const serverName = incomingData.server
      const toolName = incomingData.tool
      const toolArgs = incomingData.args || {}
      
      if (!serverName || !toolName) {
        throw new Error("Connected source data missing required 'server' or 'tool' properties")
      }
      
      return { serverName, toolName, toolArgs }
    } catch (err) {
      throw new Error(`Invalid data from connected source: ${err.message}`)
    }
  }

  /**
   * run() - Processes the command input:
   *   - Clears previous output
   *   - Checks for connected source nodes
   *   - Processes data from UI or connected node
   *   - Sends the request to the proper MCP server endpoint
   *   - Updates the node's outputs based on the response
   */
  async function run() {
    console.log('Running MCPClient:', props.id)
    try {
      // Clear previous output
      props.data.outputs.result = ''
      errorMessage.value = ''
      
      // Check for connected source nodes
      const connectedSources = getEdges.value
        .filter(edge => edge.target === props.id)
        .map(edge => edge.source)
      
      let serverName, toolName, toolArgs
      
      if (connectedSources.length > 0) {
        // Get data from connected source node (e.g., LLM output)
        const sourceNode = findNode(connectedSources[0])
        if (sourceNode && sourceNode.data.outputs?.result?.output) {
          const sourceData = sourceNode.data.outputs.result.output
          console.log('Connected source data:', sourceData)
          
          try {
            // Process the data from connected source
            const processedData = processConnectedSource(sourceData)
            serverName = processedData.serverName
            toolName = processedData.toolName
            toolArgs = processedData.toolArgs
            
            // Update UI to reflect the incoming data
            if (servers.value.includes(serverName)) {
              selectedServer.value = serverName
              
              // Fetch tools for this server if not already loaded
              if (!toolsForServer.value.length) {
                await fetchToolsForServer(serverName)
              }
              
              // Update selected tool
              selectedTool.value = toolName
              
              // Update args input
              argsInput.value = JSON.stringify(toolArgs, null, 2)
            } else {
              throw new Error(`Server "${serverName}" not found in available servers`)
            }
          } catch (err) {
            console.error("Error processing connected source data:", err)
            errorMessage.value = err.message
            props.data.outputs.result = { output: `Error: ${err.message}` }
            return { error: err.message }
          }
        }
      } else {
        // Use manually selected values from UI
        serverName = selectedServer.value
        toolName = selectedTool.value
        
        try {
          // Parse args from the text input
          toolArgs = argsInput.value ? JSON.parse(argsInput.value) : {}
        } catch (err) {
          console.error("Invalid JSON in arguments input:", err)
          errorMessage.value = `Invalid JSON in arguments: ${err.message}`
          props.data.outputs.result = { output: `Error: ${errorMessage.value}` }
          return { error: errorMessage.value }
        }
      }
      
      // Validate we have all required information
      if (!serverName) {
        errorMessage.value = "No server selected"
        props.data.outputs.result = { output: `Error: ${errorMessage.value}` }
        return { error: errorMessage.value }
      }
      
      if (!toolName) {
        errorMessage.value = "No tool selected"
        props.data.outputs.result = { output: `Error: ${errorMessage.value}` }
        return { error: errorMessage.value }
      }
      
      // Execute the tool with the specified arguments
      console.log(`Executing ${serverName}/${toolName} with args:`, toolArgs)
      const response = await fetch(`http://localhost:8080/api/mcp/servers/${serverName}/tools/${toolName}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ args: toolArgs })
      })
      
      if (!response.ok) {
        const errorMsg = await response.text()
        console.error('Error response from server:', errorMsg)
        errorMessage.value = errorMsg
        props.data.outputs.result = { output: `Error: ${errorMsg}` }
        return { error: errorMsg }
      }
      
      const result = await response.json()
      console.log('MCP Client run result:', result)
      
      // Extract result content
      let resultOutput
      if (result.content && result.content.text) {
        resultOutput = result.content.text
      } else {
        resultOutput = JSON.stringify(result, null, 2)
      }
      
      props.data.outputs = { result: { output: resultOutput } }
      updateNodeData()
      
      return { response, result }
    } catch (error) {
      console.error('Error in run():', error)
      errorMessage.value = error.message
      props.data.outputs.result = { output: `Error: ${error.message}` }
      return { error }
    }
  }

  /**
   * updateNodeData() - Emits updated node data to VueFlow
   */
  function updateNodeData() {
    const updatedData = {
      ...props.data,
      inputs: { 
        command: command.value,
        selectedServer: selectedServer.value,
        selectedTool: selectedTool.value,
        argsInput: argsInput.value
      },
      outputs: props.data.outputs
    }
    emit('update:data', { id: props.id, data: updatedData })
  }

  /**
   * onResize() - Handler for resize events (if needed)
   */
  function onResize(event) {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
    emit('resize', event)
  }

  return {
    isHovered,
    customStyle,
    command,
    servers,
    selectedServer,
    toolsForServer,
    selectedTool,
    argsInput,
    isLoadingServers,
    isLoadingTools,
    errorMessage,
    resizeHandleStyle,
    run,
    onResize,
    updateNodeData,
    fetchServers,
    fetchToolsForServer
  }
}
