// @ts-nocheck
import { serverEngine as supertonicEngine } from "./serverEngine";
import type { SupertonicTtsSettings } from "./settings";

type StreamerOptions = {
  getSettings: () => SupertonicTtsSettings;
  isEnabledForSession: (sessionId: string) => boolean;
};

type ActiveStream = {
  sessionId: string;
  messageId: string;
  buffer: string;
  spokenLength: number;
  finalized: boolean;
  queue: string[];
  abort: AbortController;
  running: boolean;
  generation: number;
};

/**
 * Consume assistant text deltas and speak sentence-sized chunks sequentially.
 */
export class SupertonicStreamer {
  private streams = new Map<string, ActiveStream>();
  private audioCtx: AudioContext | null = null;
  private playHead = 0;
  private scheduled: AudioBufferSourceNode[] = [];
  private options: StreamerOptions;
  private generation = 0;

  constructor(options: StreamerOptions) {
    this.options = options;
  }

  begin(sessionId: string, messageId: string) {
    this.stop(sessionId);
    const stream: ActiveStream = {
      sessionId,
      messageId,
      buffer: "",
      spokenLength: 0,
      finalized: false,
      queue: [],
      abort: new AbortController(),
      running: false,
      generation: ++this.generation,
    };
    this.streams.set(sessionId, stream);
  }

  pushDelta(sessionId: string, messageId: string, delta: string) {
    if (!delta) return;
    if (!this.options.isEnabledForSession(sessionId)) return;
    let stream = this.streams.get(sessionId);
    if (!stream || stream.messageId !== messageId) {
      this.begin(sessionId, messageId);
      stream = this.streams.get(sessionId)!;
    }
    stream.buffer += delta;
    this.enqueueReady(stream, false);
    void this.pump(stream);
  }

  finalize(sessionId: string, messageId: string, fullText?: string) {
    if (!this.options.isEnabledForSession(sessionId)) return;
    let stream = this.streams.get(sessionId);
    if (!stream || stream.messageId !== messageId) {
      this.begin(sessionId, messageId);
      stream = this.streams.get(sessionId)!;
      if (fullText) stream.buffer = fullText;
    } else if (fullText && fullText.length >= stream.buffer.length) {
      stream.buffer = fullText;
    }
    stream.finalized = true;
    this.enqueueReady(stream, true);
    void this.pump(stream);
  }

  /**
   * Discard the last `chars` characters of buffered text for a message that
   * the harness rejected. Trims the buffer, clamps the spoken cursor, and drops
   * any not-yet-synthesized chunks derived from the rejected text. Audio that
   * already started playing cannot be recalled — but in the chat (guarded_chat)
   * flow the spoken final answer is never rejected, so this is a rare cleanup.
   */
  rollback(sessionId: string, messageId: string, chars: number) {
    if (chars <= 0) return;
    const stream = this.streams.get(sessionId);
    if (!stream || stream.messageId !== messageId) return;
    const newLength = Math.max(0, stream.buffer.length - chars);
    stream.buffer = stream.buffer.slice(0, newLength);
    stream.spokenLength = Math.min(stream.spokenLength, stream.buffer.length);
    // Queued chunks were extracted from the (now-trimmed) tail; drop the ones
    // that have not been synthesized yet so the rejected text is not spoken.
    stream.queue = [];
  }

  stop(sessionId?: string) {
    if (sessionId) {
      const stream = this.streams.get(sessionId);
      if (stream) {
        stream.abort.abort();
        this.streams.delete(sessionId);
      }
    } else {
      for (const stream of this.streams.values()) stream.abort.abort();
      this.streams.clear();
    }
    for (const source of this.scheduled) {
      try {
        source.stop();
      } catch {
        // already stopped
      }
    }
    this.scheduled = [];
  }

