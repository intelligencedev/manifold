import { ref, computed, onMounted, watch } from 'vue'
import { useVueFlow } from '@vue-flow/core'
import { useNodeBase } from './useNodeBase'

/**
 * Composable for managing FlowControl node state and functionality
 */
export function useFlowControl(props, emit) {
  const { getEdges, findNode } = useVueFlow()
  const { isHovered, resizeHandleStyle, computedContainerStyle } = useNodeBase(props, emit)

  // Initialize inputs with sensible defaults if missing or incomplete
  if (!props.data.inputs || Object.keys(props.data.inputs).length === 0) {
    props.data.inputs = {
      mode: 'RunAllChildren',
      targetNodeId: '',
      delimiter: '',
      waitTime: 5,
      combineMode: 'newline'
    }
  } else {
    if (!props.data.inputs.mode) props.data.inputs.mode = 'RunAllChildren'
    if (props.data.inputs.targetNodeId === undefined) props.data.inputs.targetNodeId = ''
    if (props.data.inputs.delimiter === undefined) props.data.inputs.delimiter = ''
    if (props.data.inputs.waitTime === undefined) props.data.inputs.waitTime = 5
    if (props.data.inputs.combineMode === undefined) props.data.inputs.combineMode = 'newline'
  }

  // Initialize outputs if they don't exist or ensure output structure is consistent
  if (!props.data.outputs) {
    props.data.outputs = { result: { output: '' } }
  } else if (!props.data.outputs.result) {
    props.data.outputs.result = { output: '' }
  } else if (!props.data.outputs.result.output) {
    props.data.outputs.result.output = ''
  }

  // Computed properties for inputs
  const mode = computed({
    get: () => props.data.inputs.mode || 'RunAllChildren',
    set: (value) => { props.data.inputs.mode = value },
  })

  const targetNodeId = computed({
    get: () => props.data.inputs.targetNodeId || '',
    set: (value) => { props.data.inputs.targetNodeId = value },
  })

  const delimiter = computed({
    get: () => props.data.inputs.delimiter || '',
    set: (value) => { props.data.inputs.delimiter = value },
  })

  const waitTime = computed({
    get: () => props.data.inputs.waitTime || 5,
    set: (value) => { props.data.inputs.waitTime = parseInt(value) || 5 },
  })

  const combineMode = computed({
    get: () => props.data.inputs.combineMode || 'newline',
    set: (value) => { props.data.inputs.combineMode = value },
  })

  // Mode options
  const modeOptions = [
    { value: 'RunAllChildren', label: 'Run All Children' },
    { value: 'JumpToNode', label: 'Jump To Node' },
    { value: 'ForEachDelimited', label: 'For Each Delimited' },
    { value: 'Wait', label: 'Wait' },
    { value: 'Combine', label: 'Combine' }
  ]

  // Radio options for combine mode
  const combineModeOptions = [
    { value: 'newline', label: 'Newline' },
    { value: 'continuous', label: 'Continuous' }
  ]

  // Event handlers
  function handleTextareaMouseEnter() {
    emit('disable-zoom')
  }

  function handleTextareaMouseLeave() {
    emit('enable-zoom')
  }

  /**
   * The run() function for FlowControl.
   *
   * - In "RunAllChildren" mode, it does nothing special; the workflow runner
   *   in App.vue will handle propagating execution to children.
   *
   * - In "JumpToNode" mode, it returns a signal to the workflow runner
   *   indicating which node ID to jump execution to next.
   * 
   * - In "ForEachDelimited" mode, it splits the input by a delimiter and
   *   runs connected child nodes once for each split part.
   * 
   * - In "Wait" mode, it waits for the specified number of seconds before
   *   continuing execution.
   * 
   * - In "Combine" mode, it aggregates outputs from connected source nodes
   *   into a single string with line breaks.
   */
  async function run() {
    console.log(`Running FlowControl node: ${props.id} in mode: ${mode.value}`)

    props.data.outputs = {
      result: {
        output: ''
      }
    }

    const connectedSources = getEdges.value
      .filter((edge) => edge.target === props.id)
      .map((edge) => edge.source)

    console.log(`FlowControl (${props.id}): Connected sources: ${connectedSources}`)

    if (connectedSources.length > 0) {
      for (const sourceId of connectedSources) {
        const sourceNode = findNode(sourceId)
        if (sourceNode) {
          props.data.outputs.result.output = sourceNode.data.outputs.result.output

          console.log(`FlowControl (${props.id}): Output from source node ${sourceId}: ${props.data.outputs.result.output}`)
        }
      }
    }

    if (mode.value === 'RunAllChildren') {
      // No special action needed here. The main workflow runner will
      // process outgoing edges and continue execution concurrently.
      console.log(`FlowControl (${props.id}): RunAllChildren mode finished.`)
      return null // Indicate normal completion, allowing propagation
    } else if (mode.value === 'JumpToNode') {
      const targetId = targetNodeId.value
      if (!targetId) {
        console.warn(`FlowControl (${props.id}): JumpToNode mode selected, but no Target Node ID provided.`)
        return { stopPropagation: true } // Stop this execution path
      }

      // Check if target node exists
      const target = findNode(targetId)
      if (!target) {
        console.warn(`FlowControl (${props.id}): Target node ID "${targetId}" not found.`)
        return { stopPropagation: true }
      }

      // Set up the output data to be passed to the target node
      const outputToPass = props.data.outputs.result.output || ''
      
      // Store our output in the target node's data so it can use it as input
      // This creates a virtual source connection from this flow control node to the target
      if (target.data) {
        // Check if the target node has a virtualSources object already, create it if not
        if (!target.data.virtualSources) {
          target.data.virtualSources = {}
        }
        
        // Store the current flow control node as a virtual source for the target
        target.data.virtualSources[props.id] = {
          output: outputToPass,
          timestamp: Date.now()
        }
        
        console.log(`FlowControl (${props.id}): Set as virtual source for ${targetId} with output: "${outputToPass.substring(0, 50)}${outputToPass.length > 50 ? '...' : ''}"`)
      }

      console.log(`FlowControl (${props.id}): JumpToNode mode signaling jump to -> ${targetId}`)
      return { 
        jumpTo: targetId,
        virtualSourceId: props.id // Include this flow control node's ID as the virtual source
      } // Signal the jump to the workflow runner
    } else if (mode.value === 'Wait') {
      const seconds = waitTime.value
      
      if (!seconds || seconds <= 0) {
        console.warn(`FlowControl (${props.id}): Wait mode selected, but invalid wait time provided: ${seconds}`)
        return null // Continue execution if invalid time
      }
      
      console.log(`FlowControl (${props.id}): Waiting for ${seconds} seconds...`)
      
      // Use a promise to wait for the specified time
      await new Promise(resolve => setTimeout(resolve, seconds * 1000))
      
      console.log(`FlowControl (${props.id}): Wait complete after ${seconds} seconds.`)
      return null // Continue normal execution after waiting
    } else if (mode.value === 'ForEachDelimited') {
      // Handle For Each Delimited mode
      const currentDelimiter = delimiter.value
      
      if (!currentDelimiter) {
        console.warn(`FlowControl (${props.id}): ForEachDelimited mode selected, but no delimiter provided.`)
        return { stopPropagation: true }
      }

      // Get the input text from the source node
      const inputText = props.data.outputs.result.output || ''
      
      if (!inputText) {
        console.warn(`FlowControl (${props.id}): No input text available for splitting.`)
        return { stopPropagation: true }
      }

      // Split the input text by the delimiter
      const splitTexts = inputText.split(currentDelimiter).map(item => item.trim())
      console.log(`FlowControl (${props.id}): Split text into ${splitTexts.length} parts using delimiter: "${currentDelimiter}"`)

      // Get all immediate child nodes
      const childNodeIds = getEdges.value
        .filter(edge => edge.source === props.id)
        .map(edge => edge.target)

      if (childNodeIds.length === 0) {
        console.warn(`FlowControl (${props.id}): No child nodes connected to process split text.`)
        return { stopPropagation: true }
      }

      // Initialize or reset the state
      // Always create a fresh state at the beginning of each workflow run
      if (!props.data.forEachState || props.data.forEachState.reset || props.data.forEachState.completed) {
        console.log(`FlowControl (${props.id}): Initializing fresh state with ${splitTexts.length} items`)
        props.data.forEachState = { 
          currentIndex: 0, 
          totalItems: splitTexts.length,
          reset: false,
          completed: false
        }
      }

      // Check if we've processed all items
      if (props.data.forEachState.currentIndex >= props.data.forEachState.totalItems) {
        // We've finished processing all items, mark as completed
        console.log(`FlowControl (${props.id}): Completed processing all ${props.data.forEachState.totalItems} items.`)
        props.data.forEachState.completed = true
        return { stopPropagation: true }
      }

      // Get the current item to process
      const currentIndex = props.data.forEachState.currentIndex
      const splitText = splitTexts[currentIndex]
      
      console.log(`FlowControl (${props.id}): Processing part ${currentIndex+1}/${splitTexts.length}: "${splitText}"`)
      
      // Set the current split text as this node's output
      props.data.outputs.result.output = splitText
      
      // Increment the index for next iteration
      props.data.forEachState.currentIndex++
      
      // For each immediate child node, trigger execution
      for (const childId of childNodeIds) {
        // Create a special "jump" signal that will tell the workflow executor
        // to execute from this child node, but prevent normal propagation after completion
        console.log(`FlowControl (${props.id}): Executing workflow from child node ${childId} with input: "${splitText}"`)
        
        // This special signal tells App.vue to run the whole downstream workflow from this point
        // and then return to this node to process the next item
        return { 
          forEachJump: childId, 
          parentId: props.id,
          currentIndex: currentIndex + 1,
          totalItems: splitTexts.length
        }
      }
      
      // This should not happen if there are child nodes (we checked earlier)
      console.warn(`FlowControl (${props.id}): No children to execute for item ${currentIndex+1}.`)
      return { stopPropagation: true }
    } else if (mode.value === 'Combine') {
      console.log(`FlowControl (${props.id}): Combine mode activated.`)

      const connectedSources = getEdges.value
        .filter((edge) => edge.target === props.id)
        .map((edge) => edge.source)

      if (connectedSources.length === 0) {
        console.warn(`FlowControl (${props.id}): No connected source nodes to combine.`)
        return { stopPropagation: true }
      }

      let combinedOutput = ''
      for (const sourceId of connectedSources) {
        const sourceNode = findNode(sourceId)
        if (sourceNode && sourceNode.data.outputs.result.output) {
          combinedOutput += combineMode.value === 'newline'
            ? `${sourceNode.data.outputs.result.output}\n`
            : sourceNode.data.outputs.result.output
        }
      }

      // Remove trailing newline for newline mode
      if (combineMode.value === 'newline') {
        combinedOutput = combinedOutput.trim()
      }

      console.log(`FlowControl (${props.id}): Combined output: "${combinedOutput}"`)
      props.data.outputs.result.output = combinedOutput

      return null // Continue normal execution
    }

    // Default case (shouldn't happen with current modes)
    return null
  }

  // Expose run immediately for workflow engine access
  if (!props.data.run) {
    props.data.run = run
  }

  // Lifecycle management
  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run
    }
  })

  // Watch for data changes if loaded from file, update internal state
  watch(() => props.data.inputs, (newInputs) => {
    // This ensures computed props update if data is loaded externally
  }, { deep: true })

  return {
    // State
    mode,
    targetNodeId,
    delimiter,
    waitTime,
    combineMode,
    
    // Options
    modeOptions,
    combineModeOptions,
    
    // UI state
    isHovered,
    resizeHandleStyle,
    computedContainerStyle,
    
    // Event handlers
    handleTextareaMouseEnter,
    handleTextareaMouseLeave,
    
    // Core functionality
    run
  }
}
