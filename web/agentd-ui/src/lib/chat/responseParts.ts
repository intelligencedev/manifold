import type {
  ChatMessage,
  ChatResponseInputRequestPart,
  ChatResponsePart,
  ChatResponseToolPart,
} from "@/types/chat";

export function appendResponseText(
  parts: ChatResponsePart[] | undefined,
  content: string,
): ChatResponsePart[] {
  if (!content) return parts ? [...parts] : [];
  const next = parts ? [...parts] : [];
  const last = next[next.length - 1];
  if (last?.type === "text") {
    next[next.length - 1] = { ...last, content: last.content + content };
  } else {
    next.push({ id: `text-${next.length}`, type: "text", content });
  }
  return next;
}

export function rollbackResponseText(
  parts: ChatResponsePart[] | undefined,
  count: number,
): ChatResponsePart[] {
  const next = parts ? [...parts] : [];
  let remaining = Math.max(0, Math.floor(count));
  for (let index = next.length - 1; index >= 0 && remaining > 0; index -= 1) {
    const part = next[index];
    if (!part || part.type !== "text") continue;
    const removed = Math.min(part.content.length, remaining);
    const content = part.content.slice(0, part.content.length - removed);
    remaining -= removed;
    if (content) next[index] = { ...part, content };
    else next.splice(index, 1);
  }
  return next;
}

export function upsertResponseTool(
  parts: ChatResponsePart[] | undefined,
  tool: ChatResponseToolPart,
): ChatResponsePart[] {
  const next = parts ? [...parts] : [];
  const index = next.findIndex(
    (part) => part.type === "tool" && part.id === tool.id,
  );
  if (index < 0) next.push(tool);
  else {
    const existing = next[index] as ChatResponseToolPart;
    next[index] = {
      ...existing,
      ...tool,
      args: tool.args ?? existing.args,
      result: tool.result ?? existing.result,
    };
  }
  return next;
}

export function upsertResponseInputRequest(
  parts: ChatResponsePart[] | undefined,
  requestId: string,
): ChatResponsePart[] {
  const next = parts ? [...parts] : [];
  const id = `input-request-${requestId}`;
  const index = next.findIndex(
    (part) => part.type === "input_request" && part.requestId === requestId,
  );
  const requestPart: ChatResponseInputRequestPart = {
    id,
    type: "input_request",
    requestId,
  };
  if (index < 0) next.push(requestPart);
  else next[index] = requestPart;
  return next;
}

export function reconcileResponseText(
  parts: ChatResponsePart[] | undefined,
  content: string,
): ChatResponsePart[] {
  if (!parts?.length) return content ? appendResponseText([], content) : [];
  const current = responseText(parts);
  if (current === content) return [...parts];
  if (content.startsWith(current)) {
    return appendResponseText(parts, content.slice(current.length));
  }
  if (current.startsWith(content)) {
    return rollbackResponseText(parts, current.length - content.length);
  }

  const next = parts.map((part) => ({ ...part }));
  const textIndexes = next.flatMap((part, index) =>
    part.type === "text" ? [index] : [],
  );
  if (!textIndexes.length) return appendResponseText(next, content);

  let offset = 0;
  textIndexes.forEach((partIndex, textIndex) => {
    const part = next[partIndex];
    if (!part || part.type !== "text") return;
    const isLast = textIndex === textIndexes.length - 1;
    const length = isLast ? content.length - offset : part.content.length;
    next[partIndex] = {
      ...part,
      content: content.slice(offset, offset + Math.max(0, length)),
    };
    offset += Math.max(0, length);
  });
  return next.filter((part) => part.type !== "text" || part.content);
}

export function responsePartsForMessage(message: ChatMessage) {
  let parts: ChatResponsePart[] = message.responseParts?.length
    ? [...message.responseParts]
    : message.content
      ? [
          {
            id: `${message.id}-text`,
            type: "text" as const,
            content: message.content,
          },
        ]
      : [];
  for (const request of message.inputRequests || []) {
    parts = upsertResponseInputRequest(parts, request.id);
  }
  return parts;
}

function responseText(parts: ChatResponsePart[]) {
  return parts
    .filter((part) => part.type === "text")
    .map((part) => part.content)
    .join("");
}
