# Summarization Engine

This document describes how Manifold's summarization engine manages conversation history to fit within LLM context windows while preserving important information.

## Overview

The summarization engine uses a **token-based approach** with a **reserve buffer pattern** to determine when conversation history needs to be compressed. This follows OpenAI's recommended pattern for managing context windows, particularly when working with reasoning models.

### Key Concepts

- **Context Window**: The maximum number of tokens the model can process (input + output)
- **Reserve Buffer**: Tokens reserved for model output, including reasoning tokens for reasoning models
- **Token Budget**: Available tokens for input = `context_window - reserve_buffer`
- **Preflight Token Counting**: Counting input tokens before sending to the model

## How It Works

### 1. Token Budget Calculation

```
token_budget = context_window - reserve_buffer
```

For example, with a 128K context window and 25K reserve buffer:
```
token_budget = 128,000 - 25,000 = 103,000 tokens available for input
```

### 2. Summarization Trigger

Before each LLM call, the engine:

1. Counts the current input tokens (messages + system prompt + tools)
2. Compares against the token budget
3. If over budget → summarizes older messages

### 3. Message Selection

When summarization is triggered:

1. **Preserve system message** (if present)
2. **Keep recent messages** (minimum `SummaryMinKeepLastMessages`, default 4)
3. **Summarize older messages** into a condensed summary

The engine works backwards from the most recent message, keeping as many messages as fit within roughly half the token budget (leaving room for system prompts, tools, and the summary itself).

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SUMMARY_ENABLED` | Enable/disable summarization | `false` |
| `SUMMARY_RESERVE_BUFFER_TOKENS` | Tokens reserved for output | `25000` |
| `SUMMARY_MIN_KEEP_LAST_MESSAGES` | Minimum recent messages to preserve | `4` |
| `SUMMARY_MAX_SUMMARY_CHUNK_TOKENS` | Max tokens per summary chunk | `4096` |

### YAML Configuration

```yaml
summaryEnabled: true
summaryReserveBufferTokens: 25000
summaryMinKeepLastMessages: 4
summaryMaxSummaryChunkTokens: 4096
```

### Reserve Buffer Sizing

The reserve buffer should account for:

- **Standard models**: 4,000-8,000 tokens typically sufficient
- **Reasoning models** (o1, o3, etc.): ~25,000 tokens recommended
  - `max_output_tokens` bounds visible output + hidden reasoning tokens
  - Reasoning tokens can be substantial and unpredictable

## Summarization Methods

### Standard Summary

Uses an LLM call to generate a condensed summary of older messages. The summary preserves:

- User goals and preferences
- Key decisions and facts
- Important identifiers (files, URLs, IDs)
- Tool results and errors
- Open questions

### OpenAI Responses API Compaction

When using OpenAI's Responses API (`api: responses`), the engine can use the native compaction endpoint which provides optimized context compression.

## Token Counting

### Provider Support

| Provider | Tokenization Method | Status |
|----------|---------------------|--------|
| OpenAI (Responses API) | `/v1/responses/input_tokens` endpoint | ✅ Implemented |
| OpenAI (Chat API) | tiktoken estimation | 🔄 Fallback to heuristic |
| Anthropic | `/v1/messages/count_tokens` endpoint | ✅ Implemented |
| Google | Gemini tokenization | 🔄 Fallback to heuristic |
| llama.cpp | Local tokenization | 🔄 Fallback to heuristic |

### Heuristic Fallback

When native tokenization is unavailable, the engine uses a heuristic:

```
estimated_tokens = len(content) / 4 + overhead
```

This provides a conservative estimate (tends to overcount slightly).

### OpenAI Responses API Tokenizer

The `ResponsesTokenizer` uses OpenAI's `/v1/responses/input_tokens` endpoint for accurate preflight token counting:

```go
// Example usage
tokenizer := client.Tokenizer(cache)
count, err := tokenizer.CountMessagesTokens(ctx, messages)
```

Features:
- Accurate token counts before API calls
- Supports all message types (text, tool calls, images)
- Results cached to reduce API calls

### Anthropic Messages API Tokenizer

The `MessagesTokenizer` uses Anthropic's `/v1/messages/count_tokens` endpoint for accurate preflight token counting:

```go
// Example usage
tokenizer := client.Tokenizer(cache)
count, err := tokenizer.CountMessagesTokens(ctx, messages)
```

Features:
- Accurate token counts matching Claude's actual tokenization
- Supports all message types (text, tool calls, tool results)
- System messages handled separately per Anthropic API conventions
- Results cached to reduce API calls
- Always available (no special API mode required)

## Architecture

### Engine Integration

```
┌─────────────────────────────────────────────────────────┐
│                      Engine.Run()                        │
├─────────────────────────────────────────────────────────┤
│  1. Build messages from history                          │
│  2. maybeSummarize(ctx, messages)                        │
│     ├─ Count input tokens                                │
│     ├─ Compare against token budget                      │
│     └─ If over → buildSummarizedMessages()               │
│  3. Send to LLM                                          │
└─────────────────────────────────────────────────────────┘
```

### Memory Manager Integration

The `memory.Manager` handles persistent chat sessions:

```
┌─────────────────────────────────────────────────────────┐
│                  Manager.BuildContext()                  │
├─────────────────────────────────────────────────────────┤
│  1. Load messages from persistence                       │
│  2. ensureSummary() - roll summary if needed             │
│  3. Select tail messages based on token budget           │
│  4. Prepend summary (if exists)                          │
│  5. Return combined history                              │
└─────────────────────────────────────────────────────────┘
```

## Best Practices

### 1. Size Reserve Buffer Appropriately

- For standard chat: 8,000-16,000 tokens
- For reasoning models: 25,000+ tokens
- For code generation: 16,000-32,000 tokens (longer outputs)

### 2. Monitor Token Usage

Enable logging to track summarization events:

```yaml
logLevel: info
```

Look for `summarization_triggered` log entries to understand when and why summarization occurs.

### 3. Tune Min Keep Messages

- Increase `SummaryMinKeepLastMessages` for conversations requiring more context
- Decrease for simple Q&A interactions

## Frontend Summary Component

The frontend provides visual feedback when conversation context is summarized, allowing users to understand when and why older messages are being compressed.

### Overview

The summary notification system consists of:

1. **Backend callback**: `OnSummaryTriggered` fired when summarization occurs
2. **SSE event stream**: Summary metadata sent to frontend
3. **Pinia store**: Reactive state management of summary events
4. **Vue component**: Visual indicator in the chat header

### Data Flow

```
┌────────────────────────────────────────────────────────┐
│  Backend: Engine.maybeSummarize()                      │
├────────────────────────────────────────────────────────┤
│  Determines summarization needed, invokes callback:    │
│  OnSummaryTriggered(inputTokens, tokenBudget,         │
│                      messageCount, summarizedCount)    │
└─────────────────────────┬────────────────────────────┘
                          │
                          │ SSE Event
                          ▼
