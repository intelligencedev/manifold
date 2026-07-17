export const SUPERTONIC_MODEL_ID = "Supertone/supertonic-3";
export const SUPERTONIC_HF_BASE = `https://huggingface.co/${SUPERTONIC_MODEL_ID}/resolve/main`;
export const SUPERTONIC_CACHE_NAME = "manifold-supertonic-3-v1";

export const ONNX_FILES = [
  "duration_predictor.onnx",
  "text_encoder.onnx",
  "vector_estimator.onnx",
  "vocoder.onnx",
  "tts.json",
  "unicode_indexer.json",
] as const;

export const PRESET_VOICE_IDS = [
  "M1",
  "M2",
  "M3",
  "M4",
  "M5",
  "F1",
  "F2",
  "F3",
  "F4",
  "F5",
] as const;

export type PresetVoiceId = (typeof PRESET_VOICE_IDS)[number];

export type AssetProgress = {
  stage: "cache" | "download" | "done";
  file: string;
  current: number;
  total: number;
  bytesReceived?: number;
  bytesTotal?: number;
};

function assetPath(kind: "onnx" | "voice_styles", name: string): string {
  return kind === "onnx" ? `onnx/${name}` : `voice_styles/${name}`;
}

export function huggingfaceUrl(relativePath: string): string {
  return `${SUPERTONIC_HF_BASE}/${relativePath}?download=true`;
}

export function allRequiredAssetPaths(): string[] {
  return [
    ...ONNX_FILES.map((name) => assetPath("onnx", name)),
    ...PRESET_VOICE_IDS.map((id) => assetPath("voice_styles", `${id}.json`)),
  ];
}

function cacheAvailable(): boolean {
  return typeof caches !== "undefined";
}

async function openCache(): Promise<Cache | null> {
  if (!cacheAvailable()) return null;
  return caches.open(SUPERTONIC_CACHE_NAME);
}

export async function getCachedResponse(
  relativePath: string,
): Promise<Response | null> {
  const cache = await openCache();
  if (!cache) return null;
  const match = await cache.match(huggingfaceUrl(relativePath));
  return match ?? null;
}

export async function readAssetArrayBuffer(
  relativePath: string,
): Promise<ArrayBuffer> {
  const cached = await getCachedResponse(relativePath);
  if (cached) return cached.arrayBuffer();
  const response = await fetch(huggingfaceUrl(relativePath));
  if (!response.ok) {
    throw new Error(
      `Failed to fetch Supertonic asset ${relativePath}: ${response.status}`,
    );
  }
  const cache = await openCache();
  if (cache) {
    try {
      await cache.put(huggingfaceUrl(relativePath), response.clone());
    } catch {
      // Cache write failures (quota, private mode) should not block synthesis.
    }
  }
  return response.arrayBuffer();
}

export async function readAssetJson<T = unknown>(
  relativePath: string,
): Promise<T> {
  const buffer = await readAssetArrayBuffer(relativePath);
  const text = new TextDecoder().decode(buffer);
  return JSON.parse(text) as T;
}

export async function isSupertonicCached(): Promise<boolean> {
  const cache = await openCache();
  if (!cache) return false;
  for (const path of allRequiredAssetPaths()) {
    const hit = await cache.match(huggingfaceUrl(path));
    if (!hit) return false;
  }
  return true;
}

export async function ensureSupertonicAssets(
  onProgress?: (progress: AssetProgress) => void,
): Promise<void> {
  const paths = allRequiredAssetPaths();
  const total = paths.length;
  let current = 0;

  for (const path of paths) {
    current += 1;
    const cached = await getCachedResponse(path);
    if (cached) {
      onProgress?.({ stage: "cache", file: path, current, total });
      continue;
    }

    onProgress?.({ stage: "download", file: path, current, total });
    const response = await fetch(huggingfaceUrl(path));
    if (!response.ok) {
      throw new Error(
        `Failed to download Supertonic asset ${path}: ${response.status}`,
      );
    }

    const cache = await openCache();
    if (cache) {
      try {
        await cache.put(huggingfaceUrl(path), response.clone());
      } catch {
        // ignore cache persistence failures
      }
    } else {
      // still consume body so network fetch completes without keeping large clones
      await response.arrayBuffer();
    }
  }

  onProgress?.({ stage: "done", file: "", current: total, total });
}

export function voiceStyleAssetPath(voiceId: string): string {
  return assetPath("voice_styles", `${voiceId}.json`);
}

export function onnxAssetPath(fileName: string): string {
  return assetPath("onnx", fileName);
}
