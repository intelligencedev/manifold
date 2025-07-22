import { computed, onMounted } from 'vue'
import { useNodeBase } from './useNodeBase'

export function usePostgresNode(props, emit) {
  const {
    isHovered,
    customStyle,
    resizeHandleStyle,
    computedContainerStyle,
    onResize: baseOnResize
  } = useNodeBase(props, emit)

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
      const payload = {
        conn_string: connString.value,
        query: query.value
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

  onMounted(() => {
    if (!props.data.run) props.data.run = run
  })

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
