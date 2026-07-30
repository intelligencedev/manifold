class ManifoldRealtimeCaptureProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    this.frameLength = Math.max(128, Math.round(sampleRate * 0.02));
    this.pending = new Float32Array(this.frameLength);
    this.pendingLength = 0;
    this.mixBuffer = new Float32Array(128);
    this.noiseFloor = 0.003;
    this.sequence = 0;
    this.calibrationFrames = 15;
    this.lastCorrelation = 0;
    this.lastBeamformed = false;
    this.primaryChannel = 0;
    this.channelEnergy = [0, 0];
    this.port.onmessage = (event) => {
      if (event.data?.type === "calibrate") {
        this.noiseFloor = 0.003;
        this.calibrationFrames = Math.max(1, event.data.frames || 75);
      }
    };
  }

  process(inputs, outputs) {
    const startedAt = globalThis.performance?.now?.() || 0;
    const channels = inputs[0] || [];
    const input = this.mixInput(channels);
    if (input) {
      let inputOffset = 0;
      while (inputOffset < input.length) {
        const count = Math.min(
          input.length - inputOffset,
          this.frameLength - this.pendingLength,
        );
        this.pending.set(
          input.subarray(inputOffset, inputOffset + count),
          this.pendingLength,
        );
        inputOffset += count;
        this.pendingLength += count;
        if (this.pendingLength === this.frameLength) {
          const frame = this.pending;
          const metrics = this.analyzeFrame(frame);
          this.port.postMessage({
            type: "audio",
            samples: frame,
            sequence: this.sequence,
            channels: channels.length,
            beamformed: this.lastBeamformed,
            speechProbability: metrics.speechProbability,
            noiseFloor: metrics.noiseFloor,
            snrDb: metrics.snrDb,
            rejectedNoise: metrics.rejectedNoise,
            processingMs:
              (globalThis.performance?.now?.() || startedAt) - startedAt,
          });
          this.sequence += 1;
          this.pending.fill(0);
          this.pendingLength = 0;
        }
      }
    }

    // Keep a silent output connected so browsers continue pulling the graph.
    const output = outputs[0]?.[0];
    if (output) output.fill(0);
    return true;
  }

  mixInput(channels) {
    if (!channels.length || !channels[0]) return null;
    if (channels.length === 1) {
      this.lastBeamformed = false;
      return channels[0];
    }
    const length = channels[0].length;
    if (this.mixBuffer.length !== length) {
      this.mixBuffer = new Float32Array(length);
    }
    const first = channels[0];
    const second = channels[1];
    this.channelEnergy[0] =
      this.channelEnergy[0] * 0.82 + this.rms(first) * 0.18;
    this.channelEnergy[1] =
      this.channelEnergy[1] * 0.82 + this.rms(second) * 0.18;
    const alternateChannel = this.primaryChannel === 0 ? 1 : 0;
    if (
      this.channelEnergy[alternateChannel] >
      this.channelEnergy[this.primaryChannel] * 1.18
    ) {
      this.primaryChannel = alternateChannel;
    }
    const primary = channels[this.primaryChannel];
    const secondary = channels[this.primaryChannel === 0 ? 1 : 0];
    if (!secondary) return primary;

    const lag = this.bestLag(primary, secondary, 12);
    this.lastBeamformed = this.lastCorrelation >= 0.18;
    if (!this.lastBeamformed) return primary;
    for (let index = 0; index < length; index += 1) {
      const secondaryIndex = Math.min(length - 1, Math.max(0, index + lag));
      this.mixBuffer[index] =
        primary[index] * 0.72 + secondary[secondaryIndex] * 0.28;
    }
    return this.mixBuffer;
  }

  bestLag(primary, secondary, maxLag) {
    let bestLag = 0;
    let bestCorrelation = -1;
    const primaryEnergy = this.rms(primary) || 1e-6;
    const secondaryEnergy = this.rms(secondary) || 1e-6;
    for (let lag = -maxLag; lag <= maxLag; lag += 1) {
      let sum = 0;
      let count = 0;
      for (let index = 0; index < primary.length; index += 1) {
        const other = index + lag;
        if (other < 0 || other >= secondary.length) continue;
        sum += primary[index] * secondary[other];
        count += 1;
      }
      const correlation =
        count > 0 ? sum / count / (primaryEnergy * secondaryEnergy) : -1;
      if (correlation > bestCorrelation) {
        bestCorrelation = correlation;
        bestLag = lag;
      }
    }
    this.lastCorrelation = bestCorrelation;
    return bestLag;
  }

  analyzeFrame(frame) {
    const level = this.rms(frame);
    const snrDb =
      20 * Math.log10((Math.max(0, level) + 1e-6) / (this.noiseFloor + 1e-6));
    let peak = 0;
    let crossings = 0;
    let previous = frame[0] || 0;
    for (const sample of frame) {
      peak = Math.max(peak, Math.abs(sample));
      if ((sample >= 0 && previous < 0) || (sample < 0 && previous >= 0)) {
        crossings += 1;
      }
      previous = sample;
    }
    const crossingRate = crossings / Math.max(1, frame.length);
    const crestFactor = peak / Math.max(level, 1e-6);
    const snrScore = this.clamp((snrDb - 2) / 16);
    const crossingScore =
      crossingRate >= 0.01 && crossingRate <= 0.32
        ? 1
        : crossingRate < 0.01
          ? crossingRate / 0.01
          : this.clamp(1 - (crossingRate - 0.32) / 0.35);
    const crestScore = this.clamp((12 - crestFactor) / 9);
    const transientPenalty =
      crestFactor >= 8 ? this.clamp((16 - crestFactor) / 8) : 1;
    let speechProbability = this.clamp(
      (snrScore * 0.62 + crossingScore * 0.2 + crestScore * 0.18) *
        transientPenalty,
    );

    if (this.calibrationFrames > 0) {
      this.noiseFloor = this.noiseFloor * 0.82 + level * 0.18;
      this.calibrationFrames -= 1;
      speechProbability = 0;
    } else if (speechProbability < 0.36) {
      this.noiseFloor = this.noiseFloor * 0.97 + level * 0.03;
    }
    const threshold = Math.max(0.012, this.noiseFloor * 3.2);
    const rejectedNoise = level >= threshold && speechProbability < 0.48;
    return {
      speechProbability,
      noiseFloor: this.noiseFloor,
      snrDb,
      rejectedNoise,
    };
  }

  rms(samples) {
    if (!samples.length) return 0;
    let sum = 0;
    for (const sample of samples) sum += sample * sample;
    return Math.sqrt(sum / samples.length);
  }

  clamp(value) {
    return Math.max(0, Math.min(1, value));
  }
}

registerProcessor(
  "manifold-realtime-capture",
  ManifoldRealtimeCaptureProcessor,
);
