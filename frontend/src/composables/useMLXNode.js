import { ref, computed, onMounted, watch } from 'vue'
import { useVueFlow } from '@vue-flow/core'
import { useNodeBase } from './useNodeBase'

/**
 * Composable for managing MLXNode state and functionality
 */
export function useMLXNode(props, emit, vueFlow = useVueFlow()) {
  const { getEdges, findNode, updateNodeData } = vueFlow

  const {
    isHovered,
    customStyle,
    resizeHandleStyle,
    computedContainerStyle,
    onResize
  } = useNodeBase(props, emit)

  const imageSrc = ref('')

  const model = computed({
    get: () => props.data.inputs.model,
    set: value => updateNodeData(props.id, { ...props.data, inputs: { ...props.data.inputs, model: value } })
  })

  const prompt = computed({
    get: () => props.data.inputs.prompt,
    set: value => updateNodeData(props.id, { ...props.data, inputs: { ...props.data.inputs, prompt: value } })
  })

  const steps = computed({
    get: () => props.data.inputs.steps,
    set: value => updateNodeData(props.id, { ...props.data, inputs: { ...props.data.inputs, steps: value } })
  })

  const seed = computed({
    get: () => props.data.inputs.seed,
    set: value => updateNodeData(props.id, { ...props.data, inputs: { ...props.data.inputs, seed: value } })
  })

  const quality = computed({
    get: () => props.data.inputs.quality,
    set: value => updateNodeData(props.id, { ...props.data, inputs: { ...props.data.inputs, quality: value } })
  })

  const output = computed({
    get: () => props.data.inputs.output,
    set: value => updateNodeData(props.id, { ...props.data, inputs: { ...props.data.inputs, output: value } })
  })

  // Function to call the FMLX API (using a Go backend endpoint)
  async function callFMLXAPI(node) {
    const endpoint = '/api/run-fmlx'

    imageSrc.value = ''

    const requestBody = {
      model: node.data.inputs.model,
      prompt: node.data.inputs.prompt,
      steps: node.data.inputs.steps,
      seed: node.data.inputs.seed,
      quality: node.data.inputs.quality,
      output: node.data.inputs.output
    }

    try {
      const response = await fetch(endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(requestBody)
      })

      if (!response.ok) {
        const errorText = await response.text()
        throw new Error(`API error (${response.status}): ${errorText}`)
      }

      const result = await response.json()
      console.log('FMLX API response:', result)

      const currentUrl = window.location.href
      const imageUrl = `${currentUrl}tmp/${node.data.inputs.output}`

      updateNodeData(props.id, { ...props.data, outputs: { response: imageUrl } })
      imageSrc.value = imageUrl

      return { response: 'OK' }
    } catch (e) {
      console.error('Error calling fmlx api', e)
      return { error: e.message }
    }
  }

  async function run() {
    console.log('Running MLX node:', props.id)

    try {
      const connectedSources = getEdges.value
        .filter(edge => edge.target === props.id)
        .map(edge => edge.source)

      if (connectedSources.length > 0) {
        const sourceNode = findNode(connectedSources[0])
        if (sourceNode && sourceNode.data.outputs.result) {
          props.data.inputs.prompt = sourceNode.data.outputs.result.output
        }
      }

      return await callFMLXAPI(findNode(props.id))
    } catch (error) {
      console.error('Error in MLX run:', error)
      return { error }
    }
  }

  onMounted(() => {
    props.data.run = run
    if (props.data.outputs && props.data.outputs.response) {
      imageSrc.value = props.data.outputs.response
    }
  })

  watch(() => props.data.outputs.response, newVal => {
    if (newVal) {
      imageSrc.value = newVal
    }
  }, { immediate: true })

  return {
    isHovered,
    customStyle,
    resizeHandleStyle,
    computedContainerStyle,
    model,
    prompt,
    steps,
    seed,
    quality,
    output,
    imageSrc,
    run,
    onResize
  }
}
