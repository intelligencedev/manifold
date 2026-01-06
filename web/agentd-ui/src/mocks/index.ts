import type { AgentRun, AgentStatus } from "@/api/client";

export const mockAgents: AgentStatus[] = [
  {
    id: "agentd-primary",
    name: "Primary Agent",
    state: "online",
    model: "gpt-5-large",
    updatedAt: new Date().toISOString(),
  },
  {
    id: "agentd-shadow",
    name: "Shadow Agent",
    state: "degraded",
    model: "gpt-4.1-mini",
    updatedAt: new Date(Date.now() - 1000 * 60 * 12).toISOString(),
  },
];

export const mockRuns: AgentRun[] = Array.from({ length: 5 }, (_, index) => ({
  id: `run-${index + 1}`,
  prompt: "Summarise the latest deployment health metrics",
  createdAt: new Date(Date.now() - index * 1_800_000).toISOString(),
  status: index === 0 ? "running" : "completed",
  tokens: 1200 - index * 150,
}));
