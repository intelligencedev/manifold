<template>
  <div
    :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
    class="node-container message-bus-node tool-node"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label">Message&nbsp;Bus</div>

    <!-- Mode selector -->
    <div class="input-field">
      <label for="mode-select" class="input-label">Mode:</label>
      <select id="mode-select" v-model="mode" class="input-select">
        <option value="publish">Publish</option>
        <option value="subscribe">Subscribe</option>
      </select>
    </div>

    <!-- Topic entry -->
    <div class="input-field">
      <label :for="`${data.id}-topic`" class="input-label">Topic:</label>
      <input
        :id="`${data.id}-topic`"
        type="text"
        v-model="topic"
        class="input-text"
        placeholder="e.g. updates"
      />
    </div>

    <!-- Publish-mode usage hint -->
    <div v-if="mode === 'publish'" class="hint">
      <strong>Tip:</strong>  
      Upstream text may include  
      <code>TOPIC:</code> and <code>MESSAGE:</code> lines<br>
      to auto-populate topic &amp; payload, e.g.:<br>
      <pre>TOPIC: alerts
MESSAGE: service restarted</pre>
    </div>

    <!-- Input / output handles -->
    <Handle style="width:12px;height:12px" v-if="data.hasInputs" type="target" position="left" />
    <Handle style="width:12px;height:12px" v-if="data.hasOutputs" type="source" position="right" />

    <!-- Resizer -->
    <NodeResizer
      :is-resizable="true"
      :color="'#666'"
      :handle-style="resizeHandleStyle"
      :line-style="resizeHandleStyle"
      :width="320"
      :height="190"
      :min-width="320"
      :min-height="190"
      :node-id="props.id"
      @resize="onResize"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'

/* ------------------------------------------------------------------ */
/* ----  global singleton bus so every node shares the same queue ----*/
interface Bus {
  topics: Record<string, any[]>
  publish: (t: string, d: any) => void
  consume: (t: string) => any | null
}
const bus: Bus = (() => {
  if ((window as any).__MANIFOLD_MESSAGE_BUS__) return (window as any).__MANIFOLD_MESSAGE_BUS__
  const b: Bus = {
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
  ;(window as any).__MANIFOLD_MESSAGE_BUS__ = b
  return b
})()
/* ------------------------------------------------------------------ */

const props = defineProps({
  id: { type: String, required: true, default: 'MessageBus_0' },
  data: {
    type: Object,
    default: () => ({
      type: 'MessageBusNode',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: true,
      inputs: { mode: 'publish', topic: 'default' },
      outputs: { result: { output: '' } },
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '320px',
        height: '190px'
      }
    })
  }
})

const emit = defineEmits(['resize', 'disable-zoom', 'enable-zoom'])
const { getEdges, findNode } = useVueFlow()

/* ---- form bindings ---- */
const mode = computed({
  get: () => props.data.inputs.mode || 'publish',
  set: v => (props.data.inputs.mode = v)
})
const topic = computed({
  get: () => props.data.inputs.topic || 'default',
  set: v => (props.data.inputs.topic = v.trim())
})

/* ---- UI helpers ---- */
const isHovered = ref(false)
const customStyle = ref({
  width: props.data.style?.width || '320px',
  height: props.data.style?.height || '190px'
})
const resizeHandleStyle = computed(() => ({
  visibility: isHovered.value ? 'visible' : 'hidden',
  width: '12px',
  height: '12px'
}))
function onResize(e: any) {
  customStyle.value.width = `${e.width}px`
  customStyle.value.height = `${e.height}px`
  emit('resize', e)
}

/* ------------------  execution logic  ------------------ */
async function run() {
  console.log(`MessageBusNode ${props.id}: mode=${mode.value}`)

  /* collect upstream text */
  const srcIds = getEdges.value.filter(e => e.target === props.id).map(e => e.source)
  let upstream = ''
  for (const id of srcIds) {
    const n = findNode(id)
    if (n?.data?.outputs?.result?.output) upstream += `${n.data.outputs.result.output}\n`
  }
  upstream = upstream.trim()

  /* ----------------  PUBLISH  ---------------- */
  if (mode.value === 'publish') {
    let finalTopic = topic.value
    let payload = upstream || props.data.outputs.result.output || ''

    /* template detection */
    if (/^\s*TOPIC\s*:.*\n\s*MESSAGE\s*:/i.test(upstream)) {
      const topicMatch = upstream.match(/TOPIC\s*:\s*(.+)/i)
      const messageMatch = upstream.match(/MESSAGE\s*:\s*([\s\S]*)/i)
      if (topicMatch && messageMatch) {
        finalTopic = topicMatch[1].trim()
        payload = messageMatch[1].trim()
        // reflect parsed topic in UI
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

  /* ---------------- SUBSCRIBE --------------- */
  if (mode.value === 'subscribe') {
    if (!topic.value) {
      console.warn(`MessageBusNode ${props.id}: subscribe topic empty.`)
      return { stopPropagation: true }
    }

    const busData = bus.consume(topic.value)
    if (busData === null && !upstream) {
      console.log(`MessageBusNode ${props.id}: waiting for "${topic.value}"…`)
      return { stopPropagation: true }
    }

    const combined = [upstream, busData].filter(Boolean).join('\n').trim()
    props.data.outputs.result.output = combined
    return null
  }

  return null
}

/* lifecycle */
onMounted(() => {
  if (!props.data.run) props.data.run = run
  customStyle.value.width = props.data.style?.width || '320px'
  customStyle.value.height = props.data.style?.height || '190px'
})
watch(() => props.data.style, s => {
  customStyle.value.width = s?.width || '320px'
  customStyle.value.height = s?.height || '190px'
}, { deep: true })
</script>

<style scoped>
@import '@/assets/css/nodes.css';

.message-bus-node {
  display: flex;
  flex-direction: column;
  box-sizing: border-box;
  background-color: var(--node-bg-color, #333);
  border: 1px solid var(--node-border-color, #666);
  border-radius: 12px;
  color: var(--node-text-color, #eee);
  padding: 15px;
}

.node-label {
  font-size: 14px;
  font-weight: bold;
  text-align: center;
  margin-bottom: 12px;
}

.input-field {
  margin-bottom: 10px;
  text-align: left;
}

.input-label {
  display: block;
  font-size: 12px;
  margin-bottom: 4px;
  color: #ccc;
}

.input-select,
.input-text {
  width: 100%;
  padding: 6px 8px;
  background: #222;
  border: 1px solid #555;
  color: #eee;
  border-radius: 4px;
  font-size: 12px;
  box-sizing: border-box;
}

.input-select { appearance: none; }

/* hint box */
.hint {
  font-size: 11px;
  background: rgba(255,255,255,0.05);
  border: 1px dashed #555;
  border-radius: 6px;
  padding: 6px;
  margin-bottom: 6px;
  color: #ccc;
}
.hint pre {
  margin: 4px 0 0;
  background: none;
  padding: 0;
  font-size: 11px;
  color: #bbb;
}
</style>
