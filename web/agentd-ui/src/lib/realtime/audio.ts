export type VoiceActivityEvent = "speech-start" | "speech-end" | null;

export type VoiceActivityUpdate = {
  event: VoiceActivityEvent;
  speaking: boolean;
  level: number;
  threshold: number;
  noiseFloor: number;
  snrDb: number;
  speechProbability: number;
  rejectedNoise: boolean;
};

export type VoiceActivityHints = {
  speechProbability?: number;
  noiseFloor?: number;
  snrDb?: number;
};

export type VoiceActivityOptions = {
  startFrames?: number;
  endFrames?: number;
  minimumThreshold?: number;
  thresholdMultiplier?: number;
  speechProbabilityStart?: number;
  speechProbabilityContinue?: number;
};

/**
 * Lightweight adaptive VAD used for responsive browser-side turn detection.
 * Moonshine remains authoritative for the transcript; this detector only
 * decides when an utterance should be submitted and when to interrupt output.
 */
export class AdaptiveVoiceActivityDetector {
  private readonly startFrames: number;
  private readonly endFrames: number;
  private readonly minimumThreshold: number;
  private readonly thresholdMultiplier: number;
  private readonly speechProbabilityStart: number;
  private readonly speechProbabilityContinue: number;
  private noiseFloor = 0.003;
  private voicedFrames = 0;
  private silentFrames = 0;
  private speaking = false;
  private calibrationFramesRemaining = 0;

  constructor(options: VoiceActivityOptions = {}) {
    this.startFrames = options.startFrames ?? 6;
    this.endFrames = options.endFrames ?? 28;
    this.minimumThreshold = options.minimumThreshold ?? 0.012;
    this.thresholdMultiplier = options.thresholdMultiplier ?? 3.2;
    this.speechProbabilityStart = options.speechProbabilityStart ?? 0.48;
    this.speechProbabilityContinue = options.speechProbabilityContinue ?? 0.36;
  }

  process(
    samples: Float32Array,
    hints: VoiceActivityHints = {},
  ): VoiceActivityUpdate {
    const level = rootMeanSquare(samples);
    if (
      typeof hints.noiseFloor === "number" &&
      Number.isFinite(hints.noiseFloor)
    ) {
      this.noiseFloor = Math.max(
        0.0001,
        this.noiseFloor * 0.85 + hints.noiseFloor * 0.15,
      );
    }
    const threshold = Math.max(
      this.minimumThreshold,
      this.noiseFloor * this.thresholdMultiplier,
    );
    const snrDb = Number.isFinite(hints.snrDb)
      ? Number(hints.snrDb)
      : signalToNoiseDb(level, this.noiseFloor);
    const speechProbability = clamp01(
      Number.isFinite(hints.speechProbability)
        ? Number(hints.speechProbability)
        : estimateSpeechProbability(samples, snrDb),
    );
    const probabilityThreshold = this.speaking
      ? this.speechProbabilityContinue
      : this.speechProbabilityStart;
    const voiced =
      level >= threshold && speechProbability >= probabilityThreshold;
    let event: VoiceActivityEvent = null;

    if (this.calibrationFramesRemaining > 0) {
      this.noiseFloor = this.noiseFloor * 0.82 + level * 0.18;
      this.calibrationFramesRemaining -= 1;
      this.voicedFrames = 0;
      this.silentFrames = 0;
      return {
        event,
        speaking: false,
        level,
        threshold,
        noiseFloor: this.noiseFloor,
        snrDb,
        speechProbability: 0,
        rejectedNoise: level >= threshold,
      };
    }

    if (this.speaking) {
      if (voiced) {
        this.silentFrames = 0;
      } else {
        this.silentFrames += 1;
        if (this.silentFrames >= this.endFrames) {
          this.speaking = false;
          this.voicedFrames = 0;
          this.silentFrames = 0;
          event = "speech-end";
        }
      }
    } else if (voiced) {
      this.voicedFrames += 1;
      if (this.voicedFrames >= this.startFrames) {
        this.speaking = true;
        this.silentFrames = 0;
        event = "speech-start";
      }
    } else {
      this.voicedFrames = 0;
      // Learn room noise only when the frame is unlikely to contain speech.
      if (speechProbability < this.speechProbabilityContinue) {
        this.noiseFloor = this.noiseFloor * 0.97 + level * 0.03;
      }
    }

    return {
      event,
      speaking: this.speaking,
      level,
      threshold,
      noiseFloor: this.noiseFloor,
      snrDb,
      speechProbability,
      rejectedNoise: !this.speaking && level >= threshold && !voiced,
    };
  }

