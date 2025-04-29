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

  // Mode selection: 'list' or 'execute'
  const mode = ref('list')

  // State variables for MCP servers and tools
  const servers = ref([])
  const selectedServer = ref('')
  const toolsForServer = ref([])
  const selectedTool = ref('')
  const argsInput = ref('')
  const isLoadingServers = ref(false)
  const isLoadingTools = ref(false)
  const errorMessage = ref('')
  const showToolSchema = ref(false)
  const currentToolSchema = ref({})
  const toolsList = ref(null)
  const isLoadingToolsList = ref(false)

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

  // Format tool schema as a string for display
  const formattedToolSchema = computed(() => {
    if (!currentToolSchema.value || !Object.keys(currentToolSchema.value).length) {
      return '';
    }
    
    return JSON.stringify(currentToolSchema.value, null, 2);
  })

  // Fetch servers when component mounts
  onMounted(async () => {
    if (!props.data.run) {
      props.data.run = run
    }

    // Set initial mode
    if (props.data.inputs.mode) {
      mode.value = props.data.inputs.mode
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

  // Watch mode changes
  watch(mode, (newMode) => {
    props.data.inputs.mode = newMode
    updateNodeData()
  })

  // Watch selectedServer and fetch tools when it changes
  watch(selectedServer, async (newServer) => {
    if (newServer) {
      if (mode.value === 'execute') {
        await fetchToolsForServer(newServer)
      } else if (mode.value === 'list' && newServer !== 'all') {
        // In list mode, fetch tools for the selected server
        await fetchToolsListForServer(newServer)
      } else if (mode.value === 'list' && newServer === 'all') {
        // Fetch tools for all servers
        await fetchAllToolsList()
      }
    } else {
      toolsForServer.value = []
      selectedTool.value = ''
      toolsList.value = null
    }
  })

  // Watch selectedTool and fetch schema when it changes
  watch(selectedTool, async (newTool) => {
    if (newTool && selectedServer.value && mode.value === 'execute') {
      await fetchToolSchema(selectedServer.value, newTool)
    } else {
      currentToolSchema.value = {}
    }
  })

  // Update the command when server, tool, or args change
  watch([selectedServer, selectedTool, argsInput], () => {
    if (mode.value === 'execute') {
      updateCommandFromInputs()
    }
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
        if (mode.value === 'list') {
          selectedServer.value = 'all'  // Default to 'all' for list mode
        } else {
          selectedServer.value = servers.value[0]  // First server for execute mode
        }
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
   * Used in execute mode
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
   * fetchToolsListForServer - Retrieves tools list for a specific server
   * Used in list mode
   */
  async function fetchToolsListForServer(serverName) {
    isLoadingToolsList.value = true
    errorMessage.value = ''
    try {
      const response = await fetch(`http://localhost:8080/api/mcp/servers/${serverName}/tools`)
      if (!response.ok) {
        throw new Error(`Failed to fetch tools: ${response.statusText}`)
      }
      const data = await response.json()
      
      // Format tools list for the specific server but preserve full schema
      const formattedTools = data.tools.map(tool => ({
        name: tool.name,
        description: tool.description || '',
        schema: {
          inputSchema: tool.inputSchema || {},
          outputSchema: tool.outputSchema || {}
        },
        server: serverName
      }))
      
      // Set the tools list and update outputs
      toolsList.value = { [serverName]: formattedTools }
      
      // Update node outputs
      props.data.outputs.result = { 
        output: JSON.stringify(toolsList.value, null, 2)
      }
      updateNodeData()
    } catch (error) {
      console.error(`Error fetching tools list for ${serverName}:`, error)
      errorMessage.value = error.message
      toolsList.value = null
    } finally {
      isLoadingToolsList.value = false
    }
  }

  /**
   * fetchAllToolsList - Fetches tools from all available servers
   * Used in list mode with 'all' selection
   */
  async function fetchAllToolsList() {
    isLoadingToolsList.value = true
    errorMessage.value = ''
    try {
      const allTools = {}
      
      // Iterate through all servers
      for (const serverName of servers.value) {
        const response = await fetch(`http://localhost:8080/api/mcp/servers/${serverName}/tools`)
        if (!response.ok) {
          console.error(`Failed to fetch tools for ${serverName}`)
          continue
        }
        
        const data = await response.json()
        const tools = data.tools || []
        
        // Add tools to the schema with full schema information
        allTools[serverName] = tools.map(tool => ({
          name: tool.name,
          description: tool.description || '',
          schema: {
            inputSchema: tool.inputSchema || {},
            outputSchema: tool.outputSchema || {}
          },
          server: serverName
        }))
      }
      
      // Set the tools list and update outputs
      toolsList.value = allTools
      
      props.data.outputs.result = { 
        output: JSON.stringify(allTools, null, 2)
      }
      updateNodeData()
    } catch (error) {
      console.error('Error fetching all tools:', error)
      errorMessage.value = error.message
      toolsList.value = null
    } finally {
      isLoadingToolsList.value = false
    }
  }

  /**
   * fetchToolSchema - Fetches the schema for a specific tool
   */
  async function fetchToolSchema(serverName, toolName) {
    try {
      // Find the tool in our toolsForServer list
      const toolInfo = toolsForServer.value.find(t => t.name === toolName)
      if (!toolInfo) {
        currentToolSchema.value = {}
        return
      }
      
      // Extract the schema information
      currentToolSchema.value = {
        name: toolInfo.name,
        description: toolInfo.description || '',
        inputSchema: toolInfo.inputSchema || {},
        outputSchema: toolInfo.outputSchema || {}
      }
    } catch (error) {
      console.error(`Error processing schema for ${toolName}:`, error)
      currentToolSchema.value = {}
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
   * run() - Processes the command input based on the current mode:
   *   - In 'list' mode: Returns the tools list for the selected server
   *   - In 'execute' mode: Executes the specified tool with provided arguments
   */
  async function run() {
    console.log('Running MCPClient:', props.id, 'Mode:', mode.value)
    try {
      // Clear previous output
      props.data.outputs.result = ''
      errorMessage.value = ''
      
      // Handle List Mode
      if (mode.value === 'list') {
        if (!selectedServer.value) {
          errorMessage.value = "No server selected"
          props.data.outputs.result = { output: `Error: ${errorMessage.value}` }
          return { error: errorMessage.value }
        }
        
        if (selectedServer.value === 'all') {
          await fetchAllToolsList()
        } else {
          await fetchToolsListForServer(selectedServer.value)
        }
        
        return { success: true, tools: toolsList.value }
      }
      
      // Handle Execute Mode
      else if (mode.value === 'execute') {
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
      }
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
        mode: mode.value,
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

  /**
   * toggleToolSchema() - Toggle display of the tool schema
   */
  function toggleToolSchema() {
    showToolSchema.value = !showToolSchema.value
  }

  return {
    isHovered,
    customStyle,
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
    toolsList,
    resizeHandleStyle,
    run,
    onResize,
    updateNodeData,
    fetchServers,
    fetchToolsForServer,
    fetchToolsListForServer,
    fetchAllToolsList,
    toggleToolSchema
  }
}
