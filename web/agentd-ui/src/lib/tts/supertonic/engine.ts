// @ts-nocheck
import * as ort from "onnxruntime-web";
import {
  Style,
  TextToSpeech,
  AVAILABLE_LANGS,
  isValidLang,
} from "./vendor/helper.mjs";
import {
  ensureSupertonicAssets,
  onnxAssetPath,
  readAssetArrayBuffer,
  readAssetJson,
  voiceStyleAssetPath,
  type AssetProgress,
  type PresetVoiceId,
  isSupertonicCached,
} from "./assets";
import {
  isPresetVoiceId,
  type CustomVoiceStyle,
  type SupertonicTtsSettings,
} from "./settings";

export type EngineStatus =
  | "idle"
  | "downloading"
  | "loading"
  | "ready"
  | "error";

export type SupertonicLoadProgress =
  | AssetProgress
  | {
      stage: "session";
      file: string;
      current: number;
      total: number;
    }
  | { stage: "voice"; file: string; current: number; total: number };

export type SynthesizeOptions = {
  text: string;
  lang?: string;
  voiceId?: string;
  totalSteps?: number;
  speed?: number;
  silenceDuration?: number;
  signal?: AbortSignal;
};

export type SynthesizeResult = {
  samples: Float32Array;
  sampleRate: number;
  durationSec: number;
};

type VoiceStyleJson = {
  style_ttl: { dims: number[]; data: number[] | number[][][] };
  style_dp: { dims: number[]; data: number[] | number[][][] };
};

function flattenNumeric(data: unknown): number[] {
  if (typeof data === "number") return [data];
  if (!Array.isArray(data)) return [];
  const out: number[] = [];
  const walk = (node: unknown) => {
    if (typeof node === "number") {
      out.push(node);
      return;
    }
    if (Array.isArray(node)) {
      for (const child of node) walk(child);
    }
  };
  walk(data);
  return out;
}

function styleFromJson(voiceStyle: VoiceStyleJson): InstanceType<typeof Style> {
  const ttlDims = voiceStyle.style_ttl.dims;
  const dpDims = voiceStyle.style_dp.dims;
  const ttlFlat = new Float32Array(flattenNumeric(voiceStyle.style_ttl.data));
  const dpFlat = new Float32Array(flattenNumeric(voiceStyle.style_dp.data));
  const ttlTensor = new ort.Tensor("float32", ttlFlat, ttlDims);
  const dpTensor = new ort.Tensor("float32", dpFlat, dpDims);
  return new Style(ttlTensor, dpTensor);
}

function configureOrtWasm(): void {
  // Keep WASM artifacts on a public CDN so Vite does not need to density-copy them.
  // Override via VITE_ONNX_WASM_PATHS when self-hosting is required.
  const configured = import.meta.env.VITE_ONNX_WASM_PATHS as string | undefined;
  ort.env.wasm.wasmPaths =
    configured ||
    "https://cdn.jsdelivr.net/npm/onnxruntime-web@1.22.0/dist/";
  ort.env.wasm.numThreads = 1;
  ort.env.wasm.simd = true;
}

async function createSession(
  relativePath: string,
  options: ort.InferenceSession.SessionOptions,
): Promise<ort.InferenceSession> {
  const buffer = await readAssetArrayBuffer(relativePath);
  return ort.InferenceSession.create(buffer, options);
}

class SupertonicEngine {
  private status: EngineStatus = "idle";
  private errorMessage = "";
  private backend: "webgpu" | "wasm" | null = null;
  private tts: InstanceType<typeof TextToSpeech> | null = null;
  private styleCache = new Map<string, InstanceType<typeof Style>>();
  private customVoices = new Map<string, CustomVoiceStyle>();
  private loadPromise: Promise<void> | null = null;
  private progressListeners = new Set<
    (progress: SupertonicLoadProgress) => void
  >();
  private statusListeners = new Set<(status: EngineStatus) => void>();

  getStatus(): EngineStatus {
    return this.status;
  }

  getBackend(): "webgpu" | "wasm" | null {
    return this.backend;
  }

  getError(): string {
    return this.errorMessage;
  }

  isReady(): boolean {
    return this.status === "ready" && this.tts != null;
  }

  onProgress(listener: (progress: SupertonicLoadProgress) => void): () => void {
    this.progressListeners.add(listener);
    return () => this.progressListeners.delete(listener);
  }

  onStatus(listener: (status: EngineStatus) => void): () => void {
    this.statusListeners.add(listener);
    listener(this.status);
    return () => this.statusListeners.delete(listener);
  }

  private setStatus(status: EngineStatus) {
    this.status = status;
    for (const listener of this.statusListeners) listener(status);
  }

  private emitProgress(progress: SupertonicLoadProgress) {
    for (const listener of this.progressListeners) listener(progress);
  }

  setCustomVoices(voices: CustomVoiceStyle[]) {
    this.customVoices.clear();
    for (const voice of voices) {
      this.customVoices.set(voice.id, voice);
      this.styleCache.delete(voice.id);
    }
  }

  async ensureReady(
    settings?: Pick<SupertonicTtsSettings, "customVoices">,
  ): Promise<void> {
    if (settings?.customVoices) this.setCustomVoices(settings.customVoices);
    if (this.isReady()) return;
    if (this.loadPromise) return this.loadPromise;
    this.loadPromise = this.bootstrap().finally(() => {
      this.loadPromise = null;
    });
    return this.loadPromise;
  }

