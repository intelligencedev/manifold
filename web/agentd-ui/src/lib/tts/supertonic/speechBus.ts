type DeltaListener = (
  sessionId: string,
  assistantId: string,
  delta: string,
) => void;
type FinalListener = (
  sessionId: string,
  assistantId: string,
  fullText: string,
) => void;
type StopListener = (sessionId?: string) => void;
type RollbackListener = (
  sessionId: string,
  assistantId: string,
  chars: number,
) => void;

const deltaListeners = new Set<DeltaListener>();
const finalListeners = new Set<FinalListener>();
const stopListeners = new Set<StopListener>();
const rollbackListeners = new Set<RollbackListener>();

export function onAssistantDelta(listener: DeltaListener): () => void {
  deltaListeners.add(listener);
  return () => deltaListeners.delete(listener);
}

export function onAssistantFinal(listener: FinalListener): () => void {
  finalListeners.add(listener);
  return () => finalListeners.delete(listener);
}

export function onAssistantStop(listener: StopListener): () => void {
  stopListeners.add(listener);
  return () => stopListeners.delete(listener);
}

export function onAssistantRollback(listener: RollbackListener): () => void {
  rollbackListeners.add(listener);
  return () => rollbackListeners.delete(listener);
}

export function emitAssistantDelta(
  sessionId: string,
  assistantId: string,
  delta: string,
) {
  for (const listener of deltaListeners) {
    try {
      listener(sessionId, assistantId, delta);
    } catch (error) {
      console.warn("assistant delta TTS listener failed", error);
    }
  }
}

export function emitAssistantFinal(
  sessionId: string,
  assistantId: string,
  fullText: string,
) {
  for (const listener of finalListeners) {
    try {
      listener(sessionId, assistantId, fullText);
    } catch (error) {
      console.warn("assistant final TTS listener failed", error);
    }
  }
}

export function emitAssistantStop(sessionId?: string) {
  for (const listener of stopListeners) {
    try {
      listener(sessionId);
    } catch (error) {
      console.warn("assistant stop TTS listener failed", error);
    }
  }
}

export function emitAssistantRollback(
  sessionId: string,
  assistantId: string,
  chars: number,
) {
  for (const listener of rollbackListeners) {
    try {
      listener(sessionId, assistantId, chars);
    } catch (error) {
      console.warn("assistant rollback TTS listener failed", error);
    }
  }
}
