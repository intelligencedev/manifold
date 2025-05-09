import { ref, computed, onMounted } from 'vue'
import { useVueFlow } from '@vue-flow/core'

export default function useTextNode(props, emit) {
  const { getEdges, findNode } = useVueFlow()
  
  // Custom style for handling resizes
  const customStyle = ref({})
  
  // Track whether the node is hovered (to show/hide the resize handles)
  const isHovered = ref(false)
  
  const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
    width: '12px',
    height: '12px',
  }))
  
  // Computed property for two-way binding of the text input
  const text = computed({
    get: () => props.data.inputs.text,
    set: (value) => {
      props.data.inputs.text = value
      updateNodeData()
    }
  })
  
  // Clear on run checkbox binding
  const clearOnRun = computed({
    get: () => props.data.clearOnRun || false,
    set: (value) => {
      props.data.clearOnRun = value
      updateNodeData()
    }
  })

  // Mode selection (text or template)
  const mode = computed({
    get: () => props.data.mode || 'text',
    set: (value) => {
      props.data.mode = value
      updateNodeData()
    }
  })
  
  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run
    }
    
    // Initialize clearOnRun if not set
    if (props.data.clearOnRun === undefined) {
      props.data.clearOnRun = false
    }

    // Initialize mode if not set
    if (props.data.mode === undefined) {
      props.data.mode = 'text'
    }
  })
  
  // Execute the node's logic
  async function run() {
    const originalText = props.data.inputs.text
    const connectedSources = getEdges.value
      .filter((edge) => edge.target === props.id)
      .map((edge) => edge.source)
  
    // Get input from connected nodes if any
    if (connectedSources.length > 0) {
      const sourceNode = findNode(connectedSources[0])
      if (sourceNode && sourceNode.data.outputs.result) {
        props.data.inputs.text = props.data.inputs.text + sourceNode.data.outputs.result.output
      }
    }
  
    let processedText = props.data.inputs.text

    // Process template if in template mode
    if (props.data.mode === 'template') {
      try {
        processedText = parseUnifiedInput(props.data.inputs.text)
      } catch (err) {
        console.error('Template parsing error:', err)
        processedText = `ERROR: ${err.message}`
      }
    }

    // Set the output
    props.data.outputs = {
      result: {
        output: processedText
      }
    }
    
    // If clearOnRun is enabled, clear the text after setting the output
    if (props.data.clearOnRun) {
      props.data.inputs.text = ''
      updateNodeData()
    }

    // log the output
    console.log('TextNode output:', props.data.outputs.result.output)
    updateNodeData()
  }
  
  // Template parsing function (based on the provided HTML example)
  function parseUnifiedInput(src) {
    const tmplStart = /^---\s*template:([A-Za-z0-9_-]+)\s*---$/i;
    const valStart  = /^---\s*values:([A-Za-z0-9_-]+)\s*---$/i;
    const tmplEnd   = /^---\s*endtemplate\s*---$/i;
    const valEnd    = /^---\s*endvalues\s*---$/i;

    const templates = Object.create(null);
    const valuesMap = Object.create(null);
    const templateBlocks = [];

    let mode = null;      // 'template' | 'values' | null
    let current = '';     // section name
    let buf = [];
    let currentBlock = null;

    function flush() {
      if (!mode) return;
      const content = buf.join('\n').trim();
      
      if (mode === 'template') {
        templates[current] = content;
        if (currentBlock) {
          currentBlock.templateContent = content;
        }
      } else {
        const kv = Object.create(null);
        content.split(/\r?\n/).forEach(line => {
          const idx = line.indexOf('=');
          if (idx !== -1) {
            const k = line.slice(0, idx).trim();
            const v = line.slice(idx + 1).trim();
            kv[k] = v;
          }
        });
        valuesMap[current] = kv;
        if (currentBlock && currentBlock.name === current) {
          currentBlock.values = kv;
        }
      }
      buf = [];
    }

    const lines = src.split(/\r?\n/);
    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];
      let m;
      
      if ((m = line.match(tmplStart))) { 
        flush(); 
        mode = 'template'; 
        current = m[1]; 
        currentBlock = { 
          name: current, 
          templateHeader: line,
          templateContent: '',
          templateFooter: '',
          values: {}
        };
        templateBlocks.push(currentBlock);
        continue;
      }
      
      if ((m = line.match(valStart))) { 
        flush(); 
        mode = 'values'; 
        current = m[1]; 
        
        // Find corresponding template block
        if (!currentBlock || currentBlock.name !== current) {
          const existingBlock = templateBlocks.find(b => b.name === current);
          if (existingBlock) {
            currentBlock = existingBlock;
          }
        }
        continue;
      }
      
      if (tmplEnd.test(line)) { 
        flush(); 
        mode = null; 
        if (currentBlock) {
          currentBlock.templateFooter = line;
        }
        continue;
      }
      
      if (valEnd.test(line)) { 
        flush(); 
        mode = null;
        continue;
      }
      
      if (mode) { buf.push(line); }
    }
    flush(); // final flush

    // Render each template block, excluding values blocks
    return templateBlocks.map(block => {
      const tpl = block.templateContent;
      const vals = block.values || {};
      
      // Process template content, preserving unmatched variables
      const processedContent = tpl.replace(/\{\{([A-Za-z0-9_]+)\}\}/g, (match, key) =>
        Object.prototype.hasOwnProperty.call(vals, key) ? vals[key] : match
      );
      
      // Only include the template sections, not the values sections
      return [
        block.templateHeader,
        processedContent,
        block.templateFooter
      ].filter(Boolean).join('\n');
    }).join('\n\n');
  }
  
  // Emit updated node data back to VueFlow
  function updateNodeData() {
    const updatedData = {
      ...props.data,
      inputs: { text: text.value },
      outputs: props.data.outputs,
      clearOnRun: clearOnRun.value,
      mode: mode.value
    }
    emit('update:data', { id: props.id, data: updatedData })
  }
  
  // Handle the resize event to update the node dimensions
  const onResize = (event) => {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
    // Also update the node's style data so it persists
    props.data.style.width = `${event.width}px`
    props.data.style.height = `${event.height}px`
    updateNodeData()
    emit('resize', { id: props.id, width: event.width, height: event.height })
  }
  
  return {
    text,
    clearOnRun,
    mode,
    customStyle,
    isHovered,
    resizeHandleStyle,
    updateNodeData,
    onResize,
    run
  }
}