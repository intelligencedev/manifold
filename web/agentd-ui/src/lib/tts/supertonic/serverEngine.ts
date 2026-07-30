// @ts-nocheck
// Server-backed TTS engine: instead of running Supertonic in-browser via
// onnxruntime-web (WebGPU/WASM), stream audio from Manifold's host-side /tts
// endpoint (tts.engine = born | supertonic). Mirrors the public surface of
// ./engine's supertonicEngine so the store and streamer use it unchanged.
import type {
  EngineStatus,
  SynthesizeOptions,
  SynthesizeResult,
} from "./constants";

const TTS_ENDPOINT = "/tts";

function decodePcm16Base64(b64: string): Int16Array {
  const bin = atob(b64);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  // Copy into an aligned buffer so Int16Array is always valid.
  return new Int16Array(
    bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength),
  );
}

function pcm16ToFloat32(pcm: Int16Array): Float32Array {
  const samples = new Float32Array(pcm.length);
  for (let i = 0; i < pcm.length; i++) samples[i] = pcm[i] / 32768;
  return samples;
}

type StreamChunkHandler = (
  samples: Float32Array,
  sampleRate: number,
) => void | Promise<void>;

class ServerTtsEngine {
  private status: EngineStatus = "ready";
  private statusListeners = new Set<(status: EngineStatus) => void>();

  getStatus(): EngineStatus {
    return this.status;
  }

  getBackend(): string {
    return "server";
  }

  getError(): string {
    return "";
  }

  isReady(): boolean {
    return true;
  }

  onProgress(): () => void {
    return () => {};
  }

  onStatus(listener: (status: EngineStatus) => void): () => void {
    this.statusListeners.add(listener);
    listener(this.status);
    return () => this.statusListeners.delete(listener);
  }

  setCustomVoices(): void {
    // Custom voices are a client-side (WebGPU) feature; the host engine uses
    // preset voices only. No-op here.
  }

  async ensureReady(): Promise<void> {
    // Host renders on demand; nothing to load in the browser.
  }

  async synthesize(options: SynthesizeOptions): Promise<SynthesizeResult> {
    const chunks: Float32Array[] = [];
    let total = 0;
    const result = await this.synthesizeStream(options, (samples) => {
      chunks.push(samples);
      total += samples.length;
    });

    const samples = new Float32Array(total);
    let offset = 0;
    for (const chunk of chunks) {
      samples.set(chunk, offset);
      offset += chunk.length;
    }
    return { ...result, samples };
  }

  async synthesizeStream(
    options: SynthesizeOptions,
    onChunk: StreamChunkHandler,
  ): Promise<SynthesizeResult> {
    const text = (options.text || "").trim();
    if (!text)
      return {
        samples: new Float32Array(0),
        sampleRate: 44100,
        durationSec: 0,
      };

    const body: Record<string, unknown> = { text };
    if (options.voiceId) body.voice = options.voiceId;
    if (options.lang) body.lang = options.lang;
    if (
      typeof options.totalSteps === "number" &&
      Number.isFinite(options.totalSteps)
    )
      body.totalSteps = Math.round(options.totalSteps);
    if (typeof options.speed === "number" && Number.isFinite(options.speed))
      body.speed = options.speed;

    let response: Response;
    try {
      response = await fetch(TTS_ENDPOINT, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
        signal: options.signal,
        credentials: "same-origin",
      });
    } catch (error) {
      if (options.signal?.aborted)
        throw new DOMException("Aborted", "AbortError");
      console.warn("Server TTS request failed:", error);
      return {
        samples: new Float32Array(0),
        sampleRate: 44100,
        durationSec: 0,
      };
    }

    if (!response.ok || !response.body) {
      console.warn(`Server TTS returned ${response.status}`);
      return {
        samples: new Float32Array(0),
        sampleRate: 44100,
        durationSec: 0,
      };
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffered = "";
    let sampleRate = 44100;
    let total = 0;
    let serverFailed = false;

    try {
      for (;;) {
        const { value, done } = await reader.read();
        if (done) break;
        buffered += decoder.decode(value, { stream: true });
        let sep: number;
        while ((sep = buffered.indexOf("\n\n")) !== -1) {
          const frame = buffered.slice(0, sep);
          buffered = buffered.slice(sep + 2);
          const line = frame.trim();
          if (!line.startsWith("data:")) continue;
          const payload = line.slice(5).trim();
          if (!payload) continue;
          let evt: Record<string, unknown>;
          try {
            evt = JSON.parse(payload);
          } catch {
            continue;
          }
          if (evt.type === "tts_chunk" && typeof evt.b64 === "string") {
            if (typeof evt.rate === "number" && evt.rate > 0)
              sampleRate = evt.rate;
            const pcm = decodePcm16Base64(evt.b64 as string);
            total += pcm.length;
            await onChunk(pcm16ToFloat32(pcm), sampleRate);
          } else if (evt.type === "error") {
            console.warn("Server TTS error:", evt.error);
            serverFailed = true;
            break;
          }
        }
        if (serverFailed) {
          await reader.cancel();
          break;
        }
      }
    } catch (error) {
      if (options.signal?.aborted)
        throw new DOMException("Aborted", "AbortError");
      console.warn("Server TTS stream error:", error);
    }

    return {
      samples: new Float32Array(0),
      sampleRate,
      durationSec: total / sampleRate,
    };
  }
}

export const serverEngine = new ServerTtsEngine();
