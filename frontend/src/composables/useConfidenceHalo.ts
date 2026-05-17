export function useConfidenceHalo(value: number | undefined) {
  const confidence = Math.max(0, Math.min(1, value ?? 0));
  const opacity = 0.2 + confidence * 0.6;
  const scale = 1 + confidence * 0.35;
  return { opacity, scale };
}