┌────────────────────────────────────────────────────────┐
│  handlers_chat.go: OnSummaryTriggered callback         │
├────────────────────────────────────────────────────────┤
│  Emits SSE event with event type "summary":           │
│  {                                                     │
│    "type": "summary",                                  │
│    "input_tokens": 105000,                             │
│    "token_budget": 103000,                             │
│    "message_count": 42,                                │
│    "summarized_count": 38                              │
│  }                                                     │
└─────────────────────────┬────────────────────────────┘
                          │
                          │ HTTP SSE Stream
                          ▼
┌────────────────────────────────────────────────────────┐
│  Frontend: streamAgentRun()                            │
├────────────────────────────────────────────────────────┤
│  Parses SSE event and calls onEvent callback           │
└─────────────────────────┬────────────────────────────┘
                          │
                          │
                          ▼
┌────────────────────────────────────────────────────────┐
│  useChatStore: handleStreamEvent()                     │
├────────────────────────────────────────────────────────┤
│  case 'summary': Creates SummaryEvent object and      │
│  stores in summaryEventBySession reactive state        │
└─────────────────────────┬────────────────────────────┘
                          │
                          │ Pinia reactivity
                          ▼
┌────────────────────────────────────────────────────────┐
│  ChatView.vue: activeSummaryEvent computed             │
├────────────────────────────────────────────────────────┤
│  Displays summary indicator in chat header             │
│  Auto-dismisses after 8 seconds                        │
└────────────────────────────────────────────────────────┘
```

### Store Implementation

The chat store (`web/agentd-ui/src/stores/chat.ts`) manages summary events:

```typescript
// Summary event interface
export interface SummaryEvent {
  inputTokens: number;        // Tokens in current input
  tokenBudget: number;        // Token budget for input
  messageCount: number;       // Total messages before summary
  summarizedCount: number;    // Messages that were summarized
  timestamp: string;          // Event timestamp
}

// Reactive state per session
const summaryEventBySession = ref<Record<string, SummaryEvent | null>>({});

