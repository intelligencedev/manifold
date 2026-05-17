import { apiClient } from "./client";

export type ChatStreamEventType =
  | "thought_summary"
  | "delta"
  | "final"
  | "tool_start"
  | "tool_result"
  | "tts_chunk"
  | "tts_audio"
  | "image"
  | "error"
  | "summary"
  | "input_request"
  | "input_request_cancelled"
  | "agent_start"
  | "agent_delta"
  | "agent_final"
  | "agent_tool_start"
  | "agent_tool_result"
  | "agent_error"
  | "agent_thought_summary";

export interface ChatStreamEvent {
  type: ChatStreamEventType;
  data?: string;
  title?: string;
  tool_id?: string;
  args?: string;
  bytes?: number;
  b64?: string;
  url?: string;
  file_path?: string;
  data_url?: string;
  rel_path?: string;
  mime?: string;
  name?: string;
  agent?: string;
  model?: string;
  call_id?: string;
  parent_call_id?: string;
  depth?: number;
  role?: string;
  content?: string;
  error?: string;
  thought_summary?: string;
  request_id?: string;
  question?: string;
  reason?: string;
  choices?: Array<{ id?: string; label?: string; description?: string }>;
  allow_free_text?: boolean;
  multiple?: boolean;
  session_id?: string;
  run_id?: string;
  created_at?: string;
}

export interface AnswerBody {
  answer: string;
  choice_ids: string[];
}

export interface AnswerResult {
  ok: boolean;
  request_id: string;
}

export async function answerInputRequest(requestId: string, body: AnswerBody): Promise<AnswerResult> {
  const { data } = await apiClient.post<AnswerResult>(
    `/chat/input-requests/${encodeURIComponent(requestId)}/answer`,
    body
  );
  return data;
}
