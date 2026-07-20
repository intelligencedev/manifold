class ManifoldRealtimeCaptureProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    this.frameLength = Math.max(128, Math.round(sampleRate * 0.02));
    this.pending = new Float32Array(this.frameLength);
    this.pendingLength = 0;
  }

  process(inputs, outputs) {
    const input = inputs[0]?.[0];
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
          this.port.postMessage({ type: "audio", samples: frame }, [
            frame.buffer,
          ]);
          this.pending = new Float32Array(this.frameLength);
          this.pendingLength = 0;
        }
      }
    }

    // Keep a silent output connected so browsers continue pulling the graph.
    const output = outputs[0]?.[0];
    if (output) output.fill(0);
    return true;
  }
}

registerProcessor(
  "manifold-realtime-capture",
  ManifoldRealtimeCaptureProcessor,
);
