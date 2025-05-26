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
      
      // Process all complete lines in the buffer
      const lines = buffer.split('\n');
      buffer = lines.pop() || ""; // The last line might be incomplete
      
      for (const line of lines) {
        const trimmedLine = line.trim();
        if (!trimmedLine) continue;
        
        if (trimmedLine.startsWith("data: ")) {
          const jsonData = trimmedLine.substring(6);
          if (jsonData === "[DONE]") break;
          
          try {
            const parsedData = JSON.parse(jsonData);
            console.log("Processed chunk:", parsedData);
            onChunk(parsedData);
          } catch (e) {
            // If not valid JSON, treat the content as a direct token
            console.log("Treating as direct content:", jsonData);
            onChunk({ content: jsonData });
            console.error("Error parsing response chunk:", jsonData, e);
          }
        } else {
          // Try to parse the line as JSON even if it doesn't have the "data: " prefix
          try {
            const parsedData = JSON.parse(trimmedLine);
            console.log("Processed non-prefixed chunk:", parsedData);
            onChunk(parsedData);
          } catch (e) {
            // If not valid JSON, treat the content as a direct token
            console.log("Treating as direct content:", trimmedLine);
            onChunk({ content: trimmedLine });
            console.error("Error parsing non-data-prefixed response chunk:", trimmedLine, e);
          }
        }
      }
    }
    
    if (onDone) {
      onDone();
    }
  } catch (error) {
    console.error("Error reading stream:", error);
    throw error;
  }
}