  private enqueueReady(stream: ActiveStream, forceTail: boolean) {
    const remaining = stream.buffer.slice(stream.spokenLength);
    if (!remaining.trim()) return;

    if (forceTail) {
      const text = remaining.trim();
      if (text) {
        stream.queue.push(text);
        stream.spokenLength = stream.buffer.length;
      }
      return;
    }

    // Prefer completed sentences; fall back to long clause chunks.
    const sentenceMatch = remaining.match(/^([\s\S]{12,}?[.!?…])(?:\s+|$)/u);
    if (sentenceMatch) {
      const chunk = sentenceMatch[1].trim();
      if (chunk) stream.queue.push(chunk);
      stream.spokenLength += sentenceMatch[0].length;
      // Keep draining if more complete sentences are already buffered.
      this.enqueueReady(stream, false);
      return;
    }

    if (remaining.length >= 220) {
      const cut = remaining.lastIndexOf(" ", 200);
      const end = cut > 40 ? cut : 200;
      const chunk = remaining.slice(0, end).trim();
      if (chunk) stream.queue.push(chunk);
      stream.spokenLength += end;
      this.enqueueReady(stream, false);
    }
  }

  private async pump(stream: ActiveStream) {
    if (stream.running) return;
    stream.running = true;
    try {
      while (!stream.abort.signal.aborted) {
        const next = stream.queue.shift();
        if (!next) {
          if (stream.finalized) break;
          break;
        }
        await this.speakChunk(stream, next);
      }
    } finally {
      stream.running = false;
      // If more work arrived while speaking, continue.
      if (
        !stream.abort.signal.aborted &&
        (stream.queue.length > 0 ||
          (!stream.finalized &&
            stream.buffer.length - stream.spokenLength >= 12))
      ) {
        this.enqueueReady(stream, stream.finalized);
        if (stream.queue.length) void this.pump(stream);
      } else if (stream.finalized && stream.queue.length === 0) {
        // leave stream entry until next message starts
      }
    }
  }

  private async speakChunk(stream: ActiveStream, text: string) {
    const settings = this.options.getSettings();
    await supertonicEngine.ensureReady({
      customVoices: settings.customVoices,
    });
    if (stream.abort.signal.aborted) return;

    await supertonicEngine.synthesizeStream(
      {
        text,
        lang: settings.language,
        voiceId: settings.voiceId,
        totalSteps: settings.totalSteps,
        speed: settings.speed,
        signal: stream.abort.signal,
      },
      async (samples, sampleRate) => {
        if (stream.abort.signal.aborted || !samples.length) return;
        await this.enqueueAudio(samples, sampleRate, stream.abort.signal);
      },
    );
  }

  private async ensureAudioContext(): Promise<AudioContext> {
    if (!this.audioCtx) {
      const Ctx =
        window.AudioContext ||
        (window as typeof window & { webkitAudioContext?: typeof AudioContext })
          .webkitAudioContext;
      if (!Ctx) throw new Error("Web Audio API is unavailable");
      this.audioCtx = new Ctx();
      this.playHead = this.audioCtx.currentTime;
    }
    if (this.audioCtx.state === "suspended") {
      await this.audioCtx.resume();
    }
    return this.audioCtx;
  }

  private async enqueueAudio(
    samples: Float32Array,
    sampleRate: number,
    signal: AbortSignal,
  ) {
    const ctx = await this.ensureAudioContext();
    if (signal.aborted) return;

    const buffer = ctx.createBuffer(1, samples.length, sampleRate);
    const channel = new Float32Array(samples);
    buffer.copyToChannel(channel, 0);

    const source = ctx.createBufferSource();
    source.buffer = buffer;
    source.connect(ctx.destination);

    const startAt = Math.max(ctx.currentTime + 0.02, this.playHead);
    source.start(startAt);
    this.playHead = startAt + buffer.duration;
    this.scheduled.push(source);
    source.onended = () => {
      this.scheduled = this.scheduled.filter((s) => s !== source);
    };
  }
}
