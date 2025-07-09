import { ref, computed, onMounted, watch } from 'vue'
import { useVueFlow } from '@vue-flow/core'
import { useNodeBase } from './useNodeBase'

/**
 * Composable for managing MessageBus node state and functionality
 */
export function useMessageBus(props, emit) {
  const { getEdges, findNode } = useVueFlow()
  const { isHovered, resizeHandleStyle, computedContainerStyle, onResize } = useNodeBase(props, emit)

  // Initialize inputs if they don't exist
  if (!props.data.inputs) {
    props.data.inputs = {
      mode: 'publish',
      topic: 'default'
    }
  }

  // Initialize outputs if they don't exist
  if (!props.data.outputs) {
    props.data.outputs = { result: { output: '' } }
  }

  // Computed properties for inputs
  const mode = computed({
    get: () => props.data.inputs.mode || 'publish',
    set: (value) => { props.data.inputs.mode = value }
  })

  const topic = computed({
    get: () => props.data.inputs.topic || 'default',
    set: (value) => { props.data.inputs.topic = value.trim() }
  })

  // Mode options
  const modeOptions = [
    { value: 'publish', label: 'Publish' },
    { value: 'subscribe', label: 'Subscribe' }
  ]

  // Global singleton bus so every node shares the same queue
  const bus = (() => {
    if (window.__MANIFOLD_MESSAGE_BUS__) return window.__MANIFOLD_MESSAGE_BUS__
    const b = {
      topics: {},
      publish(topic, data) {
        if (!this.topics[topic]) this.topics[topic] = []
        this.topics[topic].push(data)
        console.log(`[MessageBus] publish → "${topic}" (${JSON.stringify(data).slice(0,80)}${JSON.stringify(data).length>80?'…':''})`)
      },
      consume(topic) {
        const q = this.topics[topic]
        if (!q || q.length === 0) return null
        const d = q.shift()
        console.log(`[MessageBus] consume ← "${topic}" (${JSON.stringify(d).slice(0,80)}${JSON.stringify(d).length>80?'…':''})`)
        return d
      }
    }
    window.__MANIFOLD_MESSAGE_BUS__ = b
    return b
  })()

  // Event handlers
  function handleTextareaMouseEnter() {
    emit('disable-zoom')
  }

  function handleTextareaMouseLeave() {
    emit('enable-zoom')
  }

  /**
   * Execution logic for MessageBus node
   */
  async function run() {
    console.log(`MessageBusNode ${props.id}: mode=${mode.value}`)

    // Collect upstream text
    const srcIds = getEdges.value.filter(e => e.target === props.id).map(e => e.source)
    let upstream = ''
    for (const id of srcIds) {
      const n = findNode(id)
      if (n?.data?.outputs?.result?.output) upstream += `${n.data.outputs.result.output}\n`
    }
    upstream = upstream.trim()

    // PUBLISH mode
    if (mode.value === 'publish') {
      let finalTopic = topic.value
      let payload = upstream || props.data.outputs.result.output || ''

      // Template detection
      if (/^\s*TOPIC\s*:.*\n\s*MESSAGE\s*:/i.test(upstream)) {
        const topicMatch = upstream.match(/TOPIC\s*:\s*(.+)/i)
        const messageMatch = upstream.match(/MESSAGE\s*:\s*([\s\S]*)/i)
        if (topicMatch && messageMatch) {
          finalTopic = topicMatch[1].trim()
          payload = messageMatch[1].trim()
          // Reflect parsed topic in UI
          props.data.inputs.topic = finalTopic
          console.log(`MessageBusNode ${props.id}: parsed template → topic="${finalTopic}"`)
        }
      }

      if (!finalTopic || !payload) {
        console.warn(`MessageBusNode ${props.id}: nothing to publish.`)
        return null
      }

      bus.publish(finalTopic, payload)
      props.data.outputs.result.output = payload
      return null
    }

    // SUBSCRIBE mode
    if (mode.value === 'subscribe') {
      let subscribeTopic = topic.value
      // Template detection for topic from upstream
      if (/^\s*TOPIC\s*:/i.test(upstream)) {
        const topicMatch = upstream.match(/TOPIC\s*:\s*(.+)/i)
        if (topicMatch) {
          subscribeTopic = topicMatch[1].trim()
          // Optionally reflect parsed topic in UI
          props.data.inputs.topic = subscribeTopic
          console.log(`MessageBusNode ${props.id}: parsed subscribe template → topic="${subscribeTopic}"`)
        }
      }

      if (!subscribeTopic) {
        console.warn(`MessageBusNode ${props.id}: subscribe topic empty.`)
        return { stopPropagation: true }
      }

      const busData = bus.consume(subscribeTopic)
      if (busData === null && !upstream) {
        console.log(`MessageBusNode ${props.id}: waiting for "${subscribeTopic}"…`)
        return { stopPropagation: true }
      }

      // Remove TOPIC: line from upstream if present
      let messageOnly = upstream
      if (/^\s*TOPIC\s*:/i.test(upstream)) {
        // Remove TOPIC: ... (and optional MESSAGE: ...)
        const messageMatch = upstream.match(/MESSAGE\s*:\s*([\s\S]*)/i)
        if (messageMatch) {
          messageOnly = messageMatch[1].trim()
        } else {
          // Remove just the TOPIC: line
          messageOnly = upstream.replace(/^\s*TOPIC\s*:.+$/im, '').trim()
        }
      }

      const combined = [messageOnly, busData].filter(Boolean).join('\n').trim()
      props.data.outputs.result.output = combined
      return null
    }

    return null
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
  }, { deep: true });

  return {
    // State
    mode,
    topic,
    
    // Options
    modeOptions,
    
    // UI state
    isHovered,
    resizeHandleStyle,
    computedContainerStyle,
    onResize,
    
    // Event handlers
    handleTextareaMouseEnter,
    handleTextareaMouseLeave,
    
    // Core functionality
    run
  }
}
