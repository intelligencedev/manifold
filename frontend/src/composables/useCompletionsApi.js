import { fetchWithErrorHandling, handleStreamingResponse } from '@/utils/api';
import { formatRequestBody, buildReferenceString } from '@/utils/modelHelpers';

/**
 * Composable for managing API calls to completion endpoints
 */
export function useCompletionsApi() {
  /**
   * Call the combined retrieve API
   * @param {string} userPrompt - The user prompt to search for
   * @param {string} provider - The provider (openai, llama-server, etc)
   * @returns {Promise<Object>} - The retrieved documents
   */
  async function callCombinedRetrieveAPI(userPrompt, provider = 'llama-server') {
    const payload = {
      query: provider === 'openai' ? userPrompt : "retrieve: " + userPrompt,
      file_path_filter: "",
      limit: 3,
      use_inverted_index: true,
      use_vector_search: true,
      merge_mode: "weighted",
      return_full_docs: true,
      rerank: true,
      alpha: 0.5,
      beta: 0.9,
    };
    
    const retrieveEndpoint = "http://localhost:8080/api/sefii/combined-retrieve";
    
    try {
      return await fetchWithErrorHandling(retrieveEndpoint, {
        method: "POST",
        body: JSON.stringify(payload)
      });
    } catch (error) {
      console.error("Error calling combined retrieve API:", error);
      throw error;
    }
  }

  /**
   * Call the agentic memory API
   * @param {string} userPrompt - The user prompt to search for memories
   * @returns {Promise<Object>} - The retrieved memories
   */
  async function callAgenticMemoryAPI(userPrompt) {
    const payload = {
      query: userPrompt,
      limit: 3
    };
    
    const retrieveEndpoint = "http://localhost:8080/api/agentic-memory/search";
    
    try {
      return await fetchWithErrorHandling(retrieveEndpoint, {
        method: "POST",
        body: JSON.stringify(payload)
      });
    } catch (error) {
      console.error("Error calling agentic memory API:", error);
      // Optional: Return empty results instead of throwing
      return { results: [] };
    }
  }

  /**
   * Store a response in agentic memory
   * @param {string} responseText - The text to store
   * @param {Object} config - Configuration (endpoints, api keys)
   * @returns {Promise<Object>} - Response from API
   */
  async function storeResponseInAgenticMemory(responseText, config) {
    const ingestEndpoint = "http://localhost:8080/api/agentic-memory/ingest";
    const payload = {
      content: responseText,
      doc_title: "Agentic Response",
      completions_host: config.endpoint,
      completions_api_key: config.api_key,
      embeddings_host: config.embeddings?.Host,
      embeddings_api_key: config.embeddings?.APIKey
    };
    
    try {
      const response = await fetchWithErrorHandling(ingestEndpoint, {
        method: "POST",
        body: JSON.stringify(payload)
      });
      console.log("Stored response in agentic memory:", response);
      return response;
    } catch (error) {
      console.error("Error storing response in agentic memory:", error);
      return { error: error.message };
    }
  }

  /**
   * Call the completions API with streaming support
   * @param {Object} agentConfig - Configuration for the agent
   * @param {string} prompt - User prompt
   * @param {Function} onUpdate - Called when new tokens are received
   * @returns {Promise<Object>} - Final response
   */
  async function callCompletionsAPI(agentConfig, prompt, onUpdate) {
    const { 
      provider, endpoint, api_key, model, system_prompt, 
      max_completion_tokens, temperature, enableToolCalls 
    } = agentConfig;
    
    // Set up auth header
    const headers = {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${api_key}`
    };

    // If tool calls are disabled, make a straightforward streaming request
    if (!enableToolCalls) {
      const body = formatRequestBody(
        { provider, model, enableToolCalls },
        system_prompt,
        prompt,
        { temperature, max_tokens: max_completion_tokens }
      );
      
      const streamResponse = await fetch(endpoint, {
        method: "POST",
        headers,
        body: JSON.stringify(body),
        signal: AbortSignal.timeout(300000) // 5 minute timeout
      });
      
      let fullResponse = '';
      
      await handleStreamingResponse(
        streamResponse,
        (parsedData) => {
          const delta = parsedData.choices[0]?.delta || {};
          const tokenContent = (delta.content || "") + (delta.thinking || "");
          fullResponse += tokenContent;
          if (onUpdate) onUpdate(tokenContent, fullResponse);
        }
      );
      
      return { response: fullResponse };
    }
    
    // If tool calls are enabled, use the two-step workflow
    
    // Step 1: Make initial completion to potentially trigger a tool call
    const functionsConfig = getFunctionsConfig();
    
    const initialBody = formatRequestBody(
      { provider, model, enableToolCalls: true },
      system_prompt,
      prompt,
      { 
        temperature, 
        max_tokens: max_completion_tokens,
        functions: provider === 'openai' ? functionsConfig.openai : functionsConfig.local,
        function_call: provider === 'openai' ? "auto" : { name: "agentic_retrieve" }
      }
    );
    
    const responseData = await fetch(endpoint, {
      method: "POST",
      headers,
      body: JSON.stringify(initialBody)
    });
    
    const result = await responseData.json();
    const message = result.choices?.[0]?.message;
    
    // Check if a tool/function call was triggered
    let enhancedPrompt = prompt;
    
    const functionCallData = message?.function_call || 
                             (message?.tool_calls?.[0]?.function || {});
    
    const functionName = functionCallData?.name;
    
    if (functionName === "combined_retrieve") {
      const retrieveResult = await callCombinedRetrieveAPI(prompt, provider);
      const documents = retrieveResult.documents || {};
      let documentsString = '';
      
      if (typeof documents === 'object' && documents !== null) {
        documentsString = Object.entries(documents)
          .map(([key, value]) => `${key}:\n\n${String(value)}`)
          .join("\n\n");
      } else {
        documentsString = 'No valid documents found';
      }
      
      enhancedPrompt = `${prompt}\n\nREFERENCE:\n\n${documentsString}`;
    }
    
    if (functionName === "agentic_retrieve") {
      try {
        const retrieveResult = await callAgenticMemoryAPI(prompt);
        if (retrieveResult?.results) {
          const documents = retrieveResult.results;
          const documentsString = buildReferenceString(documents);
          enhancedPrompt = `${prompt}\n\nREFERENCE:\n\n${documentsString}`;
        }
      } catch (error) {
        console.warn("Error retrieving from agentic memory, continuing without retrieval");
      }
    }
    
    // Step 2: Make the final streaming request with the enhanced prompt
    const finalBody = formatRequestBody(
      { provider, model, enableToolCalls: false },
      "Use the provided documents to respond to the user's query. Be thorough and accurate and respond in a structured manner.",
      enhancedPrompt,
      { temperature, max_tokens: max_completion_tokens }
    );
    
    const streamResponse = await fetch(endpoint, {
      method: "POST",
      headers,
      body: JSON.stringify(finalBody)
    });
    
    let fullResponse = '';
    
    await handleStreamingResponse(
      streamResponse,
      (parsedData) => {
        const delta = parsedData.choices[0]?.delta || {};
        const tokenContent = (delta.content || "") + (delta.thinking || "");
        fullResponse += tokenContent;
        if (onUpdate) onUpdate(tokenContent, fullResponse);
      }
    );
    
    return { response: fullResponse };
  }

  /**
   * Get function definitions for tool calling
   * @returns {Object} Function definitions for various providers
   */
  function getFunctionsConfig() {
    const combinedRetrieveFunction = {
      name: "combined_retrieve",
      description: "Retrieves documents using a combined search that uses both an inverted index and vector search.",
      parameters: {
        type: "object",
        properties: {
          query: {
            type: "string",
            description: "The prompt or query to retrieve relevant documents."
          },
          file_path_filter: {
            type: "string",
            description: "An optional filter on file path. Leave empty to search all files.",
            default: ""
          },
          limit: {
            type: "number",
            description: "The number of documents or chunks to retrieve.",
            default: 3
          },
          use_inverted_index: {
            type: "boolean",
            description: "Whether to use the inverted index.",
            default: true
          },
          use_vector_search: {
            type: "boolean",
            description: "Whether to use vector search.",
            default: true
          },
          merge_mode: {
            type: "string",
            description: "The merge mode to combine results. For example, 'weighted'.",
            default: "weighted"
          },
          return_full_docs: {
            type: "boolean",
            description: "Return full documents rather than text chunks.",
            default: true
          },
          rerank: {
            type: "boolean",
            description: "Whether to rerank the results.",
            default: true
          },
          alpha: {
            type: "number",
            description: "The vector weight (alpha) when merge_mode is weighted.",
            default: 0.5
          },
          beta: {
            type: "number",
            description: "The keyword weight (beta) when merge_mode is weighted.",
            default: 0.9
          }
        },
        required: ["query", "limit", "merge_mode"]
      }
    };

    const agenticRetrieveFunction = {
      name: "agentic_retrieve",
      description: "Gets memories from previous discussions to help remember things.",
      parameters: {
        type: "object",
        properties: {
          query: {
            type: "string",
            description: "The prompt or query to retrieve relevant memories."
          },
          limit: {
            type: "number",
            description: "The number of memories to retrieve.",
            default: 3
          }
        },
        required: ["query"]
      }
    };

    const mcpServerFunctions = {
      "tools": [
        {
          "description": "Performs basic mathematical operations",
          "inputSchema": {
            "$schema": "https://json-schema.org/draft/2020-12/schema",
            "properties": {
              "a": { "description": "First number", "type": "number" },
              "b": { "description": "Second number", "type": "number" },
              "operation": {
                "description": "The mathematical operation to perform",
                "enum": ["add", "subtract", "multiply", "divide"],
                "type": "string"
              }
            },
            "required": ["operation", "a", "b"],
            "type": "object"
          },
          "name": "calculate"
        },
        {
          "description": "Says hello to the provided name",
          "inputSchema": {
            "$schema": "https://json-schema.org/draft/2020-12/schema",
            "properties": {
              "name": { "description": "The name to say hello to", "type": "string" }
            },
            "required": ["name"],
            "type": "object"
          },
          "name": "hello"
        },
        {
          "description": "Returns the current time",
          "inputSchema": {
            "$schema": "https://json-schema.org/draft/2020-12/schema",
            "properties": {
              "format": {
                "description": "Optional time format (default: RFC3339)",
                "type": "string"
              }
            },
            "type": "object"
          },
          "name": "time"
        }
      ]
    };

    return {
      openai: [mcpServerFunctions],
      local: [combinedRetrieveFunction, agenticRetrieveFunction]
    };
  }

  return {
    callCompletionsAPI,
    callCombinedRetrieveAPI,
    callAgenticMemoryAPI,
    storeResponseInAgenticMemory
  };
}