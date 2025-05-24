import { ref, watch, onMounted } from 'vue'
import { useVueFlow } from '@vue-flow/core'
import { useNodeBase } from './useNodeBase'

export function useWebSearch(props, emit) {
  const { getEdges, findNode } = useVueFlow()
  const {
    isHovered,
    customStyle,
    resizeHandleStyle,
    computedContainerStyle,
  } = useNodeBase(props, emit)

  // Reactive state
  const query = ref(props.data.inputs?.query || '')
  const resultSize = ref(props.data.inputs?.result_size || 1)
  const searchBackend = ref(props.data.inputs?.search_backend || 'ddg')
  const sxngUrl = ref(props.data.inputs?.sxng_url || 'https://searx.be')

  // Main run function
  async function run() {
    console.log('Running WebSearchNode:', props.id)
    
    // Check for connected input nodes
    const connectedTargetEdges = getEdges.value.filter(
      (edge) => edge.target === props.id
    )
    
    if (connectedTargetEdges.length > 0) {
      const targetEdge = connectedTargetEdges[0]
      console.log('Connected target edge:', targetEdge)
      
      const sourceNode = findNode(targetEdge.source)  
      query.value = sourceNode.data.outputs.result.output
    }

    updateNodeData()
    console.log('Query value:', props.data.inputs.query)

    // Perform Web Search
    let webUrls = []
    try {
      const { query, result_size, search_backend, sxng_url } = props.data.inputs
      let apiURL = `http://localhost:8080/api/web-search?query=${encodeURIComponent(
        query
      )}&result_size=${result_size}&search_backend=${search_backend}`
      
      if (search_backend === 'sxng') {
        apiURL += `&sxng_url=${encodeURIComponent(sxng_url)}`
      }
      
      const response = await fetch(apiURL)
      if (!response.ok) {
        const errorText = await response.text()
        throw new Error(
          `Web Search API error (${response.status}): ${errorText}`
        )
      }
      
      webUrls = await response.json()
      props.data.outputs.result.output = webUrls
      
      console.log('WebSearchNode run result:', props.data.outputs.result.output)
      updateNodeData()
    } catch (error) {
      console.error('Error in WebSearchNode run:', error)
      props.data.error = error.message
      return { error: error.message }
    }
  }

  // Watch for input changes
  watch(
    [query, resultSize, searchBackend, sxngUrl],
    ([newQuery, newResultSize, newSearchBackend, newSxngUrl]) => {
      props.data.inputs.query = newQuery
      props.data.inputs.result_size = newResultSize
      props.data.inputs.search_backend = newSearchBackend
      props.data.inputs.sxng_url = newSxngUrl
      updateNodeData()
    },
    { deep: true }
  )

  // Watch for data changes
  watch(
    () => props.data,
    (newData) => {
      emit('update:data', { id: props.id, data: newData })
    },
    { deep: true }
  )

  // Update node data
  function updateNodeData() {
    emit('update:data', {
      id: props.id,
      data: {
        ...props.data,
        inputs: {
          query: query.value,
          result_size: resultSize.value,
          search_backend: searchBackend.value,
          sxng_url: sxngUrl.value,
        },
      },
    })
  }

  // Setup function
  function setup() {
    if (!props.data.run) {
      props.data.run = run
    }
    if (props.data.style) {
      customStyle.value.width = props.data.style.width || '352px'
      customStyle.value.height = props.data.style.height || '200px'
    }
  }

  function onResize(event) {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
    emit('resize', { event })
  }

  onMounted(() => {
    setup()
    if (!props.data.style) {
      props.data.style = {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '352px',
        height: '200px'
      }
    }
    customStyle.value.width = props.data.style.width || '352px'
    customStyle.value.height = props.data.style.height || '200px'
  })

  return {
    isHovered,
    resizeHandleStyle,
    computedContainerStyle,
    query,
    resultSize,
    searchBackend,
    sxngUrl,
    updateNodeData,
    run,
    setup,
    onResize
  }
}