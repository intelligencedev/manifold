import { computed, onMounted, watch } from 'vue'
import { useVueFlow } from '@vue-flow/core'
import { useNodeBase } from './useNodeBase'
import { useConfigStore } from '@/stores/configStore'
import { getApiEndpoint, API_PATHS } from '@/utils/endpoints'

export function useDocumentsRetrieveNode(props, emit) {
  const { getEdges, findNode, updateNodeData } = useVueFlow()
  const configStore = useConfigStore()

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

  // Update endpoint when config changes
  watch(
    () => configStore.config,
    (newConfig) => {
      if (newConfig) {
        const newEndpoint = getApiEndpoint(newConfig, API_PATHS.SEFII_COMBINED_RETRIEVE)
        if (props.data.inputs.retrieve_endpoint !== newEndpoint) {
          props.data.inputs.retrieve_endpoint = newEndpoint
        }
      }
    },
    { immediate: true }
  )

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

  const retrieval_mode = computed({
    get: () => props.data.inputs.retrieval_mode ?? 'combined',
    set: (val) => { props.data.inputs.retrieval_mode = val }
  })

  const context_window = computed({
    get: () => props.data.inputs.context_window ?? 2,
    set: (val) => { props.data.inputs.context_window = val }
  })

  const include_full_doc = computed({
    get: () => props.data.inputs.include_full_doc ?? false,
    set: (val) => { props.data.inputs.include_full_doc = val }
  })

  const chunk_id = computed({
    get: () => props.data.inputs.chunk_id ?? null,
    set: (val) => { props.data.inputs.chunk_id = val }
  })

  async function retrieveDocuments(inputText) {
    // Choose endpoint and payload based on retrieval mode
    let endpoint = retrieve_endpoint.value
    let payload = {}

    switch (retrieval_mode.value) {
      case 'contextual':
        endpoint = retrieve_endpoint.value.replace('/combined-retrieve', '/contextual-search')
        payload = {
          query: inputText.trim(),
          file_path_filter: "",
          limit: Number(limit.value),
          context_window: Number(context_window.value),
          include_full_doc: include_full_doc.value
        }
        break

      case 'neighbors':
        endpoint = retrieve_endpoint.value.replace('/combined-retrieve', '/chunk-neighbors')
        payload = {
          chunk_id: Number(chunk_id.value),
          context_window: Number(context_window.value),
          include_full_doc: include_full_doc.value
        }
        break

      case 'summary':
        endpoint = retrieve_endpoint.value.replace('/combined-retrieve', '/summary-search')
        payload = {
          query: inputText.trim(),
          file_path_filter: "",
          limit: Number(limit.value)
        }
        break

      case 'combined':
      default:
        payload = {
          query: inputText.trim(),
          file_path_filter: "",
          limit: Number(limit.value),
          use_inverted_index: true,
          use_vector_search: true,
          merge_mode: merge_mode.value,
          return_full_docs: return_full_docs.value,
          rerank: true,
          alpha: merge_mode.value === 'weighted' ? Number(alpha.value) : 0.7,
          beta: merge_mode.value === 'weighted' ? Number(beta.value) : 0.3
        }
        break
    }

    const response = await fetch(endpoint, {
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
    // Handle contextual search results
    if (responseData.results && Array.isArray(responseData.results)) {
      return responseData.results
        .map(result => {
          let output = ''
          
          // Add main chunk content
          if (result.content) {
            output += result.content
          }
          
          // Add neighbor chunks if available
          if (result.neighbor_chunks && result.neighbor_chunks.length > 0) {
            output += '\n\n--- Surrounding Context ---\n'
            result.neighbor_chunks.forEach((neighbor, idx) => {
              output += `\n[Chunk ${idx + 1}]: ${neighbor.content.substring(0, 200)}${neighbor.content.length > 200 ? '...' : ''}`
            })
          }
          
          // Add full document if available
          if (result.full_document) {
            output += '\n\n--- Full Document ---\n'
            output += result.full_document
          }
          
          // Add document stats if available
          if (result.document_stats) {
            output += `\n\n--- Document Info ---\n`
            output += `Title: ${result.document_stats.document_title || 'N/A'}\n`
            output += `Language: ${result.document_stats.language || 'N/A'}\n`
            output += `Total Chunks: ${result.document_stats.total_chunks || 'N/A'}`
          }
          
          // Add metadata
          if (result.metadata) {
            const keywords = result.metadata.keywords
            if (keywords) {
              output += `\n\nKeywords: ${keywords}`
            }
          }
          
          // Add source
          output += `\n\nSource: ${result.file_path}`
          
          return output
        })
        .join('\n\n=== Next Result ===\n\n')
    }

    // Handle neighbors endpoint response (single result)
    if (responseData.content) {
      let output = responseData.content
      
      if (responseData.neighbor_chunks && responseData.neighbor_chunks.length > 0) {
        output += '\n\n--- Surrounding Context ---\n'
        responseData.neighbor_chunks.forEach((neighbor, idx) => {
          output += `\n[Chunk ${idx + 1}]: ${neighbor.content.substring(0, 200)}${neighbor.content.length > 200 ? '...' : ''}`
        })
      }
      
      if (responseData.full_document) {
        output += '\n\n--- Full Document ---\n'
        output += responseData.full_document
      }
      
      output += `\n\nSource: ${responseData.file_path}`
      return output
    }

    // Handle combined retrieval with documents
    if (responseData.documents) {
      return Object.entries(responseData.documents)
        .map(([filePath, content]) => `Source: ${filePath}\n\n${content}`)
        .join('\n\n---\n\n')
    }

    // Handle chunk-based responses
    if (responseData.chunks) {
      return responseData.chunks
        .map(chunk => {
          let output = ''
          
          // Add summary if available
          if (chunk.summary && chunk.summary.trim()) {
            output += `Summary: ${chunk.summary}\n\n`
          }
          
          // Add main content
          output += chunk.content
          
          // Add metadata if available
          if (chunk.metadata) {
            const keywords = chunk.metadata.keywords
            if (keywords) {
              output += `\n\nKeywords: ${keywords}`
            }
          }
          
          // Add source
          output += `\n\nSource: ${chunk.file_path}`
          
          return output
        })
        .join('\n\n---\n\n')
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
    retrieval_mode,
    context_window,
    include_full_doc,
    chunk_id,
    onResize
  }
}

