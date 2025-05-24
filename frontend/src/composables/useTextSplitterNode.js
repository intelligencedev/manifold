import { ref, watch, computed, onMounted } from 'vue'
import { useVueFlow } from '@vue-flow/core'
import { useNodeBase } from './useNodeBase'

export function useTextSplitterNode(props, emit) {
  const { getEdges, findNode } = useVueFlow()
  const {
    isHovered,
    customStyle,
    resizeHandleStyle,
    computedContainerStyle,
    onResize
  } = useNodeBase(props, emit)

  const endpoint = computed({
    get: () => props.data.inputs?.endpoint || 'http://localhost:8080/api/split-text',
    set: (val) => { props.data.inputs.endpoint = val }
  })

  const text = computed({
    get: () => props.data.inputs?.text || '',
    set: (val) => { props.data.inputs.text = val }
  })
  const outputConnectionCount = ref(0)

  watch(
    () => getEdges.value,
    () => {
      updateOutputConnections();
    },
    { deep: true }
  );

  const updateOutputConnections = async () => {
    // Find connected output edges
    const outputEdges = getEdges.value.filter((edge) => edge.source === props.id);
    outputConnectionCount.value = outputEdges.length;

    // Update the node data with the new output connection count
    updateNodeData();
  };

  const updateNodeData = async () => {
    const updatedData = {
      ...props.data,
      inputs: {
        endpoint: endpoint.value,
        text: text.value,
      },
      outputs: {}, // Initialize outputs as an empty object
      num_chunks: outputConnectionCount.value
    };

    // Get connected input edges and their source nodes
    const inputEdges = getEdges.value.filter((edge) => edge.target === props.id && edge.targetHandle === 'input');
    for (const edge of inputEdges) {
      const sourceNode = findNode(edge.source);
      if (sourceNode && sourceNode.data.outputs) {
        // Assuming the text to be split is in the 'output' property of the source node's outputs
        if (sourceNode.data.outputs[edge.sourceHandle]) {
          updatedData.inputs.text = sourceNode.data.outputs[edge.sourceHandle];
        }
      }
    }
    emit('update:data', { id: props.id, data: updatedData });
  };

  const run = async () => {
    console.log('Running TextSplitterNode:', props.id);

    const connectedTargetEdges = getEdges.value.filter(
      (edge) => edge.target === props.id
    );

    // Get the first connected edge
    const targetEdge = connectedTargetEdges[0];

    console.log('Connected target edge:', targetEdge);

    // Get the source node of the connected edge
    const sourceNode = findNode(targetEdge.source);

    console.log('Source node:', sourceNode);

    // Get the response value from the source node's outputs
    const response = sourceNode.data.outputs.result.output;

    console.log('Response:', response);

    // Update the input text with the response value
    text.value = response;

    // Update the node data with the new input text
    updateNodeData();

    // Get the source edges
    const sourceEdges = getEdges.value.filter(
      (edge) => edge.source === props.id
    );

    // Get the target nodes of the source edges
    const targetNodes = sourceEdges.map((edge) => findNode(edge.target));

    console.log('Target nodes:', targetNodes);
    
    // First get a count of the source connections
    const sourceCount = sourceEdges.length;

    // Split the text into chunks based on the number of source connections
    const words = text.value.split(' ');
    const wordsPerChunk = Math.ceil(words.length / sourceCount);
    const chunks = [];

    for (let i = 0; i < sourceCount; i++) {
      const start = i * wordsPerChunk;
      const end = Math.min(start + wordsPerChunk, words.length);
      chunks.push(words.slice(start, end).join(' '));

      // Update the outputs for each target node
      if (targetNodes[i]) {
        console.log('Updating target node:', targetNodes[i]);
        targetNodes[i].data.inputs.response += chunks[i];
        targetNodes[i].data.inputs.text += chunks[i];

        updateNodeData();
      }
    }

    console.log('Chunks:', chunks);
  };

  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run;
    }
    if (!props.data.style) {
      props.data.style = {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '320px',
        height: '180px'
      };
    }
    customStyle.value.width = props.data.style.width || '320px';
    customStyle.value.height = props.data.style.height || '180px';
  })

  // Return the values and methods needed in the component
  return {
    isHovered,
    resizeHandleStyle,
    computedContainerStyle,
    endpoint,
    text,
    updateNodeData,
    run,
    onResize
  };
}