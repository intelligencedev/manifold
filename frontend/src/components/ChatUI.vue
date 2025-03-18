<template>
    <div class="chat-container">
      <!-- Header (fixed height) -->
      <div class="chat-header">
        <div class="header-left">
          <div class="chat-title">LLM Chat</div>
          <div class="provider-section">
            <label>Provider:</label>
            <select v-model="provider">
              <option value="llama-server">llama-server</option>
              <option value="mlx_lm.server">mlx_lm.server</option>
              <option value="openai">openai</option>
            </select>
          </div>
        </div>
        <button @click="toggleSettings" class="settings-button">⚙️</button>
      </div>
  
      <!-- Main Content (flex:1) -->
      <div class="main-content">
        <!-- Scrollable Chat Messages -->
        <div class="chat-messages" ref="chatMessages">
          <div
            v-for="(msg, index) in messages"
            :key="index"
            :class="['message', msg.role]"
          >
            <div class="message-bubble" :style="{ fontSize: currentFontSize + 'px' }">
              <div
                v-if="msg.role === 'assistant' && selectedRenderMode === 'markdown'"
                v-html="formatMessage(msg.content)"
              ></div>
              <div v-else>{{ msg.content }}</div>
            </div>
          </div>
        </div>
  
        <!-- Chat Input -->
        <div class="chat-input">
          <textarea
            v-model="userInput"
            placeholder="Type a message..."
            @keyup.enter.exact.prevent="sendMessage"
          ></textarea>
          <button @click="sendMessage">Send</button>
        </div>
  
        <!-- Settings Panel (toggles on/off). 
             If the settings get large, they have internal scrolling. -->
        <div
          v-if="showSettings"
          class="chat-settings"
        >
          <details open>
            <summary>Parameters</summary>
            <!-- Endpoint -->
            <div class="input-field">
              <label>Endpoint:</label>
              <input type="text" v-model="endpoint" />
            </div>
            <!-- API Key -->
            <div class="input-field">
              <label>OpenAI API Key:</label>
              <input :type="showApiKey ? 'text' : 'password'" v-model="api_key" />
              <button @click="showApiKey = !showApiKey" class="toggle-password">
                <span v-if="showApiKey">👁️</span>
                <span v-else>🙈</span>
              </button>
            </div>
            <!-- Model -->
            <div class="input-field">
              <label>Model:</label>
              <input type="text" v-model="model" />
            </div>
            <!-- Max Completion Tokens -->
            <div class="input-field">
              <label>Max Completion Tokens:</label>
              <input type="number" v-model.number="max_completion_tokens" min="1" />
            </div>
            <!-- Temperature -->
            <div class="input-field">
              <label>Temperature:</label>
              <input type="number" v-model.number="temperature" step="0.1" min="0" max="2" />
            </div>
            <!-- Toggle Tool/Function Calls -->
            <div class="input-field">
              <label>
                <input type="checkbox" v-model="enableToolCalls" />
                Enable Tool/Function Calls
              </label>
            </div>
            <!-- Predefined System Prompt -->
            <div class="input-field">
              <label>Predefined System Prompt:</label>
              <select v-model="selectedSystemPrompt">
                <option
                  v-for="(prompt, key) in systemPromptOptions"
                  :key="key"
                  :value="key"
                >
                  {{ prompt.role }}
                </option>
              </select>
            </div>
            <!-- System Prompt -->
            <div class="input-field">
              <label>System Prompt:</label>
              <textarea v-model="system_prompt"></textarea>
            </div>
            <!-- Rendering Options -->
            <div class="input-field">
              <label>Render Mode:</label>
              <select v-model="selectedRenderMode">
                <option value="raw">Raw Text</option>
                <option value="markdown">Markdown</option>
              </select>
            </div>
            <div v-if="selectedRenderMode === 'markdown'" class="input-field">
              <label>Theme:</label>
              <select v-model="selectedTheme">
                <option value="atom-one-dark">Dark</option>
                <option value="atom-one-light">Light</option>
                <option value="github">GitHub</option>
                <option value="monokai">Monokai</option>
                <option value="vs">VS</option>
              </select>
            </div>
            <div class="input-field font-size-controls">
              <button @click="decreaseFontSize">-</button>
              <span>{{ currentFontSize }}px</span>
              <button @click="increaseFontSize">+</button>
            </div>
          </details>
        </div>
      </div>
    </div>
  </template>
  
  <script setup>
  import { ref, reactive, watch, nextTick, onMounted } from 'vue';
  import { marked } from 'marked';
  import hljs from 'highlight.js';
  
  // Conversation and input state
  const messages = ref([]);
  const userInput = ref('');
  const chatMessages = ref(null);
  
  // Settings visibility and API key toggle
  const showSettings = ref(false);
  const showApiKey = ref(false);
  
  // Request parameters
  const provider = ref('llama-server');
  const endpoint = ref('http://localhost:8080/v1/chat/completions');
  const api_key = ref('');
  const model = ref('local');
  const max_completion_tokens = ref(8192);
  const temperature = ref(0.6);
  const enableToolCalls = ref(false);
  
  // Predefined system prompts
  const selectedSystemPrompt = ref('friendly_assistant');
  const systemPromptOptions = {
    friendly_assistant: {
      role: 'Friendly Assistant',
      system_prompt: 'You are a helpful, friendly, and knowledgeable general-purpose AI assistant. Be concise when possible, but always clear and accurate.',
    },
    search_assistant: {
      role: 'Search Assistant',
      system_prompt: 'You are a helpful assistant that specializes in generating effective search engine queries. Provide concise and relevant search queries.',
    },
    research_analyst: {
      role: 'Research Analyst',
      system_prompt: 'You are a skilled research analyst with deep expertise in synthesizing information. Provide structured, clear, and evidence-backed responses.',
    },
    creative_writer: {
      role: 'Creative Writer',
      system_prompt: 'You are an exceptional creative writer. Use vivid sensory details and emotional resonance to craft your narratives.',
    },
    code_expert: {
      role: 'Programming Expert',
      system_prompt: 'You are a senior software developer with expertise across multiple languages. Present code solutions with clear comments and thorough explanations.',
    },
    teacher: {
      role: 'Educational Expert',
      system_prompt: 'You are an experienced teacher skilled at explaining complex concepts in a clear and progressive manner.',
    },
    data_analyst: {
      role: 'Data Analysis Expert',
      system_prompt: 'You are a data analysis expert. Provide clear insights, identify patterns, and explain statistical significance in your responses.',
    },
    retrieval_assistant: {
      role: 'Retrieval Assistant',
      system_prompt: 'You are capable of executing available functions. Always use the provided documents to support your responses.',
    },
    mcp_client: {
      role: 'MCP Client',
      system_prompt: 'Below is a list of file system, Git, and agent operations you can perform. Choose the best output to answer the user’s query.',
    },
  };
  
  const system_prompt = ref(systemPromptOptions[selectedSystemPrompt.value].system_prompt);
  watch(selectedSystemPrompt, (newKey) => {
    if (systemPromptOptions[newKey]) {
      system_prompt.value = systemPromptOptions[newKey].system_prompt;
    }
  });
  
  // Rendering options
  const selectedRenderMode = ref('markdown');
  const selectedTheme = ref('atom-one-dark');
  const currentFontSize = ref(12);
  
  // Format markdown using marked
  function formatMessage(content) {
    return marked(content);
  }
  
  // Load highlight.js theme dynamically
  let currentThemeLink = null;
  function loadTheme(themeName) {
    if (currentThemeLink) {
      document.head.removeChild(currentThemeLink);
    }
    currentThemeLink = document.createElement('link');
    currentThemeLink.rel = 'stylesheet';
    currentThemeLink.href = `https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.7.0/styles/${themeName}.min.css`;
    document.head.appendChild(currentThemeLink);
  }
  
  onMounted(() => {
    marked.setOptions({
      highlight: (code, lang) => {
        if (lang && hljs.getLanguage(lang)) {
          try {
            return hljs.highlight(code, { language: lang }).value;
          } catch (e) {
            console.error(e);
          }
        }
        try {
          return hljs.highlightAuto(code).value;
        } catch (e) {
          console.error(e);
        }
        return code;
      },
      breaks: false,
      gfm: true,
      headerIds: false,
    });
    loadTheme(selectedTheme.value);
  });
  
  watch(selectedTheme, (newTheme) => {
    loadTheme(newTheme);
  });
  
  // Auto-scroll to bottom when new messages arrive
  watch(messages, () => {
    nextTick(() => {
      if (chatMessages.value) {
        chatMessages.value.scrollTop = chatMessages.value.scrollHeight;
      }
    });
  });
  
  // Toggle settings panel
  function toggleSettings() {
    showSettings.value = !showSettings.value;
  }
  
  // Font size controls
  function increaseFontSize() {
    currentFontSize.value = Math.min(currentFontSize.value + 2, 24);
  }
  function decreaseFontSize() {
    currentFontSize.value = Math.max(currentFontSize.value - 2, 10);
  }
  
  // --- Streaming API calls ---
  async function streamCompletionsAPI_local(prompt, onToken) {
    const payload = {
      model: model.value,
      max_completion_tokens: max_completion_tokens.value,
      temperature: temperature.value,
      messages: [
        { role: 'system', content: system_prompt.value },
        { role: 'user', content: prompt },
      ],
      stream: true,
    };
  
    const res = await fetch(endpoint.value, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${api_key.value}`,
      },
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const errorText = await res.text();
      throw new Error(`API error (${res.status}): ${errorText}`);
    }
  
    const reader = res.body.getReader();
    const decoder = new TextDecoder('utf-8');
    let done = false;
    while (!done) {
      const { value, done: doneReading } = await reader.read();
      done = doneReading;
      const chunkValue = decoder.decode(value);
      const lines = chunkValue.split('\n').filter(line => line.trim() !== '');
      for (const line of lines) {
        if (line.startsWith('data: ')) {
          const data = line.slice(6).trim();
          if (data === '[DONE]') {
            done = true;
            break;
          }
          try {
            const parsed = JSON.parse(data);
            const token = parsed.choices[0]?.delta?.content || '';
            onToken(token);
          } catch (e) {
            console.error('Error parsing stream token', e);
          }
        }
      }
    }
  }
  
  async function streamCompletionsAPI_openai(prompt, onToken) {
    const payload = {
      model: model.value,
      max_completion_tokens: max_completion_tokens.value,
      temperature: temperature.value,
      messages: [
        { role: 'system', content: system_prompt.value },
        { role: 'user', content: prompt },
      ],
      stream: true,
    };
  
    const res = await fetch(endpoint.value, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${api_key.value}`,
      },
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const errorText = await res.text();
      throw new Error(`API error (${res.status}): ${errorText}`);
    }
  
    const reader = res.body.getReader();
    const decoder = new TextDecoder('utf-8');
    let done = false;
    while (!done) {
      const { value, done: doneReading } = await reader.read();
      done = doneReading;
      const chunkValue = decoder.decode(value);
      const lines = chunkValue.split('\n').filter(line => line.trim() !== '');
      for (const line of lines) {
        if (line.startsWith('data: ')) {
          const data = line.slice(6).trim();
          if (data === '[DONE]') {
            done = true;
            break;
          }
          try {
            const parsed = JSON.parse(data);
            const token = parsed.choices[0]?.delta?.content || '';
            onToken(token);
          } catch (e) {
            console.error('Error parsing stream token', e);
          }
        }
      }
    }
  }
  
  // Send message with streaming
  async function sendMessage() {
    if (!userInput.value.trim()) return;
    const userMsg = { role: 'user', content: userInput.value.trim() };
    messages.value.push(userMsg);
    const prompt = userInput.value.trim();
    userInput.value = '';
  
    // Reactive placeholder for assistant message
    const assistantMsg = reactive({ role: 'assistant', content: '' });
    messages.value.push(assistantMsg);
  
    try {
      if (provider.value === 'openai') {
        await streamCompletionsAPI_openai(prompt, token => {
          assistantMsg.content += token;
        });
      } else {
        await streamCompletionsAPI_local(prompt, token => {
          assistantMsg.content += token;
        });
      }
    } catch (error) {
      console.error(error);
      assistantMsg.content = 'Error fetching response.';
    }
  }
  </script>
  
  <style scoped>
  /* 
    The parent container is assumed to have some size, 
    and we fill it 100% without overflowing. 
  */
  .chat-container {
    display: flex;
    flex-direction: column;
    width: 100%;
    height: 90%;
    box-sizing: border-box;
    background-color: #1e1e1e;
    color: #fff;
    /* This ensures we do NOT scroll on the container itself. */
    overflow: hidden;
  }
  
  /* HEADER (Fixed height by content) */
  .chat-header {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 10px;
    background-color: #333;
  }
  
  .header-left {
    display: flex;
    align-items: center;
  }
  
  .chat-title {
    font-size: 18px;
    font-weight: bold;
    margin-right: 15px;
  }
  
  .provider-section {
    display: flex;
    align-items: center;
  }
  
  .provider-section label {
    margin-right: 5px;
  }
  
  .settings-button {
    background: none;
    border: none;
    color: #fff;
    font-size: 18px;
    cursor: pointer;
  }
  
  /* MAIN CONTENT area - grows to fill leftover space. */
  .main-content {
    flex: 1; 
    display: flex;
    flex-direction: column;
    /* no overflow on main-content so it doesn't produce parent scroll bars */
    overflow: hidden;
  }
  
  /* CHAT MESSAGES - scrollable if content grows large */
  .chat-messages {
    flex: 1;
    overflow-y: auto;
    padding: 10px;
    background-color: #2a2a2a;
  }
  
  .message {
    margin-bottom: 10px;
    display: flex;
  }
  .message.user {
    justify-content: flex-end;
  }
  .message.assistant {
    justify-content: flex-start;
  }
  
  .message-bubble {
    max-width: 70%;
    padding: 10px;
    border-radius: 10px;
    background-color: #444;
    word-wrap: break-word;
    text-align: left;
  }
  .message.user .message-bubble {
    background-color: #007aff;
    color: #fff;
  }
  
  /* CHAT INPUT (Fixed height by content) */
  .chat-input {
    flex-shrink: 0;
    display: flex;
    padding: 10px;
    background-color: #333;
  }
  .chat-input textarea {
    flex: 1;
    resize: none;
    padding: 8px;
    border-radius: 4px;
    border: none;
    font-size: 14px;
  }
  .chat-input button {
    margin-left: 10px;
    padding: 8px 12px;
    border: none;
    border-radius: 4px;
    background-color: #007aff;
    color: #fff;
    cursor: pointer;
  }
  
  /* SETTINGS (also fixed height by content, 
     can scroll internally if it's too tall for the leftover space) */
  .chat-settings {
    flex-shrink: 0;
    overflow-y: auto; /* This allows the settings to scroll internally if needed. */
    background-color: #2a2a2a;
    padding: 10px;
  }
  
  .input-field {
    margin-bottom: 10px;
  }
  
  .input-field label {
    display: block;
    margin-bottom: 4px;
  }
  
  .input-field input,
  .input-field select,
  .input-field textarea {
    width: 100%;
    padding: 6px;
    border: 1px solid #555;
    border-radius: 4px;
    background-color: #1e1e1e;
    color: #fff;
  }
  
  .toggle-password {
    background: none;
    border: none;
    cursor: pointer;
    margin-top: 5px;
  }
  
  /* Font Size Controls */
  .font-size-controls {
    display: flex;
    align-items: center;
    gap: 5px;
  }
  </style>
  