import type { ChatStoreRefs, StreamState } from "@/stores/chatStoreState";

export function createStreamStateActions(refs: ChatStoreRefs) {
  function streamingStateFor(sessionId: string) {
    return refs.streamingStateBySession.value[sessionId];
  }
  return {
    isSessionStreaming: (sessionId: string) =>
      streamingStateFor(sessionId) !== undefined,
    streamingStateFor,
    setStreamingState: (sessionId: string, state: StreamState) => {
      refs.streamingStateBySession.value = {
        ...refs.streamingStateBySession.value,
        [sessionId]: state,
      };
    },
    clearStreamingState: (sessionId: string) => {
      if (!(sessionId in refs.streamingStateBySession.value)) return;
      const { [sessionId]: _removed, ...rest } =
        refs.streamingStateBySession.value;
      refs.streamingStateBySession.value = rest;
    },
    toolIndexFor: (sessionId: string, streamId: string) =>
      toolIndexFor(refs, sessionId, streamId),
    clearToolIndex: (sessionId: string, streamId: string) => {
      const entry = refs.toolMessageIndex.get(sessionId);
      if (entry?.streamId === streamId) refs.toolMessageIndex.delete(sessionId);
    },
    isStreamCurrent: (sessionId: string, streamId: string) => {
      const state = streamingStateFor(sessionId);
      return Boolean(state && state.streamId === streamId);
    },
  };
}

function toolIndexFor(
  refs: ChatStoreRefs,
  sessionId: string,
  streamId: string,
) {
  let entry = refs.toolMessageIndex.get(sessionId);
  if (!entry || entry.streamId !== streamId) {
    entry = { streamId, index: new Map<string, string>() };
    refs.toolMessageIndex.set(sessionId, entry);
  }
  return entry.index;
}