  calibrate(frameCount = 75) {
    this.reset();
    this.calibrationFramesRemaining = Math.max(1, Math.round(frameCount));
  }

  get calibrationRemaining() {
    return this.calibrationFramesRemaining;
  }

  get ambientNoiseFloor() {
    return this.noiseFloor;
  }

  resetSpeechState() {
    this.voicedFrames = 0;
    this.silentFrames = 0;
    this.speaking = false;
  }

  reset() {
    this.noiseFloor = 0.003;
    this.resetSpeechState();
    this.calibrationFramesRemaining = 0;
  }
}

export function signalToNoiseDb(level: number, noiseFloor: number): number {
  return 20 * Math.log10((Math.max(0, level) + 1e-6) / (noiseFloor + 1e-6));
}

export function estimateSpeechProbability(
  samples: Float32Array,
  snrDb: number,
): number {
  if (!samples.length) return 0;
  const rms = rootMeanSquare(samples);
  if (rms < 0.0001) return 0;
  let peak = 0;
  let crossings = 0;
  let previous = samples[0];
  for (const sample of samples) {
    peak = Math.max(peak, Math.abs(sample));
    if ((sample >= 0 && previous < 0) || (sample < 0 && previous >= 0)) {
      crossings += 1;
    }
    previous = sample;
  }
  const zeroCrossingRate = crossings / samples.length;
  const crestFactor = peak / Math.max(rms, 1e-6);
  const snrScore = clamp01((snrDb - 2) / 16);
  const crossingScore =
    zeroCrossingRate >= 0.01 && zeroCrossingRate <= 0.32
      ? 1
      : zeroCrossingRate < 0.01
        ? zeroCrossingRate / 0.01
        : clamp01(1 - (zeroCrossingRate - 0.32) / 0.35);
  const crestScore = clamp01((12 - crestFactor) / 9);
  const transientPenalty =
    crestFactor >= 8 ? clamp01((16 - crestFactor) / 8) : 1;
  return clamp01(
    (snrScore * 0.62 + crossingScore * 0.2 + crestScore * 0.18) *
      transientPenalty,
  );
}

function clamp01(value: number): number {
  return Math.min(1, Math.max(0, value));
}

export function rootMeanSquare(samples: Float32Array): number {
  if (!samples.length) return 0;
  let sum = 0;
  for (const sample of samples) sum += sample * sample;
  return Math.sqrt(sum / samples.length);
}

export function mergeAudioFrames(frames: Float32Array[]): Float32Array {
  const length = frames.reduce((total, frame) => total + frame.length, 0);
  const merged = new Float32Array(length);
  let offset = 0;
  for (const frame of frames) {
    merged.set(frame, offset);
    offset += frame.length;
  }
  return merged;
}

export function resampleLinear(
  input: Float32Array,
  inputRate: number,
  outputRate: number,
): Float32Array {
  if (!input.length || inputRate <= 0 || outputRate <= 0) {
    return new Float32Array(0);
  }
  if (inputRate === outputRate) return new Float32Array(input);

  const outputLength = Math.max(
    1,
    Math.floor((input.length * outputRate) / inputRate),
  );
  const output = new Float32Array(outputLength);
  const ratio = inputRate / outputRate;
  for (let index = 0; index < outputLength; index += 1) {
    const position = index * ratio;
    const lower = Math.floor(position);
    const upper = Math.min(lower + 1, input.length - 1);
    const fraction = position - lower;
    output[index] = input[lower] * (1 - fraction) + input[upper] * fraction;
  }
  return output;
}

export function encodePCM16WAV(
  samples: Float32Array,
  sampleRate: number,
): Blob {
  const buffer = new ArrayBuffer(44 + samples.length * 2);
  const view = new DataView(buffer);
  writeASCII(view, 0, "RIFF");
  view.setUint32(4, 36 + samples.length * 2, true);
  writeASCII(view, 8, "WAVE");
  writeASCII(view, 12, "fmt ");
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true);
  view.setUint16(22, 1, true);
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, sampleRate * 2, true);
  view.setUint16(32, 2, true);
  view.setUint16(34, 16, true);
  writeASCII(view, 36, "data");
  view.setUint32(40, samples.length * 2, true);

  let offset = 44;
  for (const value of samples) {
    const sample = Math.max(-1, Math.min(1, value));
    view.setInt16(offset, sample < 0 ? sample * 0x8000 : sample * 0x7fff, true);
    offset += 2;
  }
  return new Blob([buffer], { type: "audio/wav" });
}

function writeASCII(view: DataView, offset: number, value: string) {
  for (let index = 0; index < value.length; index += 1) {
    view.setUint8(offset + index, value.charCodeAt(index));
  }
}
