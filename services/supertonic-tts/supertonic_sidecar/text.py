"""Sentence-level text chunking for streaming synthesis.

Ports the chunking behaviour of the browser reference (``vendor/helper.mjs``
``chunkText``) so the sidecar streams one synthesized segment per sentence-group.
"""

from __future__ import annotations

import re

# Abbreviations whose trailing period must not be treated as a sentence end.
_ABBREVIATIONS = {
    "Mr.", "Mrs.", "Ms.", "Dr.", "Prof.", "Sr.", "Jr.", "Ph.D.", "etc.",
    "e.g.", "i.e.", "vs.", "Inc.", "Ltd.", "Co.", "Corp.", "St.", "Ave.",
    "Blvd.",
}

_PARAGRAPH_RE = re.compile(r"\n\s*\n+")
_BOUNDARY_RE = re.compile(r"[.!?]+\s+")
_INITIAL_RE = re.compile(r"[A-Z]\.")


def _is_abbreviation(token: str) -> bool:
    return token in _ABBREVIATIONS or _INITIAL_RE.fullmatch(token) is not None


def _sentences(paragraph: str) -> list[str]:
    sentences: list[str] = []
    start = 0
    for m in _BOUNDARY_RE.finditer(paragraph):
        punct = re.match(r"[.!?]+", m.group()).group()
        end = m.start() + len(punct)
        token_start = paragraph.rfind(" ", start, m.start())
        token_start = start if token_start == -1 else token_start + 1
        token = paragraph[token_start:m.start() + 1].strip()
        if _is_abbreviation(token):
            continue
        piece = paragraph[start:end].strip()
        if piece:
            sentences.append(piece)
        start = m.end()
    tail = paragraph[start:].strip()
    if tail:
        sentences.append(tail)
    return sentences


def split_sentences(text: str, max_len: int = 300) -> list[str]:
    """Split ``text`` into sentence-grouped chunks no longer than ``max_len``.

    A single sentence longer than ``max_len`` is emitted as its own (oversized)
    chunk rather than being cut mid-sentence.
    """
    chunks: list[str] = []
    for paragraph in _PARAGRAPH_RE.split(text.strip()):
        paragraph = paragraph.strip()
        if not paragraph:
            continue
        current = ""
        for sentence in _sentences(paragraph):
            if len(current) + len(sentence) + 1 <= max_len:
                current = f"{current} {sentence}" if current else sentence
            else:
                if current:
                    chunks.append(current.strip())
                current = sentence
        if current:
            chunks.append(current.strip())
    return chunks
