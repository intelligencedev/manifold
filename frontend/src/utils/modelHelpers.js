/**
 * Check if a model is an O1/O3 variant
 * @param {string} model - Model name to check
 * @returns {boolean} - True if the model is an O1/O3 variant
 */
export function isO1Model(model) {
  const lower = model.toLowerCase();
  return lower.startsWith("o1") || lower.startsWith("o3");
}

/**
 * Build a reference string from retrieved documents,
 * excluding keys like 'embedding' and 'links'
 * @param {Object} documents - The documents object
 * @returns {string} - Formatted reference string
 */
export function buildReferenceString(documents) {
  let reference = "";
  
  if (!documents || typeof documents !== 'object') {
    return 'No valid documents found';
  }
  
  Object.entries(documents).forEach(([key, value]) => {
    if (typeof value === 'object' && value !== null) {
      const filtered = Object.entries(value)
        .filter(([fieldKey]) => fieldKey !== 'embedding' && fieldKey !== 'links')
        .map(([fieldKey, fieldValue]) => `${fieldKey}: ${fieldValue}`)
        .join("\n");
      reference += `${key}:\n\n${filtered}\n\n`;
    } else {
      reference += `${key}:\n\n${String(value)}\n\n`;
    }
  });
  
  return reference.trim() || 'No valid documents found';
}

/**
 * Format agent request data based on model and provider
 * @param {Object} config - Configuration containing provider, model, etc.
 * @param {string} systemPrompt - System prompt text
 * @param {string} userPrompt - User prompt text
 * @param {Object} options - Additional options (temperature, max_tokens, etc.)
 * @returns {Object} - Formatted request body
 */
export function formatRequestBody(config, systemPrompt, userPrompt, options = {}) {
  const { provider, model, enableToolCalls } = config;
  const { temperature = 0.7, max_tokens = 4096 } = options;
  
  // Special case for O1 models which handle system prompts differently
  if (provider === 'openai' && isO1Model(model)) {
    return {
      model,
      max_completion_tokens: max_tokens,
      temperature,
      messages: [
        { role: "user", content: `${systemPrompt}\n\n${userPrompt}` }
      ],
      reasoning_effort: "high",
      stream: true,
      ...(!enableToolCalls ? {} : { functions: options.functions })
    };
  }
  
  // Build the base request body
  const requestBody = {
    max_completion_tokens: max_tokens,
    temperature,
    messages: [
      { role: "system", content: systemPrompt },
      { role: "user", content: userPrompt }
    ],
    stream: true
  };
  
  // Only add the model parameter for OpenAI provider
  if (provider === 'openai') {
    requestBody.model = model;
  }
  
  // Add function calling options if enabled
  if (enableToolCalls) {
    requestBody.functions = options.functions;
    requestBody.function_call = options.function_call;
  }
  
  return requestBody;
}