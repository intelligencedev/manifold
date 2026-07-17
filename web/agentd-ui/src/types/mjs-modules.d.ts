declare module "*.mjs" {
  export const AVAILABLE_LANGS: string[];
  export function isValidLang(lang: string): boolean;

  export class UnicodeProcessor {
    constructor(indexer: number[]);
    call(
      textList: string[],
      langList: string[],
    ): { textIds: number[][]; textMask: number[][][] };
  }

  export class Style {
    ttl: unknown;
    dp: unknown;
    constructor(ttlTensor: unknown, dpTensor: unknown);
  }

  export class TextToSpeech {
    sampleRate: number;
    constructor(
      cfgs: unknown,
      textProcessor: unknown,
      dpOrt: unknown,
      textEncOrt: unknown,
      vectorEstOrt: unknown,
      vocoderOrt: unknown,
    );
    call(
      text: string,
      lang: string,
      style: Style,
      totalStep: number,
      speed?: number,
      silenceDuration?: number,
      progressCallback?: ((step: number, total: number) => void) | null,
    ): Promise<{ wav: number[]; duration: number[] }>;
  }
}
