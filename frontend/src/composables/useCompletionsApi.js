import { fetchWithErrorHandling, handleStreamingResponse } from '@/utils/api';
import { formatRequestBody, buildReferenceString } from '@/utils/modelHelpers';
import { getApiEndpoint, API_PATHS } from '@/utils/endpoints';
import { useConfigStore } from '@/stores/configStore';

/**
 * Composable for managing API calls to completion endpoints
 */
export function useCompletionsApi() {
  const configStore = useConfigStore();

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
    
    const retrieveEndpoint = getApiEndpoint(configStore.config, API_PATHS.SEFII_COMBINED_RETRIEVE);
    
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
    
    const retrieveEndpoint = getApiEndpoint(configStore.config, API_PATHS.AGENTIC_MEMORY_SEARCH);
    
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
  async function callCompletionsAPI(agentConfig, prompt, onUpdate, abortSignal) {
    const { 
      provider, endpoint, api_key, model, system_prompt, 
      max_completion_tokens, temperature, reasoning_effort
    } = agentConfig;
    
    // Set up auth header
    const headers = {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${api_key}`
    };

    const body = formatRequestBody(
      { provider, model },
      system_prompt,
      prompt,
      { temperature, max_tokens: max_completion_tokens }
    );

    if (typeof model === 'string' && model.toLowerCase().startsWith('gpt-5')) {
      body.response_format = { type: 'text' };
      body.reasoning_effort = reasoning_effort || 'low';
      body.verbosity = 'medium';
    }
    
    const streamResponse = await fetch(endpoint, {
      method: "POST",
      headers,
      body: JSON.stringify(body),
      signal: abortSignal || AbortSignal.timeout(300000) // 5 minute timeout
    });
    
    let fullResponse = '';
    
    await handleStreamingResponse(
      streamResponse,
      (parsedData) => {
        // Handle different response formats
        let tokenContent = '';
        
        if (parsedData.choices && parsedData.choices[0]) {
          const choice = parsedData.choices[0];
          if (choice.delta) {
            // OpenAI-style streaming format
            const delta = choice.delta;
            tokenContent = (delta.content || "") + (delta.thinking || "");
          } else if (choice.text) {
            // Some LLM servers use 'text' field instead of 'delta.content'
            tokenContent = choice.text;
          } else if (typeof choice === 'string') {
            // Simple string format
            tokenContent = choice;
          }
        } else if (parsedData.content) {
          // Some LLM servers provide direct content field
          tokenContent = parsedData.content;
        } else if (typeof parsedData === 'string') {
          // Plain string format
          tokenContent = parsedData;
        }
        
        // Only update if we have content
        if (tokenContent) {
          console.log("Token content:", tokenContent);
          fullResponse += tokenContent;
          if (onUpdate) onUpdate(tokenContent);
        }
      }
    );
    
    return { response: fullResponse };
  }

  return {
    callCompletionsAPI,
    callCombinedRetrieveAPI,
    callAgenticMemoryAPI,
    storeResponseInAgenticMemory
  };
}
