import { ref, computed } from 'vue'
import { useVueFlow } from '@vue-flow/core'

/**
 * Composable for managing RepoConcat functionality
 * @param {Object} props - Component props
 * @param {Function} emit - Component emit function
 * @returns {Object} - RepoConcat functionality
 */
export function useRepoConcat(props, emit) {
  const { getEdges, findNode } = useVueFlow()

  // Local reactive variable for the update-from-source checkbox
  const updateFromSource = ref(props.data.updateFromSource)

  // Computed property for the node label
  const label = computed({
    get: () => props.data.type,
    set: (value) => {
      props.data.type = value
      updateNodeData()
    },
  })

  // Computed property for the "paths" input
  const paths = computed({
    get: () => props.data.inputs?.paths || '',
    set: (value) => {
      props.data.inputs.paths = value
      updateNodeData()
    },
  })

  // Computed property for the "types" input
  const types = computed({
    get: () => props.data.inputs?.types || '',
    set: (value) => {
      props.data.inputs.types = value
      updateNodeData()
    },
  })

  // Computed property for the "recursive" checkbox
  const recursive = computed({
    get: () => props.data.inputs?.recursive || false,
    set: (value) => {
      props.data.inputs.recursive = value
      updateNodeData()
    },
  })

  // Computed property for the "ignorePattern" input
  const ignorePattern = computed({
    get: () => props.data.inputs?.ignorePattern || '',
    set: (value) => {
      props.data.inputs.ignorePattern = value
      updateNodeData()
    },
  })

  /**
   * Emit updated node data to VueFlow
   */
  function updateNodeData() {
    const updatedData = {
      ...props.data,
      inputs: {
        paths: paths.value,
        types: types.value,
        recursive: recursive.value,
        ignorePattern: ignorePattern.value,
      },
      outputs: props.data.outputs,
      updateFromSource: updateFromSource.value,
    }
    emit('update:data', { id: props.id, data: updatedData })
  }

  /**
   * Main run function that concatenates repository files
   * @returns {Promise<Object>} - Result of concatenation
   */
  async function run() {
    try {
      // Clear previous output
      props.data.outputs.result = ''

      // Check for connected source nodes (to optionally update input parameters)
      const connectedSources = getEdges.value
        .filter(edge => edge.target === props.id)
        .map(edge => edge.source)

      let payload
      if (connectedSources.length > 0 && updateFromSource.value) {
        const sourceData = findNode(connectedSources[0]).data.outputs.result.output
        console.log('Connected source data:', sourceData)
        try {
          payload = JSON.parse(sourceData)
        } catch (err) {
          console.error('Error parsing JSON from connected node:', err)
          props.data.outputs.result = { error: 'Invalid JSON from connected node' }
          return { error: 'Invalid JSON from connected node' }
        }
      } else {
        // Use the values entered in this node's input fields.
        const pathsInput = props.data.inputs.paths
        const typesInput = props.data.inputs.types
        const recursiveValue = props.data.inputs.recursive
        const ignorePatternValue = props.data.inputs.ignorePattern

        // Convert comma-separated strings into arrays.
        const pathsArray = pathsInput.split(',').map(s => s.trim()).filter(s => s)
        const typesArray = typesInput.split(',').map(s => s.trim()).filter(s => s)

        payload = {
          paths: pathsArray,
          types: typesArray,
          recursive: recursiveValue,
          ignorePattern: ignorePatternValue,
        }
      }

      // POST the parameters to the /api/repoconcat endpoint.
      const response = await fetch('http://localhost:8080/api/repoconcat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })

      if (!response.ok) {
        const errorMsg = await response.text()
        console.error('Error response from server:', errorMsg)
        props.data.outputs.result = { error: errorMsg }
        return { error: errorMsg }
      }

      // The API returns plain text – the concatenated output.
      const result = await response.text()
      console.log('RepoConcat run result:', result)

      props.data.outputs = {
        result: {
          output: result,
        },
      }

      updateNodeData()
      return { response, result }
    } catch (error) {
      console.error('Error in run():', error)
      props.data.outputs.result = { error: error.message }
      return { error }
    }
  }

  return {
    updateFromSource,
    label,
    paths,
    types,
    recursive,
    ignorePattern,
    updateNodeData,
    run
  }
}