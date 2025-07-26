import { ref, computed } from 'vue'
import { useVueFlow } from '@vue-flow/core'

export function useDocumentsRetrieve(props) {
  const { getEdges, findNode } = useVueFlow()
  
  const retrieve_endpoint = ref('http://localhost:8080/api/sefii/combined-retrieve')
  const prompt = ref('Enter prompt text here...')
  const limit = ref(1)
  const merge_mode = ref('intersect')
  const return_full_docs = ref(true)
  const updateFromSource = ref(true)
  const alpha = ref(0.7)
  const beta = ref(0.3)
  const use_summary_search = ref(false)
  const retrieval_mode = ref('combined') // 'combined', 'contextual', 'summary', 'neighbors'
  const context_window = ref(2)
  const include_full_doc = ref(false)
  const chunk_id = ref(null)

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
      body: JSON.stringify(payload),
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

  return {
    retrieve_endpoint,
    prompt,
    limit,
    merge_mode,
    return_full_docs,
    updateFromSource,
    alpha,
    beta,
    use_summary_search,
    retrieval_mode,
    context_window,
    include_full_doc,
    chunk_id,
    retrieveDocuments,
    formatOutput,
    getConnectedNodesText
  }
}