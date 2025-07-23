import { computed, onMounted, watch } from 'vue'
import { useNodeBase } from './useNodeBase.js'
import { useConfigStore } from '@/stores/configStore'
import { useVueFlow } from '@vue-flow/core'

export function usePostgresNode(props, emit) {
  const { getEdges, findNode } = useVueFlow()
  const {
    isHovered,
    customStyle,
    resizeHandleStyle,
    computedContainerStyle,
    onResize: baseOnResize
  } = useNodeBase(props, emit)

  // Config store for loading default connection string
  const configStore = useConfigStore()

  if (!props.data.style) {
    props.data.style = {
      border: '1px solid #666',
      borderRadius: '12px',
      backgroundColor: '#333',
      color: '#eee',
      width: '320px',
      height: '200px'
    }
  }

  customStyle.value.width = props.data.style.width || '320px'
  customStyle.value.height = props.data.style.height || '200px'

  const connString = computed({
    get: () => props.data.inputs.conn_string,
    set: (val) => { props.data.inputs.conn_string = val }
  })

  const query = computed({
    get: () => props.data.inputs.query,
    set: (val) => { props.data.inputs.query = val }
  })

  function updateNodeData() {
    emit('update:data', { id: props.id, data: { ...props.data } })
  }

  async function run() {
    try {
      // --- Aggregate query from connected source nodes (like TextNode) ---
      const incomingEdges = getEdges.value.filter(edge => edge.target === props.id)
      let finalQuery = query.value
      if (incomingEdges.length > 0) {
        // For now, just use the first connected source node's output
        const sourceNode = findNode(incomingEdges[0].source)
        if (sourceNode && sourceNode.data?.outputs?.result?.output) {
          finalQuery = finalQuery + sourceNode.data.outputs.result.output
        }
      }

      const payload = {
        conn_string: connString.value,
        query: finalQuery
      }
      const response = await fetch(`${window.location.origin}/api/db/query`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      })
      if (!response.ok) {
        const text = await response.text()
        throw new Error(text || response.statusText)
      }
      const result = await response.text()
      props.data.outputs = { result: { output: result } }
      updateNodeData()
      return result
    } catch (err) {
      console.error('PostgresNode run error:', err)
      props.data.outputs = { result: { output: `Error: ${err.message}` } }
      updateNodeData()
      return { error: err.message }
    }
  }

  // Load default connection string from config if not set
  onMounted(() => {
    if (!props.data.run) props.data.run = run
    if (!props.data.inputs.conn_string && configStore.config?.Database?.ConnectionString) {
      props.data.inputs.conn_string = configStore.config.Database.ConnectionString
    }
  })

  // Also watch for config changes and update if not set
  watch(
    () => configStore.config?.Database?.ConnectionString,
    (newConnString) => {
      if (!props.data.inputs.conn_string && newConnString) {
        props.data.inputs.conn_string = newConnString
      }
    },
    { immediate: true }
  )

  function onResize(event) {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
    baseOnResize(event)
  }

  return {
    isHovered,
    resizeHandleStyle,
    computedContainerStyle,
    connString,
    query,
    onResize,
    run
  }
}
