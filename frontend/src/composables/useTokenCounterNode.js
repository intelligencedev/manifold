import { computed, onMounted } from 'vue'
import { useVueFlow } from '@vue-flow/core'
import { useNodeBase } from './useNodeBase'

export function useTokenCounterNode(props, emit) {
  const { getEdges, findNode, updateNodeData } = useVueFlow();

  const {
    isHovered,
    customStyle,
    resizeHandleStyle,
    computedContainerStyle,
    onResize: baseOnResize,
  } = useNodeBase(props, emit)

  const endpoint = computed({
    get: () => props.data.inputs.endpoint,
    set: (value) => { props.data.inputs.endpoint = value }
  })

  const api_key = computed({
    get: () => props.data.inputs.api_key,
    set: (value) => { props.data.inputs.api_key = value }
  })

  const tokenCount = computed(() => props.data.tokenCount)

  /**
   *  updateInputData: Persist data changes. Called when inputs change.
   */
  function updateInputData() {
    updateNodeData({ id: props.id, data: props.data });
  }

  /**
   * callTokenizeAPI: calls the /v1/tokenize endpoint with the provided text.
   */
  async function callTokenizeAPI(text) {
    const endpoint = props.data.inputs.endpoint;
    const apiKey = props.data.inputs.api_key;
    const response = await fetch(`${endpoint}/tokenize`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${apiKey}`,
      },
      body: JSON.stringify({
        content: text,
        add_special: false,
        with_pieces: false,
      }),
    });
    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`API error (${response.status}): ${errorText}`);
    }
    return await response.json();
  }

  /**
   * run: invoked by external logic or from a parent node,
   * collects text from connected source nodes, calls callTokenizeAPI,
   * and updates the token count on this node.
   */
  async function run() {
    console.log('Running TokenCounterNode:', props.id);
    try {
      // Find this node
      const tokenNode = findNode(props.id);
      if (!tokenNode) {
        throw new Error(`Node with id "${props.id}" not found`);
      }
      let combinedText = '';
      // Gather text from all connected source nodes
      const edges = getEdges.value.filter(edge => edge.target === props.id);
      for (const edge of edges) {
        const sourceNode = findNode(edge.source);
        if (sourceNode && sourceNode.data?.outputs?.result?.output) {
          combinedText += sourceNode.data.outputs.result.output;
        }
      }
      // Call the tokenize endpoint
      const responseData = await callTokenizeAPI(combinedText);
      const tokens = responseData.tokens ?? [];
      // Update the token count in the node's data
      tokenNode.data.tokenCount = tokens.length;
      console.log('Token count:', tokenNode.data.tokenCount);
        
      // Persist data changes. Calling this here is less jumpy than using a watcher.
      updateNodeData({
          id: tokenNode.id,
          data: tokenNode.data,
      });
        
      return { tokens };
    } catch (error) {
      console.error('Error in TokenCounterNode run:', error);
      return { error };
    }
  }

  function onResize(event) {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
    props.data.style = props.data.style || {}
    props.data.style.width = `${event.width}px`
    props.data.style.height = `${event.height}px`
    updateInputData()
    emit('resize', event)
  }

  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run
    }
    if (props.data.style) {
      customStyle.value.width = props.data.style.width || '200px'
      customStyle.value.height = props.data.style.height || '160px'
    }
  })

  return {
    endpoint,
    api_key,
    tokenCount,
    onResize,
    updateInputData,
    resizeHandleStyle,
    computedContainerStyle
  };
}

