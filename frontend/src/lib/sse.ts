export type SSEHandler<T = unknown> = (payload: T) => void;

/**
 * Opens a persistent SSE connection and calls `onMessage` for each parsed
 * JSON event. Returns the EventSource so the caller can `.close()` it.
 */
export function connectSSE<T = unknown>(
  url: string,
  onMessage: SSEHandler<T>,
  onError?: (e: Event) => void
): EventSource {
  const source = new EventSource(url, { withCredentials: true });

  source.onmessage = (event) => {
    try {
      const line = event.data as string;
      // Skip keepalive comments
      if (!line || line.startsWith(":")) return;
      onMessage(JSON.parse(line) as T);
    } catch {
      // Ignore malformed payloads
    }
  };

  if (onError) {
    source.onerror = onError;
  }

  return source;
}
