import { onMounted, computed } from 'vue'
import { useVueFlow } from '@vue-flow/core'

export default function useEmbeddingsNode(props, emit) {
  const { getEdges, findNode, updateNodeData } = useVueFlow()

  // The run() logic
  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run
    }
  })

  async function callEmbeddingsAPI(embeddingsNode, text) {
    // remove leading/trailing whitespace
    text = text.trim()

    const endpoint = embeddingsNode.data.inputs.embeddings_endpoint
    const response = await fetch(endpoint, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        input: [text],
        // key is required but the value is ignored by our backend for now since we start it with a model already configured
        model: "nomic-embed-text-v1.5.Q8_0",
        encoding_format: "float"
      }),
    })

    if (!response.ok) {
      const errorText = await response.text()
      throw new Error(`API error (${response.status}): ${errorText}`)
    }

    const responseData = await response.json()

    console.log('Embeddings API response:', responseData)

    return responseData
  }

  async function run() {
    console.log('Running EmbeddingsNode:', props.id)

    try {
      const embeddingsNode = findNode(props.id)
      let inputText = ""

      // Get connected source nodes
      const connectedSources = getEdges.value
        .filter((edge) => edge.target === props.id)
        .map((edge) => edge.source)

      // If there are connected sources, process their outputs
      if (connectedSources.length > 0) {
        console.log('Connected sources:', connectedSources)
        for (const sourceId of connectedSources) {
          const sourceNode = findNode(sourceId)
          if (sourceNode) {
            console.log('Processing source node:', sourceNode.id)
            inputText += `\n\n${sourceNode.data.outputs.result.output}`
          }
        }
      }

      inputText = inputText.trim()
      inputText = inputText.split(" ").slice(0, 512).join(" ")
      console.log('Processed input text:', inputText)

      const embeddingsData = await callEmbeddingsAPI(embeddingsNode, inputText)

      // convert embeddingsData to a string
      let embeddingsJson = JSON.stringify(embeddingsData, null, 2)

      console.log('Embeddings data:', embeddingsJson)

      // Update the node's output with the embeddings data
      props.data.outputs = {
        result: { output: embeddingsJson },
      }

      updateNodeData(props.id, props.data)

      return { embeddingsData }
    } catch (error) {
      console.error('Error in EmbeddingsNode run:', error)
      return { error }
    }
  }

  const embeddings_endpoint = computed({
    get: () => props.data.inputs.embeddings_endpoint,
    set: (value) => { props.data.inputs.embeddings_endpoint = value },
  })

  function onResize(event) {
    props.data.style.width = `${event.width}px`
    props.data.style.height = `${event.height}px`
    emit('resize', { id: props.id, width: event.width, height: event.height })
  }

  return {
    embeddings_endpoint,
    onResize
  }
}
