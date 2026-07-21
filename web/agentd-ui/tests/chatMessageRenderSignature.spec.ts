import { describe, expect, it } from "vitest";
import { chatMessageRenderSignature } from "@/composables/chat/useChatViewController";
import type { ChatMessage } from "@/types/chat";

describe("chatMessageRenderSignature", () => {
  it("changes when non-text response components are added or updated", () => {
    const message: ChatMessage = {
      id: "assistant-1",
      role: "assistant",
      content: "Waiting for approval.",
      createdAt: "2026-07-20T00:00:00.000Z",
      streaming: true,
    };
    const initial = chatMessageRenderSignature(message);
    const withRequest: ChatMessage = {
      ...message,
      inputRequests: [
        {
          id: "approval-1",
          question: "Allow the command?",
          choices: [],
          allowFreeText: true,
          multiple: false,
          status: "pending",
          createdAt: "2026-07-20T00:00:01.000Z",
        },
      ],
      responseParts: [
        {
          id: "input-request-approval-1",
          type: "input_request",
          requestId: "approval-1",
        },
      ],
    };
    const requestSignature = chatMessageRenderSignature(withRequest);

    expect(requestSignature).not.toBe(initial);
    expect(
      chatMessageRenderSignature({
        ...withRequest,
        inputRequests: [
          { ...withRequest.inputRequests![0], status: "answered" },
        ],
      }),
    ).not.toBe(requestSignature);
    expect(
      chatMessageRenderSignature({
        ...withRequest,
        attachments: [
          {
            id: "image-1",
            kind: "image",
            name: "result.png",
            previewUrl: "/result.png",
          },
        ],
      }),
    ).not.toBe(requestSignature);
  });
});
