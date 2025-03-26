import { computed } from 'vue'
import { useVueFlow } from '@vue-flow/core'

/**
 * Composable for managing PythonRunner functionality
 * @param {Object} props - Component props
 * @param {Function} emit - Component emit function
 * @returns {Object} - PythonRunner functionality
 */
export function usePythonRunner(props, emit) {
  const { getEdges, findNode } = useVueFlow()

  // Command computed property for the multiline user input or JSON
  const command = computed({
    get: () => props.data.inputs?.command || '',
    set: (value) => {
      props.data.inputs.command = value
    }
  })

  // Label computed property for the node label
  const label = computed({
    get: () => props.data.type,
    set: (value) => {
      props.data.type = value
      updateNodeData()
    }
  })

  /**
   * Update node data and emit changes
   */
  function updateNodeData() {
    const updatedData = {
      ...props.data,
      inputs: {
        command: command.value,
      },
      outputs: props.data.outputs,
    }
    emit('update:data', { id: props.id, data: updatedData })
  }

  /**
   * Convert Python script to JSON format with dependencies
   * @param {string} script - Python script as raw text
   * @returns {string} - JSON string with code and dependencies
   */
  function pythonScriptToJson(script) {
    // Split the script into lines
    const lines = script.split('\\n');

    // Store dependencies in a Set to avoid duplicates
    const deps = new Set();

    // Regular expressions to capture Python imports
    const importRegex = /^import\\s+([\\w\\d_]+)(?:\\s+as\\s+\\w+)?(?:,\\s*[\\w\\d_]+(?:\\s+as\\s+\\w+)?)*$/;
    const fromRegex = /^from\\s+([\\w\\d_]+)\\s+import\\s+([\\w\\d_]+)(?:\\s+as\\s+\\w+)?/;

    for (const line of lines) {
      // Check for "import ..." pattern
      const importMatch = line.match(importRegex);
      if (importMatch) {
        // Split by commas for multiple imports on the same line
        const items = line.replace(/^import\\s+/, '').split(',');
        for (const item of items) {
          // Remove 'as' parts, keep the first token
          const cleaned = item.trim().split(/\\s+as\\s+/)[0];
          deps.add(cleaned);
        }
        continue;
      }

      // Check for "from ... import ..." pattern
      const fromMatch = line.match(fromRegex);
      if (fromMatch) {
        // The first capturing group after "from"
        deps.add(fromMatch[1]);
        // If you want to track sub-imports separately, handle them here
      }
    }

    // Prepare the final JSON object
    const output = {
      code: script,
      dependencies: Array.from(deps)
    };

    // Return it as a JSON string
    return JSON.stringify(output);
  }

  /**
   * Main run function that executes Python code
   * @returns {Promise<Object>} - Result of execution
   */
  async function run() {
    try {
      // Clear previous output
      props.data.outputs.result = '';

      // Identify connected source nodes
      const connectedSources = getEdges.value
        .filter((edge) => edge.target === props.id)
        .map((edge) => edge.source);

      let payload;

      if (connectedSources.length > 0) {
        // Source node might produce JSON
        const sourceData = findNode(connectedSources[0]).data.outputs.result.output;
        console.log('Connected source data:', sourceData);

        // Convert the Python script to JSON
        payload = pythonScriptToJson(sourceData);

        // Update the input field with the connected source data
        props.data.inputs.command = payload;

        // Attempt to parse the JSON produced by pythonScriptToJson
        try {
          payload = JSON.parse(payload);
        } catch (err) {
          console.error('Error parsing JSON from connected node:', err);
          props.data.outputs.result = {
            error: 'Invalid JSON from connected node',
          };
          return { error: 'Invalid JSON from connected node' };
        }
      } else {
        let userInput = props.data.inputs.command;
        // Attempt to parse as JSON
        try {
          payload = JSON.parse(userInput);
        } catch (_err) {
          // Not JSON => treat as raw Python code
          payload = {
            code: userInput, // Pass code as is, without escaping newlines
            dependencies: [],
          };
        }
      }

      // Default to empty code if none
      if (!payload.code) {
        payload.code = '';
      }
      // Ensure dependencies is an array
      if (!Array.isArray(payload.dependencies)) {
        payload.dependencies = [];
      }

      // POST to /api/executePython
      const response = await fetch('http://localhost:8080/api/executePython', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });

      if (!response.ok) {
        const errorMsg = await response.text();
        console.error('Error response from server:', errorMsg);
        props.data.outputs.result = { error: errorMsg };
        return { error: errorMsg };
      }

      const result = await response.json();
      console.log('Node-level run result:', result);

      // Parse the json for a stdout and stderr key and only return one or the other if its not empty
      const resultStr = result.stdout || result.stderr || '';

      props.data.outputs = {
        result: {
          output: resultStr,
        },
      }

      updateNodeData();

      return { response, result };
    } catch (error) {
      console.error('Error in run():', error);
      props.data.outputs.result = { error: error.message };
      return { error };
    }
  }

  return {
    command,
    label,
    updateNodeData,
    run
  }
}