const LEADING_SEPARATOR_RE = /^[\s.,;:!?-]+/;
const MENTION_BOUNDARY_RE = /[\s.,;:!?]/;

export interface SpecialistMentionResolution {
  specialist: string | null;
  prompt: string;
}

function startsWithIgnoreCase(value: string, prefix: string): boolean {
  if (value.length < prefix.length) return false;
  return value.slice(0, prefix.length).toLowerCase() === prefix.toLowerCase();
}

function normalizeCandidates(candidates: string[]): string[] {
  const seen = new Set<string>();
  const normalized: string[] = [];
  for (const raw of candidates) {
    const name = (raw || "").trim();
    if (!name) continue;
    const key = name.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    normalized.push(name);
  }
  // Prefer longest names first to avoid partial collisions.
  normalized.sort((a, b) => b.length - a.length);
  return normalized;
}

export function resolveLeadingSpecialistMention(
  text: string,
  candidates: string[],
): SpecialistMentionResolution {
  const input = (text || "").trim();
  if (!input || !candidates.length) {
    return { specialist: null, prompt: input };
  }

  for (const candidate of normalizeCandidates(candidates)) {
    const mention = `@${candidate}`;
    if (!startsWithIgnoreCase(input, mention)) continue;

    const nextChar = input.slice(mention.length, mention.length + 1);
    if (nextChar && !MENTION_BOUNDARY_RE.test(nextChar)) continue;

    const prompt = input.slice(mention.length).replace(LEADING_SEPARATOR_RE, "");
    return { specialist: candidate, prompt };
  }

  return { specialist: null, prompt: input };
}

export function stripLeadingSpecialistMention(
  text: string,
  specialist?: string,
): string {
  const name = (specialist || "").trim();
  if (!name) return (text || "").trim();
  return resolveLeadingSpecialistMention(text, [name]).prompt;
}
