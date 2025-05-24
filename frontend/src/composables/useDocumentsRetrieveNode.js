import { computed, onMounted } from 'vue'
import { useVueFlow } from '@vue-flow/core'
import { useNodeBase } from './useNodeBase'

export function useDocumentsRetrieveNode(props, emit) {
  const { getEdges, findNode, updateNodeData } = useVueFlow()

  const {
    isHovered,
    customStyle,
    resizeHandleStyle,
    computedContainerStyle,
  } = useNodeBase(props, emit)

  const retrieve_endpoint = computed({
    get: () => props.data.inputs.retrieve_endpoint,
    set: (val) => { props.data.inputs.retrieve_endpoint = val }
  })

  const prompt = computed({
    get: () => props.data.inputs.text,
    set: (val) => { props.data.inputs.text = val }
  })

  const limit = computed({
    get: () => props.data.inputs.limit,
    set: (val) => { props.data.inputs.limit = val }
  })

  const merge_mode = computed({
    get: () => props.data.inputs.merge_mode,
    set: (val) => { props.data.inputs.merge_mode = val }
  })

  const return_full_docs = computed({
    get: () => props.data.inputs.return_full_docs,
    set: (val) => { props.data.inputs.return_full_docs = val }
  })

  const updateFromSource = computed({
    get: () => props.data.updateFromSource,
    set: (val) => { props.data.updateFromSource = val }
  })

  const alpha = computed({
    get: () => props.data.inputs.alpha ?? 0.7,
    set: (val) => { props.data.inputs.alpha = val }
  })

  const beta = computed({
    get: () => props.data.inputs.beta ?? 0.3,
    set: (val) => { props.data.inputs.beta = val }
  })

  async function retrieveDocuments(inputText) {
    const payload = {
      query: inputText.trim(),
      file_path_filter: '',
      limit: Number(limit.value),
      use_inverted_index: true,
      use_vector_search: true,
      merge_mode: merge_mode.value,
      return_full_docs: return_full_docs.value,
      rerank: true,
      alpha: merge_mode.value === 'weighted' ? Number(alpha.value) : 0.7,
      beta: merge_mode.value === 'weighted' ? Number(beta.value) : 0.3
    }

    const response = await fetch(retrieve_endpoint.value, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })

    if (!response.ok) {
      const errorText = await response.text()
      throw new Error(`API error (${response.status}): ${errorText}`)
    }

    return await response.json()
  }

  function formatOutput(responseData) {
    if (responseData.documents) {
      return Object.entries(responseData.documents)
        .map(([filePath, content]) => `Source: ${filePath}\n\n${content}`)
        .join('\n\n---\n\n')
    }

    if (responseData.chunks) {
      return responseData.chunks
        .map(chunk => `${chunk.content}\n\nSource: ${chunk.file_path}\n---`)
        .join('\n\n')
    }

    return 'No results found.'
  }

  function getConnectedNodesText() {
    const connectedSources = getEdges.value
      .filter(edge => edge.target === props.id)
      .map(edge => edge.source)

    if (connectedSources.length === 0 || !updateFromSource.value) {
      return prompt.value
    }

    return connectedSources
      .map(sourceId => {
        const sourceNode = findNode(sourceId)
        return sourceNode?.data.outputs?.result.output || ''
      })
      .filter(Boolean)
      .join('\n\n')
      .trim()
  }

  async function run() {
    try {
      const inputText = getConnectedNodesText()
      const responseData = await retrieveDocuments(inputText)
      const outputText = formatOutput(responseData)

      props.data.outputs = {
        result: { output: outputText }
      }
      updateNodeData()
      return { responseData }
    } catch (error) {
      console.error('Error in DocumentsRetrieveNode run:', error)
      return { error }
    }
  }

  function onResize(event) {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
    emit('resize', { event })
  }

  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run
    }
    if (props.data.style) {
      customStyle.value.width = props.data.style.width || '200px'
      customStyle.value.height = props.data.style.height || '150px'
    }
  })

  if (!props.data.style) {
    props.data.style = {
      border: '1px solid #666',
      borderRadius: '12px',
      backgroundColor: '#333',
      color: '#eee',
      width: '200px',
      height: '150px'
    }
  }
  customStyle.value.width = props.data.style.width || '200px'
  customStyle.value.height = props.data.style.height || '150px'

  return {
    isHovered,
    resizeHandleStyle,
    computedContainerStyle,
    retrieve_endpoint,
    prompt,
    limit,
    merge_mode,
    return_full_docs,
    updateFromSource,
    alpha,
    beta,
    onResize
  }
}

