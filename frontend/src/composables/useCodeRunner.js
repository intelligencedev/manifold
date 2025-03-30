import { computed } from 'vue'
import { useVueFlow } from '@vue-flow/core'

/**
 * Composable for managing code execution in a sandbox environment.
 * @param {Object} props - Component props
 * @param {Function} emit - Component emit function
 * @returns {Object} - CodeRunner functionality
 */
export function useCodeRunner(props, emit) {
  const { getEdges, findNode } = useVueFlow()

  // Command computed property for the multiline user input or JSON
  const command = computed({
    get: () => props.data.inputs?.command || '',
    set: (value) => {
      props.data.inputs.command = value
    }
  })

  // Label computed property for the node label
  const label = computed({
    get: () => props.data.type,
    set: (value) => {
      props.data.type = value
      updateNodeData()
    }
  })

  /**
   * Update node data and emit changes
   */
  function updateNodeData() {
    const updatedData = {
      ...props.data,
      inputs: {
        command: command.value
      },
      outputs: props.data.outputs
    }
    emit('update:data', { id: props.id, data: updatedData })
  }

  /**
   * Attempt to parse the user input as JSON. If it parses successfully,
   * check for a valid language. Otherwise, default to Python with empty dependencies.
   * @param {string} userInput - The raw user input (could be JSON or code)
   * @returns {Object} - A valid payload object or { error: 'language not supported' }
   */
  function parseOrCreatePayload(userInput) {
    let payload
    try {
      // Try to parse as JSON
      payload = JSON.parse(userInput)

      if (!payload.language || !['python', 'go', 'javascript'].includes(payload.language)) {
        return { error: 'language not supported' }
      }
      // Ensure 'code' exists
      if (!payload.code) {
        payload.code = ''
      }
      // Ensure 'dependencies' is an array
      if (!Array.isArray(payload.dependencies)) {
        payload.dependencies = []
      }
    } catch (_err) {
      // Not JSON => default to Python code with no dependencies
      payload = {
        language: 'python',
        code: userInput,
        dependencies: []
      }
    }
    return payload
  }

  /**
   * Main run function that executes code via /api/code/eval
   * @returns {Promise<Object>} - Result of execution
   */
  async function run() {
    try {
      // Clear previous output
      props.data.outputs.result = { output: '' }

      // Identify connected source nodes
      const connectedSources = getEdges.value
        .filter((edge) => edge.target === props.id)
        .map((edge) => edge.source)

      let payload

      if (connectedSources.length > 0) {
        // Source node might produce code or JSON
        const sourceData = findNode(connectedSources[0]).data.outputs.result.output
        console.log('Connected source data:', sourceData)

        payload = parseOrCreatePayload(sourceData)
        props.data.inputs.command = JSON.stringify(payload)
      } else {
        // Use local user input
        let userInput = props.data.inputs.command
        payload = parseOrCreatePayload(userInput)
      }

      // If language not supported, return immediately
      if (payload.error === 'language not supported') {
        props.data.outputs.result = { output: `Error: ${payload.error}` }
        updateNodeData()
        return payload
      }

      // POST to /api/code/eval
      const response = await fetch('http://localhost:8080/api/code/eval', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      })

      if (!response.ok) {
        const errorMsg = await response.text()
        console.error('Error response from server:', errorMsg)
        props.data.outputs.result = { output: `Error: ${errorMsg}` }
        updateNodeData()
        return { error: errorMsg }
      }

      const result = await response.json()
      console.log('Node-level run result:', result)

      // Extract output or error and ensure it's always properly set
      const resultStr = result.stdout || result.stderr || ''
      props.data.outputs = {
        result: {
          output: resultStr
        }
      }

      updateNodeData()
      return { response, result }
    } catch (error) {
      console.error('Error in run():', error)
      props.data.outputs.result = { output: `Error: ${error.message}` }
      updateNodeData()
      return { error }
    }
  }

  return {
    command,
    label,
    updateNodeData,
    run
  }
}
