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

  async function retrieveDocuments(inputText) {
    // Choose endpoint based on search mode
    const endpoint = use_summary_search.value 
      ? retrieve_endpoint.value.replace('/combined-retrieve', '/summary-search')
      : retrieve_endpoint.value

    const payload = {
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
    if (responseData.documents) {
      return Object.entries(responseData.documents)
        .map(([filePath, content]) => `Source: ${filePath}\n\n${content}`)
        .join('\n\n---\n\n')
    } 
    
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
    retrieveDocuments,
    formatOutput,
    getConnectedNodesText
  }
}