  private async bootstrap(): Promise<void> {
    configureOrtWasm();
    this.errorMessage = "";

    try {
      const cached = await isSupertonicCached();
      this.setStatus(cached ? "loading" : "downloading");
      await ensureSupertonicAssets((progress) => {
        if (progress.stage !== "done") {
          this.setStatus(
            progress.stage === "download" ? "downloading" : "loading",
          );
        }
        this.emitProgress(progress);
      });

      this.setStatus("loading");
      const sessionOptions: ort.InferenceSession.SessionOptions = {
        executionProviders: ["webgpu"],
        graphOptimizationLevel: "all",
      };

      try {
        this.tts = await this.loadPipeline(sessionOptions);
        this.backend = "webgpu";
      } catch (webgpuError) {
        console.warn(
          "Supertonic WebGPU backend unavailable, falling back to WASM",
          webgpuError,
        );
        this.tts = await this.loadPipeline({
          executionProviders: ["wasm"],
          graphOptimizationLevel: "all",
        });
        this.backend = "wasm";
      }

      this.setStatus("ready");
    } catch (error) {
      this.tts = null;
      this.backend = null;
      this.errorMessage =
        error instanceof Error ? error.message : String(error);
      this.setStatus("error");
      throw error;
    }
  }

  private async loadPipeline(
    sessionOptions: ort.InferenceSession.SessionOptions,
  ): Promise<InstanceType<typeof TextToSpeech>> {
    const cfgs = await readAssetJson(onnxAssetPath("tts.json"));
    const models = [
      { name: "Duration Predictor", path: onnxAssetPath("duration_predictor.onnx") },
      { name: "Text Encoder", path: onnxAssetPath("text_encoder.onnx") },
      { name: "Vector Estimator", path: onnxAssetPath("vector_estimator.onnx") },
      { name: "Vocoder", path: onnxAssetPath("vocoder.onnx") },
    ];

    const sessions: ort.InferenceSession[] = [];
    for (let i = 0; i < models.length; i++) {
      this.emitProgress({
        stage: "session",
        file: models[i].name,
        current: i + 1,
        total: models.length,
      });
      sessions.push(await createSession(models[i].path, sessionOptions));
    }

    const [dpOrt, textEncOrt, vectorEstOrt, vocoderOrt] = sessions;
    // UnicodeProcessor is constructed inside vendor loadTextProcessor equivalent:
    const indexer = await readAssetJson<number[]>(
      onnxAssetPath("unicode_indexer.json"),
    );
    // Dynamic import keeps helper types loose.
    const helper = await import("./vendor/helper.mjs");
    const textProcessor = new helper.UnicodeProcessor(indexer);
    return new helper.TextToSpeech(
      cfgs,
      textProcessor,
      dpOrt,
      textEncOrt,
      vectorEstOrt,
      vocoderOrt,
    );
  }

  private async resolveStyle(
    voiceId: string,
  ): Promise<InstanceType<typeof Style>> {
    const cached = this.styleCache.get(voiceId);
    if (cached) return cached;

    if (isPresetVoiceId(voiceId)) {
      this.emitProgress({
        stage: "voice",
        file: voiceId,
        current: 1,
        total: 1,
      });
      const json = await readAssetJson<VoiceStyleJson>(
        voiceStyleAssetPath(voiceId as PresetVoiceId),
      );
      const style = styleFromJson(json);
      this.styleCache.set(voiceId, style);
      return style;
    }

    const custom = this.customVoices.get(voiceId);
    if (!custom) {
      throw new Error(`Unknown voice style: ${voiceId}`);
    }
    const style = styleFromJson(custom.style as VoiceStyleJson);
    this.styleCache.set(voiceId, style);
    return style;
  }

  async synthesize(options: SynthesizeOptions): Promise<SynthesizeResult> {
    await this.ensureReady();
    if (!this.tts) throw new Error("Supertonic engine is not ready");
    if (options.signal?.aborted) throw new DOMException("Aborted", "AbortError");

    const text = options.text.trim();
    if (!text) {
      return {
        samples: new Float32Array(0),
        sampleRate: this.tts.sampleRate,
        durationSec: 0,
      };
    }

    const lang = options.lang && isValidLang(options.lang) ? options.lang : "en";
    const voiceId = options.voiceId || "M1";
    const totalStep = options.totalSteps ?? 5;
    const speed = options.speed ?? 1.05;
    const silenceDuration = options.silenceDuration ?? 0.25;
    const style = await this.resolveStyle(voiceId);

    if (options.signal?.aborted) throw new DOMException("Aborted", "AbortError");

    const { wav, duration } = await this.tts.call(
      text,
      lang,
      style,
      totalStep,
      speed,
      silenceDuration,
    );

    if (options.signal?.aborted) throw new DOMException("Aborted", "AbortError");

    const durationSec = Array.isArray(duration) ? Number(duration[0]) : Number(duration);
    const sampleRate = this.tts.sampleRate as number;
    const wavLen = Math.max(0, Math.floor(sampleRate * durationSec));
    const samples = Float32Array.from(wav.slice(0, wavLen));
    return { samples, sampleRate, durationSec };
  }
}

export const availableLanguages: string[] = AVAILABLE_LANGS;

export const supertonicEngine = new SupertonicEngine();
