import { ref, computed, watch } from 'vue';
import { useVueFlow } from '@vue-flow/core';

/**
 * Composable for managing OpenFileNode functionality
 * @param {Object} props - Component props
 * @param {Function} emit - Component emit function
 * @returns {Object} - OpenFileNode functionality
 */
export function useOpenFileNode(props, emit) {
  const { getEdges, findNode } = useVueFlow();

  // File path input
  const filepath = computed({
    get: () => props.data.inputs.filepath,
    set: (value) => {
      props.data.inputs.filepath = value;
    },
  });

  // Option to update from source
  const updateFromSource = ref(props.data.updateFromSource);

  /**
   * Updates the node data and emits changes
   */
  const updateNodeData = () => {
    const updatedData = {
      ...props.data,
      inputs: {
        filepath: filepath.value,
      },
      outputs: props.data.outputs,
      updateFromSource: updateFromSource.value,
    };
    emit('update:data', { id: props.id, data: updatedData });
  };

  /**
   * Main run function that opens and reads a file
   * @returns {Promise<Object>} Result of the operation
   */
  async function run() {
    console.log('Running OpenFileNode:', props.id);

    // Identify connected source nodes
    const connectedSources = getEdges.value
      .filter((edge) => edge.target === props.id)
      .map((edge) => edge.source);

    let payload;

    if (connectedSources.length > 0 && updateFromSource.value) {
      // Source node might produce JSON
      const sourceData = findNode(connectedSources[0]).data.outputs.result.output;
      console.log('Connected source data:', sourceData);

      // Update the input field with the connected source data
      props.data.inputs.filepath = sourceData;

      // Attempt to parse JSON
      try {
        payload = JSON.parse(sourceData);
      } catch (err) {
        console.error('Error parsing JSON from connected node:', err);
        props.data.outputs.result = {
          error: 'Invalid JSON from connected node',
        };
        return { error: 'Invalid JSON from connected node' };
      }
    } else {
      // No connected nodes or updateFromSource is false => user typed something in the textarea
      payload = { filepath: props.data.inputs.filepath };
    }

    try {
      const response = await fetch('http://localhost:8080/api/open-file', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          filepath: payload.filepath,
        }),
      });

      if (!response.ok) {
        const errorData = await response.json();
        console.error('Error reading file content:', errorData.error);
        props.data.outputs.result = {
          error: errorData.error,
        };
        return { error: errorData.error }; // Return error object
      } else {
        const fileContent = await response.text(); // Get as text
        console.log('File content:', fileContent);
        props.data.outputs = {
          result: {
            output: fileContent,
          },
        };
      }
    } catch (error) {
      console.error('Error opening file:', error);
      props.data.outputs.result = {
        error: error.message,
      };
      return { error: error.message }; // Return error object
    }

    updateNodeData(); // Update data after processing
    return { result: props.data.outputs.result };
  }

  // Watch for filepath changes
  watch(filepath, () => {
    updateNodeData();
  });

  return {
    filepath,
    updateFromSource,
    updateNodeData,
    run
  };
}