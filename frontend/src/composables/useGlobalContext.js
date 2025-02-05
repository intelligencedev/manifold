// /src/composables/useGlobalContext.js
import { reactive, readonly } from 'vue';

const context = reactive({
  systemInstructions: "You are a helpful assistant. Always follow the given guidelines and be concise.",
  taskContext: {
    taskId: "task-001",
    taskDescription: "Summarize the provided document and answer follow-up questions.",
    taskParameters: {
      summaryLength: "short",
      includeKeyPoints: true,
      additionalNotes: ""
    },
    taskStatus: {
      status: "in-progress", // "pending", "in-progress", or "complete"
      stage: "analysis",
      progress: 0.0,
      startTime: new Date().toISOString(),
      lastUpdated: new Date().toISOString()
    },
    assignedAssistants: {
      summarizationAssistant: "assistant-A",
      questionAnsweringAssistant: "assistant-B"
    }
  },
  conversationHistory: [
    // Entries are objects: { role, message, timestamp }
  ],
  pendingComputations: {
    // Example: comp_001: { description, status: { status, attempts, ... }, data: {} }
  },
  computedResults: {
    workingSummary: "",
    keyInsights: ""
  },
  finalOutput: "",
  metadata: {
    sessionId: "abc123",
    userId: "user-123",
    assistantIds: ["assistant-A", "assistant-B"],
    version: 1,
    lastUpdated: new Date().toISOString(),
    errorLogs: []
  }
});

function appendToConversationHistory(entry) {
  context.conversationHistory.push(entry);
}

function updateComputedResults(newResults) {
  Object.assign(context.computedResults, newResults);
}

function updateFinalOutput(output) {
  context.finalOutput = output;
}

function updatePendingComputation(id, update) {
  if (context.pendingComputations[id]) {
    Object.assign(context.pendingComputations[id], update);
  } else {
    context.pendingComputations[id] = update;
  }
}

function updateMetadata(newMetadata) {
  Object.assign(context.metadata, newMetadata);
}

export function useGlobalContext() {
  return {
    context: readonly(context),
    appendToConversationHistory,
    updateComputedResults,
    updateFinalOutput,
    updatePendingComputation,
    updateMetadata
  };
}
