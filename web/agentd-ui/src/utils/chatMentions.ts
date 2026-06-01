const LEADING_SEPARATOR_RE = /^[\s.,;:!?-]+/;
const MENTION_BOUNDARY_RE = /[\s.,;:!?]/;

export interface SpecialistMentionResolution {
  specialist: string | null;
  prompt: string;
}

export type ChatMentionTargetKind = "specialist" | "team";

export interface ChatMentionTarget {
  kind: ChatMentionTargetKind;
  name: string;
}

export interface ChatMentionTargetResolution {
  kind: ChatMentionTargetKind | null;
  name: string | null;
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

function normalizeTargetCandidates(
  candidates: ChatMentionTarget[],
): ChatMentionTarget[] {
  const seen = new Set<string>();
  const normalized: ChatMentionTarget[] = [];
  for (const raw of candidates) {
    const name = (raw?.name || "").trim();
    const kind = raw?.kind;
    if (!name || (kind !== "specialist" && kind !== "team")) continue;
    const key = `${kind}:${name.toLowerCase()}`;
    if (seen.has(key)) continue;
    seen.add(key);
    normalized.push({ kind, name });
  }
  // Prefer longest names first to avoid partial collisions; for exact
  // specialist/team collisions, teams win.
  normalized.sort((a, b) => {
    const lengthDiff = b.name.length - a.name.length;
    if (lengthDiff !== 0) return lengthDiff;
    if (a.kind === b.kind) return 0;
    return a.kind === "team" ? -1 : 1;
  });
  return normalized;
}

export function resolveLeadingChatMention(
  text: string,
  candidates: ChatMentionTarget[],
): ChatMentionTargetResolution {
  const input = (text || "").trim();
  if (!input || !candidates.length) {
    return { kind: null, name: null, prompt: input };
  }

  for (const candidate of normalizeTargetCandidates(candidates)) {
    const mention = `@${candidate.name}`;
    if (!startsWithIgnoreCase(input, mention)) continue;

    const nextChar = input.slice(mention.length, mention.length + 1);
    if (nextChar && !MENTION_BOUNDARY_RE.test(nextChar)) continue;

    const prompt = input.slice(mention.length).replace(LEADING_SEPARATOR_RE, "");
    return { kind: candidate.kind, name: candidate.name, prompt };
  }

  return { kind: null, name: null, prompt: input };
}

export function resolveLeadingSpecialistMention(
  text: string,
  candidates: string[],
): SpecialistMentionResolution {
  const resolved = resolveLeadingChatMention(
    text,
    normalizeCandidates(candidates).map((name) => ({
      kind: "specialist",
      name,
    })),
  );
  return {
    specialist: resolved.kind === "specialist" ? resolved.name : null,
    prompt: resolved.prompt,
  };
}

export function stripLeadingChatMention(text: string, name?: string): string {
  const targetName = (name || "").trim();
  if (!targetName) return (text || "").trim();
  return resolveLeadingChatMention(text, [
    { kind: "team", name: targetName },
    { kind: "specialist", name: targetName },
  ]).prompt;
}

export function stripLeadingSpecialistMention(
  text: string,
  specialist?: string,
): string {
  return stripLeadingChatMention(text, specialist);
}
