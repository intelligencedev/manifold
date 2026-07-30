import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import ApproveCommand from "@/components/chat/ApproveCommand.vue";

const message = {
  id: "assistant-1",
  role: "assistant" as const,
  content: "",
  createdAt: "2026-07-28T00:00:00.000Z",
};

function request(id = "approval-1") {
  return {
    id,
    question: "Approve command execution: go test ./...",
    choices: [
      { id: "approve_once", label: "Approve once" },
      { id: "allow_all_session", label: "Allow all in this session" },
      { id: "deny", label: "Deny" },
    ],
    allowFreeText: true,
    multiple: false,
    status: "pending" as const,
    createdAt: "2026-07-28T00:00:00.000Z",
  };
}

function mountApproval(count = 1) {
  const toggleInputRequestChoice = vi.fn();
  const submitInputRequest = vi.fn();
  const items = Array.from({ length: count }, (_, index) => ({
    message,
    request: request(`approval-${index + 1}`),
  }));
  const wrapper = mount(ApproveCommand, {
    props: {
      items,
      model: {
        agentNameFor: () => "Orchestrator",
        inputRequestFieldName: () => "approval",
        inputRequestChoiceSelected: () => false,
        toggleInputRequestChoice,
        isInputRequestSubmitting: () => false,
        inputRequestDraft: () => "",
        setInputRequestDraft: vi.fn(),
        inputRequestLocalError: () => "",
        canSubmitInputRequest: () => true,
        submitInputRequest,
      } as never,
    },
  });
  return { wrapper, toggleInputRequestChoice, submitInputRequest };
}

describe("ApproveCommand", () => {
  it("shows one queued command and calls out session-wide permission", () => {
    const { wrapper } = mountApproval(2);

    expect(wrapper.text()).toContain("go test ./...");
    expect(wrapper.text()).toContain("1 / 2");
    expect(wrapper.findAll(".command-approval__choice")).toHaveLength(3);
    expect(wrapper.findAll(".command-approval__choice--danger")).toHaveLength(
      1,
    );
  });

  it("supports deliberate number selection and Enter submission", () => {
    const { toggleInputRequestChoice, submitInputRequest } = mountApproval();

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "2" }));
    expect(toggleInputRequestChoice).toHaveBeenCalledWith(
      message,
      expect.objectContaining({ id: "approval-1" }),
      "allow_all_session",
    );

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter" }));
    expect(submitInputRequest).toHaveBeenCalledOnce();
  });

  it("does not map Escape to denial", () => {
    const { toggleInputRequestChoice, submitInputRequest } = mountApproval();

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    expect(toggleInputRequestChoice).not.toHaveBeenCalled();
    expect(submitInputRequest).not.toHaveBeenCalled();
  });
});
