export type VoiceActivityEvent = "speech-start" | "speech-end" | null;

export type VoiceActivityUpdate = {
  event: VoiceActivityEvent;
  speaking: boolean;
  level: number;
  threshold: number;
};

export type VoiceActivityOptions = {
  startFrames?: number;
  endFrames?: number;
  minimumThreshold?: number;
  thresholdMultiplier?: number;
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
  private noiseFloor = 0.003;
  private voicedFrames = 0;
  private silentFrames = 0;
  private speaking = false;

  constructor(options: VoiceActivityOptions = {}) {
    this.startFrames = options.startFrames ?? 6;
    this.endFrames = options.endFrames ?? 28;
    this.minimumThreshold = options.minimumThreshold ?? 0.012;
    this.thresholdMultiplier = options.thresholdMultiplier ?? 3.2;
  }

  process(samples: Float32Array): VoiceActivityUpdate {
    const level = rootMeanSquare(samples);
    const threshold = Math.max(
      this.minimumThreshold,
      this.noiseFloor * this.thresholdMultiplier,
    );
    const voiced = level >= threshold;
    let event: VoiceActivityEvent = null;

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
      // Learn background noise slowly and only while no speech is active.
      this.noiseFloor = this.noiseFloor * 0.97 + level * 0.03;
    }

    return { event, speaking: this.speaking, level, threshold };
  }

  reset() {
    this.noiseFloor = 0.003;
    this.voicedFrames = 0;
    this.silentFrames = 0;
    this.speaking = false;
  }
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
