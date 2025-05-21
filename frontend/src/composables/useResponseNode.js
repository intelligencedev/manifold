import { ref, computed, watch, nextTick, onMounted } from 'vue'
import { useVueFlow } from '@vue-flow/core'
import { marked } from 'marked'
import hljs from 'highlight.js'
import DOMPurify from 'dompurify'
import { useNodeBase } from './useNodeBase'

/**
 * Composable for ResponseNode behavior and state
 */
export function useResponseNode(props, emit) {
  const { getEdges, findNode, updateNodeData } = useVueFlow()

  const {
    isHovered,
    resizeHandleStyle,
    computedContainerStyle,
    onResize
  } = useNodeBase(props, emit)

  // Theme selection
  const selectedTheme = ref('atom-one-dark')
  const selectedModelType = ref('openai')
  let currentThemeLink = null

  // Model type label
  const modelTypeLabel = computed(() => {
    const labels = {
      openai: 'OpenAI Response',
      claude: 'Claude Response',
      gemini: 'Gemini Response'
    }
    return labels[selectedModelType.value] || 'Response'
  })

  // Load highlight.js theme
  function loadTheme(themeName) {
    if (currentThemeLink) {
      document.head.removeChild(currentThemeLink)
    }
    currentThemeLink = document.createElement('link')
    currentThemeLink.rel = 'stylesheet'
    currentThemeLink.href = `https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.7.0/styles/${themeName}.min.css`
    document.head.appendChild(currentThemeLink)
  }

  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run
    }

    loadTheme(selectedTheme.value)

    marked.setOptions({
      highlight(code, lang) {
        if (lang && hljs.getLanguage(lang)) {
          try {
            return hljs.highlight(code, { language: lang }).value
          } catch (e) {
            console.error(e)
          }
        }
        try {
          return hljs.highlightAuto(code).value
        } catch (e) {
          console.error(e)
        }
        return code
      }
    })
  })

  watch(selectedTheme, (newTheme) => {
    loadTheme(newTheme)
  })

  watch(selectedModelType, (newType) => {
    props.data.type = `${newType}Response`
    updateNodeData()
  })

  // Render mode
  const selectedRenderMode = ref('markdown')
  const textContainer = ref(null)
  const isAutoScrollEnabled = ref(true)

  // Copy feedback
  const copyStatus = ref('')
  const isCopying = ref(false)

  // Font size control
  const currentFontSize = ref(12)
  const minFontSize = 10
  const maxFontSize = 24
  const fontSizeStep = 2

  const increaseFontSize = () => {
    currentFontSize.value = Math.min(currentFontSize.value + fontSizeStep, maxFontSize)
  }

  const decreaseFontSize = () => {
    currentFontSize.value = Math.max(currentFontSize.value - fontSizeStep, minFontSize)
  }

  // Parsing logic
  const reRenderKey = ref(0)
  const thinkingBlocks = ref([])
  const outsideThinkingRaw = ref('')

  function processClaudeStreamingResponse(input) {
    if (!input.includes('event:')) {
      return input
    }

    let extractedText = ''
    const lines = input.split('\n')
    let i = 0

    while (i < lines.length) {
      const line = lines[i].trim()
      if (line.startsWith('event: content_block_delta')) {
        if (i + 1 < lines.length) {
          const dataLine = lines[i + 1]
          if (dataLine.startsWith('data:')) {
            try {
              const jsonStr = dataLine.substring(dataLine.indexOf('{'))
              const data = JSON.parse(jsonStr)
              if (data.type === 'content_block_delta' && data.delta && data.delta.type === 'text_delta' && data.delta.text) {
                extractedText += data.delta.text
              }
            } catch (e) {
              console.error('Error parsing Claude SSE JSON:', e)
            }
          }
        }
      }
      i++
    }

    return extractedText
  }

  function parseResponse(txt) {
    const previousStates = thinkingBlocks.value.map(b => b.collapsed)

    if (selectedModelType.value === 'claude' && txt.includes('event:')) {
      const processedText = processClaudeStreamingResponse(txt)
      const blocks = []
      const outside = []
      const regex = /<(?:think|thinking)>([\s\S]*?)(?:<\/(?:think|thinking)>|$)/gi
      let lastIndex = 0
      let match
      let blockIndex = 0

      while ((match = regex.exec(processedText)) !== null) {
        if (match.index > lastIndex) {
          outside.push(processedText.slice(lastIndex, match.index))
        }

        const full = match[1].trimEnd()
        const lines = full.split('\n')
        const preview = lines.slice(-2).join('\n')
        const wasCollapsed = blockIndex < previousStates.length ? previousStates[blockIndex] : true

        blocks.push({
          content: full,
          preview,
          hasMore: lines.length > 2,
          collapsed: wasCollapsed
        })

        lastIndex = match.index + match[0].length
        blockIndex++
      }

      if (lastIndex < processedText.length) {
        outside.push(processedText.slice(lastIndex))
      }

      thinkingBlocks.value = blocks
      outsideThinkingRaw.value = outside.join('')
      return
    }

    const blocks = []
    const outside = []
    const regex = /<(?:think|thinking)>([\s\S]*?)(?:<\/(?:think|thinking)>|$)/gi
    let lastIndex = 0
    let match
    let blockIndex = 0

    while ((match = regex.exec(txt)) !== null) {
      if (match.index > lastIndex) {
        outside.push(txt.slice(lastIndex, match.index))
      }

      const full = match[1].trimEnd()
      const lines = full.split('\n')
      const preview = lines.slice(-2).join('\n')
      const wasCollapsed = blockIndex < previousStates.length ? previousStates[blockIndex] : true

      blocks.push({
        content: full,
        preview,
        hasMore: lines.length > 2,
        collapsed: wasCollapsed
      })

      lastIndex = match.index + match[0].length
      blockIndex++
    }

    if (lastIndex < txt.length) {
      outside.push(txt.slice(lastIndex))
    }

    thinkingBlocks.value = blocks
    outsideThinkingRaw.value = outside.join('')
  }

  const markdownOutsideThinking = computed(() => marked(outsideThinkingRaw.value))
  const htmlOutsideThinking = computed(() => DOMPurify.sanitize(outsideThinkingRaw.value))

  function toggleThink(idx) {
    thinkingBlocks.value[idx].collapsed = !thinkingBlocks.value[idx].collapsed
  }

  const response = computed(() => props.data.inputs.response || '')

  const scrollToBottom = () => {
    nextTick(() => {
      if (textContainer.value) {
        textContainer.value.scrollTop = textContainer.value.scrollHeight
      }
    })
  }

  const handleScroll = () => {
    if (textContainer.value) {
      const { scrollTop, scrollHeight, clientHeight } = textContainer.value
      if (scrollTop + clientHeight < scrollHeight) {
        isAutoScrollEnabled.value = false
      } else {
        isAutoScrollEnabled.value = true
      }
    }
  }

  async function run() {
    console.log('Running ResponseNode:', props.id)

    const connectedSources = getEdges.value
      .filter(edge => edge.target === props.id)
      .map(edge => edge.source)

    if (connectedSources.length > 0) {
      const sourceNode = findNode(connectedSources[0])
      if (sourceNode && sourceNode.data.outputs.result) {
        props.data.inputs.response = sourceNode.data.outputs.result.output

        props.data.outputs = {
          result: { output: response.value }
        }

        reRenderKey.value++
        updateNodeData()
      }
    }

    props.data.outputs = {
      result: { output: response.value }
    }
  }

  function copyToClipboard() {
    if (isCopying.value) return
    isCopying.value = true
    navigator.clipboard.writeText(response.value)
      .then(() => { copyStatus.value = 'Copied!' })
      .catch((err) => { console.error('Failed to copy text:', err); copyStatus.value = 'Failed to copy.' })
      .finally(() => {
        setTimeout(() => {
          copyStatus.value = ''
          isCopying.value = false
        }, 2000)
      })
  }

  watch(() => props.data, (newData) => {
    emit('update:data', { id: props.id, data: newData })
    if (isAutoScrollEnabled.value) {
      scrollToBottom()
    }
  }, { deep: true })

  watch(selectedRenderMode, () => {
    nextTick(() => {
      if (isAutoScrollEnabled.value) {
        scrollToBottom()
      }
    })
  })

  watch(() => props.data.inputs.response, (newResponseText) => {
    parseResponse(newResponseText || '')
    nextTick(() => {
      if (isAutoScrollEnabled.value) {
        scrollToBottom()
      }
    })
  }, { immediate: true })

  watch(response, () => {
    nextTick(() => {
      document.querySelectorAll('pre code:not(.hljs)').forEach((block) => hljs.highlightElement(block))
    })
  })

  return {
    isHovered,
    resizeHandleStyle,
    computedContainerStyle,
    onResize,
    selectedTheme,
    selectedModelType,
    selectedRenderMode,
    modelTypeLabel,
    currentFontSize,
    increaseFontSize,
    decreaseFontSize,
    copyStatus,
    isCopying,
    copyToClipboard,
    textContainer,
    handleScroll,
    response,
    markdownOutsideThinking,
    htmlOutsideThinking,
    thinkingBlocks,
    toggleThink
  }
}

