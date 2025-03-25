/**
 * Fetch API wrapper with common error handling
 * @param {string} url - The endpoint URL
 * @param {Object} options - Fetch options
 * @returns {Promise} - Response data
 */
export async function fetchWithErrorHandling(url, options = {}) {
  try {
    const response = await fetch(url, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        ...(options.headers || {})
      }
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`API error (${response.status}): ${errorText}`);
    }

    // Check if the response should be parsed as JSON
    const contentType = response.headers.get('content-type');
    if (contentType && contentType.includes('application/json')) {
      return await response.json();
    }

    return await response.text();
  } catch (error) {
    console.error(`Error fetching ${url}:`, error);
    throw error;
  }
}

/**
 * Create a reader for streaming API responses
 * @param {Response} response - Fetch API response object
 * @param {Function} onChunk - Callback for each chunk
 * @param {Function} onDone - Callback when stream is complete
 * @returns {Promise} - Resolves when stream is complete
 */
export async function handleStreamingResponse(response, onChunk, onDone) {
  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`API error (${response.status}): ${errorText}`);
  }

  const reader = response.body.getReader();
  let buffer = "";

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      
      const chunk = new TextDecoder().decode(value);
      buffer += chunk;
      
      let start = 0;
      for (let i = 0; i < buffer.length; i++) {
        if (buffer[i] === "\n") {
          const line = buffer.substring(start, i).trim();
          start = i + 1;
          
          if (line.startsWith("data: ")) {
            const jsonData = line.substring(6);
            if (jsonData === "[DONE]") break;
            
            try {
              const parsedData = JSON.parse(jsonData);
              onChunk(parsedData);
            } catch (e) {
              console.error("Error parsing response chunk:", e);
            }
          }
        }
      }
      
      buffer = buffer.substring(start);
    }
    
    if (onDone) {
      onDone();
    }
  } catch (error) {
    console.error("Error reading stream:", error);
    throw error;
  }
}