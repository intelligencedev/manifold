import { useVueFlow } from '@vue-flow/core';

export function useTokenCounterNode(props) {
  const { getEdges, findNode, updateNodeData } = useVueFlow();

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

  return {
    updateInputData,
    run
  };
}