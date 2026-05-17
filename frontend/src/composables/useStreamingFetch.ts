/**
 * Streams a POST request with `Accept: text/event-stream` and calls
 * `onEvent` for each parsed SSE data line.
 */
export async function useStreamingFetch(
  url: string,
  body: unknown,
  onEvent: (event: unknown) => void,
  signal?: AbortSignal
): Promise<void> {
  const response = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "text/event-stream",
    },
    body: JSON.stringify(body),
    credentials: "include",
    signal,
  });

  if (!response.ok) {
    throw new Error(`HTTP ${response.status}: ${response.statusText}`);
  }

  const reader = response.body?.getReader();
  if (!reader) return;

  const decoder = new TextDecoder();
  let buffer = "";

  try {
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const chunks = buffer.split("\n\n");
      buffer = chunks.pop() ?? "";
      for (const chunk of chunks) {
        for (const line of chunk.split("\n")) {
          if (line.startsWith("data: ")) {
            const data = line.slice(6).trim();
            if (!data || data === ":keepalive") continue;
            try {
              onEvent(JSON.parse(data));
            } catch {
              // Plain text delta — wrap it
              onEvent({ type: "delta", data });
            }
          }
        }
      }
    }
  } finally {
    reader.cancel();
  }
}
