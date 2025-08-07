/**
 * Utility functions for constructing API endpoints based on configuration
 */

/**
 * Constructs a base URL from host and port configuration
 * @param {Object} config - The configuration object
 * @param {string} config.Host - The hostname 
 * @param {number} config.Port - The port number
 * @returns {string} - The base URL (e.g., "http://localhost:8080")
 */
export function getBaseUrl(config) {
  if (!config?.Host || !config?.Port) {
    // Fallback to localhost:8080 if config not available
    return 'http://localhost:8080'
  }
  
  // Use http for localhost, https for other hosts (adjust as needed)
  const protocol = config.Host === 'localhost' ? 'http' : 'https'
  return `${protocol}://${config.Host}:${config.Port}`
}

/**
 * Constructs an API endpoint URL from config
 * @param {Object} config - The configuration object
 * @param {string} path - The API path (e.g., "/api/v1/chat/completions")
 * @returns {string} - The full endpoint URL
 */
export function getApiEndpoint(config, path) {
  const baseUrl = getBaseUrl(config)
  return `${baseUrl}${path}`
}

/**
 * Common API endpoints
 */
export const API_PATHS = {
  CHAT_COMPLETIONS: '/api/v1/chat/completions',
  SEFII_COMBINED_RETRIEVE: '/api/sefii/combined-retrieve',
  AGENTIC_MEMORY_SEARCH: '/api/agentic-memory/search',
  AGENTS_REACT_STREAM: '/api/agents/react/stream',
  CODE_EVAL: '/api/code/eval',
  SEFII_INGEST: '/api/sefii/ingest',
  SPLIT_TEXT: '/api/split-text'
}