// Computed property for active session
const activeSummaryEvent = computed(() => 
  summaryEventBySession.value[activeSessionId.value] || null
);

// Clear summary event
function clearSummaryEvent(sessionId?: string) {
  const id = sessionId || activeSessionId.value;
  if (id) {
    summaryEventBySession.value = { 
      ...summaryEventBySession.value, 
      [id]: null 
    };
  }
}
```

### Visual Component

The summary indicator appears in the ChatView.vue header when summarization is triggered:

```vue
<span
  v-if="activeSummaryEvent"
  class="flex items-center gap-1.5 rounded-full bg-warning/10 
         dark:bg-warning/20 border border-warning/30 px-2.5 py-1 
         text-warning dark:text-warning-foreground transition-all duration-300"
  :title="`Summarized ${activeSummaryEvent.summarizedCount} of 
           ${activeSummaryEvent.messageCount} messages 
           (${activeSummaryEvent.inputTokens.toLocaleString()} tokens 
           exceeded ${activeSummaryEvent.tokenBudget.toLocaleString()} budget)`"
>
  <!-- Document icon -->
  <svg xmlns="http://www.w3.org/2000/svg" class="h-3 w-3" 
       viewBox="0 0 20 20" fill="currentColor">
    <path fill-rule="evenodd" 
          d="M4 4a2 2 0 012-2h4.586A2 2 0 0112 2.586L15.414 6A2 2 0 0116 7.414V16a2 2 0 01-2 2H6a2 2 0 01-2-2V4zm2 6a1 1 0 011-1h6a1 1 0 110 2H7a1 1 0 01-1-1zm1 3a1 1 0 100 2h6a1 1 0 100-2H7z" 
          clip-rule="evenodd" />
  </svg>
  
  <!-- Label -->
  <span class="font-medium">Context summarized</span>
  
  <!-- Dismiss button -->
  <button @click.stop="chat.clearSummaryEvent()">
    <svg class="h-3 w-3" viewBox="0 0 20 20" fill="currentColor">
      <!-- X icon -->
    </svg>
  </button>
</span>
```

**Styling Features:**

- **Warning color**: Uses `bg-warning/10` for subtle background and `text-warning` for text
- **Dark mode**: Automatically adjusts with `dark:bg-warning/20` and `dark:text-warning-foreground`
- **Hover states**: Dismiss button has `hover:bg-warning/20` feedback
- **Positioning**: Appears inline with streaming indicator in chat header
- **Persistence**: Visible for 8 seconds, then auto-dismisses via watcher
- **Interactive**: Users can manually dismiss with close button

### Auto-Dismiss Behavior

A Vue watcher automatically clears the summary event after 8 seconds:

```typescript
watch(activeSummaryEvent, (event) => {
  if (event) {
    setTimeout(() => {
      chat.clearSummaryEvent();
    }, 8000);
  }
});
```

This prevents visual clutter while still giving users time to read the notification.

### Event Types and Fields

The SSE stream includes a new `summary` event type:

```typescript
// From api/chat.ts
export type ChatStreamEventType =
  | "delta"
  | "final"
  | "tool_start"
  | "tool_result"
  | "summary"  // ← New event type
  | "image"
  | "error"
  // ... other types
```

Summary event fields:

```typescript
export interface ChatStreamEvent {
  type: "summary";
  input_tokens?: number;        // Current input token count
  token_budget?: number;        // Available token budget
  message_count?: number;       // Total message count before summary
  summarized_count?: number;    // Messages summarized away
  // ... other properties
}
```

### User Experience

1. **Summarization triggers** during model processing
2. **SSE event** is sent with summary metadata
3. **Indicator appears** in chat header showing:
   - Document icon
   - "Context summarized" label
   - Dismiss button
   - Tooltip with details (hover to see token counts)
4. **Auto-dismiss** after 8 seconds or user can click X

**Tooltip details example:**
```
Summarized 38 of 42 messages (105,000 tokens exceeded 103,000 budget)
```

This tells the user:
- How many messages were compressed (38 of 42)
- How much over budget the input was (105,000 vs 103,000 budget)

## Future Enhancements

- [ ] Anthropic native token counting API
- [ ] Google Gemini token counting API
- [ ] Local tokenizer support (tiktoken, sentencepiece)
- [ ] Adaptive reserve buffer based on model behavior
- [ ] Summary quality metrics and validation
- [ ] Configurable summary notification duration
- [ ] Summary event history in session metadata
- [ ] Summary compression ratio analytics
