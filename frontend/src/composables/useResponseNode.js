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
      gfm: true, // Enable GitHub Flavored Markdown
      breaks: true, // Treat single line breaks as <br>
      headerIds: false, // Disable auto-generating header IDs
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
  const currentFontSize = ref(16)
  const minFontSize = 16
  const maxFontSize = 34
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
    let processedText = txt
    if (selectedModelType.value === 'claude' && txt.includes('event:')) {
      processedText = processClaudeStreamingResponse(txt)
    }

    const regex = /<(?:think|thinking)>([\s\S]*?)(?:<\/(?:think|thinking)>|$)/gi
    let match
    let lastMatch = null
    let lastIndex = 0
    while ((match = regex.exec(processedText)) !== null) {
      lastMatch = match
      lastIndex = regex.lastIndex
    }

    if (lastMatch) {
      const full = lastMatch[1].trimEnd()
      thinkingBlocks.value = [{ content: full }]
      reRenderKey.value++
      if (lastIndex < processedText.length) {
        outsideThinkingRaw.value = processedText.slice(lastIndex)
      } else {
        outsideThinkingRaw.value = ''
      }
    } else {
      thinkingBlocks.value = []
      outsideThinkingRaw.value = processedText
    }
  }

  const markdownOutsideThinking = computed(() => marked(outsideThinkingRaw.value))
  const htmlOutsideThinking = computed(() => DOMPurify.sanitize(outsideThinkingRaw.value))


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
    
    let textToCopy = '';
    
    // Copy text based on the selected render mode
    if (selectedRenderMode.value === 'markdown') {
      // For markdown, we want to copy the rendered text without HTML tags
      // Create a temporary DOM element to strip HTML tags
      const tempDiv = document.createElement('div');
      tempDiv.innerHTML = markdownOutsideThinking.value;
      textToCopy = tempDiv.textContent || tempDiv.innerText || outsideThinkingRaw.value;
    } else if (selectedRenderMode.value === 'html') {
      // For HTML, we also want to copy the rendered text without HTML tags
      const tempDiv = document.createElement('div');
      tempDiv.innerHTML = htmlOutsideThinking.value;
      textToCopy = tempDiv.textContent || tempDiv.innerText || outsideThinkingRaw.value;
    } else {
      // For raw text, just use the outside thinking raw text
      textToCopy = outsideThinkingRaw.value;
    }
    
    navigator.clipboard.writeText(textToCopy)
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
    outsideThinkingRaw,
    thinkingBlocks,
    reRenderKey
  }
}

