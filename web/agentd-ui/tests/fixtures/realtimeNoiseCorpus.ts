export const corpusSampleRate = 48_000;
export const corpusFrameLength = 960;

export type LabeledAudioFrame = {
  samples: Float32Array;
  speech: boolean;
};

export type RealtimeNoiseFixture = {
  name: string;
  calibration: Float32Array[];
  frames: LabeledAudioFrame[];
  expectedTurns: number;
};

export function buildRealtimeNoiseCorpus(): RealtimeNoiseFixture[] {
  return [
    {
      name: "quiet room",
      calibration: repeatFrames(quietFrame, 15),
      frames: [
        ...labeledFrames(quietFrame, 20, false),
        ...labeledFrames(closeSpeechFrame, 28, true),
        ...labeledFrames(quietFrame, 36, false),
      ],
      expectedTurns: 1,
    },
    {
      name: "fan and close speech",
      calibration: repeatFrames(fanFrame, 15),
      frames: [
        ...labeledFrames(fanFrame, 30, false),
        ...labeledFrames(speechOverFanFrame, 30, true),
        ...labeledFrames(fanFrame, 36, false),
      ],
      expectedTurns: 1,
    },
    {
      name: "keyboard impulses",
      calibration: repeatFrames(quietFrame, 15),
      frames: labeledFrames(keyboardFrame, 120, false),
      expectedTurns: 0,
    },
    {
      name: "distant television",
      calibration: repeatFrames(quietFrame, 15),
      frames: labeledFrames(distantTelevisionFrame, 120, false),
      expectedTurns: 0,
    },
    {
      name: "background music",
      calibration: repeatFrames(quietFrame, 15),
      frames: labeledFrames(backgroundMusicFrame, 120, false),
      expectedTurns: 0,
    },
    {
      name: "residual TTS speaker echo",
      calibration: repeatFrames(quietFrame, 15),
      frames: labeledFrames(residualTtsEchoFrame, 120, false),
      expectedTurns: 0,
    },
    {
      name: "near speaker over distant speaker",
      calibration: repeatFrames(distantSpeakerFrame, 15),
      frames: [
        ...labeledFrames(distantSpeakerFrame, 30, false),
        ...labeledFrames(nearAndDistantSpeakersFrame, 30, true),
        ...labeledFrames(distantSpeakerFrame, 36, false),
      ],
      expectedTurns: 1,
    },
  ];
}

export function quietFrame(frameIndex = 0): Float32Array {
  return toneFrame(frameIndex, 0.002, 73, 0.0006);
}

export function fanFrame(frameIndex = 0): Float32Array {
  const frame = toneFrame(frameIndex, 0.014, 91, 0.002);
  addTone(frame, frameIndex, 0.005, 182);
  return frame;
}

export function closeSpeechFrame(frameIndex = 0): Float32Array {
  const envelope = 0.78 + 0.22 * Math.sin(frameIndex * 0.47);
  const frame = toneFrame(frameIndex, 0.075 * envelope, 178, 0.003);
  addTone(frame, frameIndex, 0.026 * envelope, 356);
  addTone(frame, frameIndex, 0.012 * envelope, 712);
  return frame;
}

export function speechOverFanFrame(frameIndex = 0): Float32Array {
  const frame = fanFrame(frameIndex);
  addInto(frame, closeSpeechFrame(frameIndex));
  return frame;
}

export function keyboardFrame(frameIndex = 0): Float32Array {
  const frame = quietFrame(frameIndex);
  if (frameIndex % 5 !== 0) return frame;
  const first = (frameIndex * 37 + 101) % (corpusFrameLength - 80);
  frame[first] += 0.95;
  frame[first + 51] += 0.72;
  return frame;
}

export function distantTelevisionFrame(frameIndex = 0): Float32Array {
  const frame = toneFrame(frameIndex, 0.007, 143, 0.002);
  addTone(frame, frameIndex, 0.004, 277);
  return frame;
}

export function backgroundMusicFrame(frameIndex = 0): Float32Array {
  const frame = toneFrame(frameIndex, 0.004, 220, 0.001);
  addTone(frame, frameIndex, 0.003, 330);
  addTone(frame, frameIndex, 0.002, 440);
  return frame;
}

export function residualTtsEchoFrame(frameIndex = 0): Float32Array {
  const frame = toneFrame(frameIndex, 0.006, 165, 0.001);
  addTone(frame, frameIndex, 0.0025, 330);
  return frame;
}

export function distantSpeakerFrame(frameIndex = 0): Float32Array {
  const frame = toneFrame(frameIndex, 0.014, 132, 0.0015);
  addTone(frame, frameIndex, 0.006, 264);
  return frame;
}

export function nearAndDistantSpeakersFrame(frameIndex = 0): Float32Array {
  const frame = distantSpeakerFrame(frameIndex);
  addInto(frame, closeSpeechFrame(frameIndex));
  return frame;
}

function repeatFrames(
  factory: (frameIndex: number) => Float32Array,
  count: number,
): Float32Array[] {
  return Array.from({ length: count }, (_, index) => factory(index));
}

function labeledFrames(
  factory: (frameIndex: number) => Float32Array,
  count: number,
  speech: boolean,
): LabeledAudioFrame[] {
  return Array.from({ length: count }, (_, index) => ({
    samples: factory(index),
    speech,
  }));
}

function toneFrame(
  frameIndex: number,
  amplitude: number,
  frequency: number,
  noiseAmplitude: number,
): Float32Array {
  const frame = new Float32Array(corpusFrameLength);
  const offset = frameIndex * corpusFrameLength;
  let randomState = (frameIndex + 1) * 0x9e3779b1;
  for (let index = 0; index < frame.length; index += 1) {
    randomState = (Math.imul(randomState, 1_664_525) + 1_013_904_223) >>> 0;
    const noise = (randomState / 0xffff_ffff) * 2 - 1;
    frame[index] =
      Math.sin(
        ((offset + index) / corpusSampleRate) * Math.PI * 2 * frequency,
      ) *
        amplitude +
      noise * noiseAmplitude;
  }
  return frame;
}

function addTone(
  frame: Float32Array,
  frameIndex: number,
  amplitude: number,
  frequency: number,
) {
  const offset = frameIndex * corpusFrameLength;
  for (let index = 0; index < frame.length; index += 1) {
    frame[index] +=
      Math.sin(
        ((offset + index) / corpusSampleRate) * Math.PI * 2 * frequency,
      ) * amplitude;
  }
}

function addInto(target: Float32Array, source: Float32Array) {
  for (let index = 0; index < target.length; index += 1) {
    target[index] += source[index];
  }
}
