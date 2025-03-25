<template>
    <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
        class="node-container openai-node tool-node" @mouseenter="isHovered = true" @mouseleave="isHovered = false">
        <div :style="data.labelStyle" class="node-label">Open AI / Local</div>

        <!-- Provider Selection -->
        <div class="input-field">
            <label for="provider-select" class="input-label">Provider:</label>
            <select id="provider-select" v-model="provider" class="input-select">
                <option value="llama-server">llama-server</option>
                <option value="mlx_lm.server">mlx_lm.server</option>
                <option value="openai">openai</option>
            </select>
        </div>

        <!-- Parameters Accordion -->
        <details class="parameters-accordion" open>
            <summary>Parameters</summary>

            <!-- Endpoint Input -->
            <div class="input-field">
                <label class="input-label">Endpoint:</label>
                <input type="text" class="input-text" v-model="endpoint" />
            </div>

            <!-- OpenAI API Key Input -->
            <div class="input-field">
                <label :for="`${data.id}-api_key`" class="input-label">OpenAI API Key:</label>
                <input :id="`${data.id}-api_key`" :type="showApiKey ? 'text' : 'password'" class="input-text"
                    v-model="api_key" />
                <button @click="showApiKey = !showApiKey" class="toggle-password">
                    <span v-if="showApiKey">👁️</span>
                    <span v-else>🙈</span>
                </button>
            </div>

            <!-- Model Selection -->
            <div class="input-field">
                <label :for="`${data.id}-model`" class="input-label">Model:</label>
                <input type="text" :id="`${data.id}-model`" v-model="model" class="input-text" />
            </div>

            <!-- Max Completion Tokens Input -->
            <div class="input-field">
                <label :for="`${data.id}-max_completion_tokens`" class="input-label">Max Completion Tokens:</label>
                <input type="number" :id="`${data.id}-max_completion_tokens`" v-model.number="max_completion_tokens"
                    class="input-text" min="1" />
            </div>

            <!-- Temperature Input -->
            <div class="input-field">
                <label :for="`${data.id}-temperature`" class="input-label">Temperature:</label>
                <input type="number" :id="`${data.id}-temperature`" v-model.number="temperature" class="input-text"
                    step="0.1" min="0" max="2" />
            </div>

            <!-- Toggle for Tool/Function Calling -->
            <div class="input-field">
                <label class="input-label">
                    <input type="checkbox" v-model="enableToolCalls" />
                    Enable Tool/Function Calls
                </label>
            </div>

            <!-- Predefined System Prompt Dropdown -->
            <div class="input-field">
                <label for="system-prompt-select" class="input-label">Predefined System Prompt:</label>
                <select id="system-prompt-select" v-model="selectedSystemPrompt" class="input-select">
                    <option v-for="(prompt, key) in systemPromptOptions" :key="key" :value="key">
                        {{ prompt.role }}
                    </option>
                </select>
            </div>

            <!-- System Prompt -->
            <div class="input-field">
                <label :for="`${data.id}-system_prompt`" class="input-label">System Prompt:</label>
                <textarea :id="`${data.id}-system_prompt`" v-model="system_prompt" class="input-textarea"></textarea>
            </div>
        </details>

        <!-- User Prompt -->
        <div class="input-field user-prompt-field">
            <label :for="`${data.id}-user_prompt`" class="input-label">User Prompt:</label>
            <textarea :id="`${data.id}-user_prompt`" v-model="user_prompt" class="input-textarea user-prompt-area"
                @mouseenter="handleTextareaMouseEnter" @mouseleave="handleTextareaMouseLeave"></textarea>
        </div>

        <!-- Input/Output Handles -->
        <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" />
        <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" />

        <!-- NodeResizer -->
        <NodeResizer :is-resizable="true" :color="'#666'" :handle-style="resizeHandleStyle"
            :line-style="resizeHandleStyle" :width="380" :height="760" :min-width="380" :min-height="760"
            :node-id="props.id" @resize="onResize" />
    </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'
import { useConfigStore } from '@/stores/configStore'
//import config from 'mermaid/dist/defaultConfig.js'

const props = defineProps({
    id: {
        type: String,
        required: true,
        default: 'Agent_0',
    },
    data: {
        type: Object,
        required: false,
        default: () => ({
            type: 'AgentNode',
            labelStyle: { fontWeight: 'normal' },
            hasInputs: true,
            hasOutputs: true,
            inputs: {
                endpoint: '',
                api_key: "",
                model: 'local',
                system_prompt: '',
                user_prompt: '',
                max_completion_tokens: 8192,
                temperature: 0.6,
            },
            outputs: { response: '' },
            models: ['local', 'chatgpt-4o-latest', 'gpt-4o', 'gpt-4o-mini', 'o1-mini', 'o1', 'o3-mini'],
            style: {
                border: '1px solid #666',
                borderRadius: '12px',
                backgroundColor: '#333',
                color: '#eee',
                width: '320px',
                height: '760px',
            },
        }),
    },
})

const configStore = useConfigStore()
const { getEdges, findNode, zoomIn, zoomOut } = useVueFlow()
const emit = defineEmits(['update:data', 'resize', 'disable-zoom', 'enable-zoom'])

const showApiKey = ref(false)
const enableToolCalls = ref(false)
const availableTools = ref([])

// Predefined System Prompt Options
const selectedSystemPrompt = ref("friendly_assistant")
const systemPromptOptions = {
    friendly_assistant: {
        role: "Friendly Assistant",
        system_prompt:
            "You are a helpful, friendly, and knowledgeable general-purpose AI assistant. You can answer questions, provide information, engage in conversation, and assist with a wide variety of tasks.  Be concise in your responses when possible, but prioritize clarity and accuracy.  If you don't know something, admit it.  Maintain a conversational and approachable tone."
    },
    search_assistant: {
        role: "Search Assistant",
        system_prompt:
            "You are a helpful assistant that specializes in generating effective search engine queries.  Given any text input, your task is to create one or more concise and relevant search queries that would be likely to retrieve information related to that text from a search engine (like Google, Bing, etc.).  Consider the key concepts, entities, and the user's likely intent.  Prioritize clarity and precision in the queries."
    },
    research_analyst: {
        role: "Research Analyst",
        system_prompt:
            "You are a skilled research analyst with deep expertise in synthesizing information. Approach queries by breaking down complex topics, organizing key points hierarchically, evaluating evidence quality, providing multiple perspectives, and using concrete examples. Present information in a structured format with clear sections, use bullet points for clarity, and visually separate different points with markdown. Always cite limitations of your knowledge and explicitly flag speculation."
    },
    creative_writer: {
        role: "Creative Writer",
        system_prompt:
            "You are an exceptional creative writer. When responding, use vivid sensory details, emotional resonance, and varied sentence structures. Organize your narratives with clear beginnings, middles, and ends. Employ literary techniques like metaphor and foreshadowing appropriately. When providing examples or stories, ensure they have depth and authenticity. Present creative options when asked, rather than single solutions."
    },
    code_expert: {
        role: "Programming Expert",
        system_prompt:
            "You are a senior software developer with expertise across multiple programming languages. Present code solutions with clear comments explaining your approach. Structure responses with: 1) Problem understanding 2) Solution approach 3) Complete, executable code 4) Explanation of how the code works 5) Alternative approaches. Include error handling in examples, use consistent formatting, and provide explicit context for any code snippets. Test your solutions mentally before presenting them."
    },
    teacher: {
        role: "Educational Expert",
        system_prompt:
            "You are an experienced teacher skilled at explaining complex concepts. Present information in a structured, progressive manner from foundational to advanced. Use analogies and examples to connect new concepts to familiar ones. Break down complex ideas into smaller components. Incorporate multiple formats (definitions, examples, diagrams described in text) to accommodate different learning styles. Ask thought-provoking questions to deepen understanding. Anticipate common misconceptions and address them proactively."
    },
    data_analyst: {
        role: "Data Analysis Expert",
        system_prompt:
            "You are a data analysis expert. When working with data, focus on identifying patterns and outliers, considering statistical significance, and exploring causal relationships vs. correlations. Present your analysis with a clear narrative structure that connects data points to insights. Use hypothetical data visualization descriptions when relevant. Consider alternative interpretations of data and potential confounding variables. Clearly communicate limitations and assumptions in any analysis."
    },
    retrieval_assistant: {
        role: "Retrieval Assistant",
        system_prompt: `You are capable of executing available function(s) and should always use them.
Always ask for the required input to: recipient==all.
Use JSON for function arguments.
Respond using the following format:
>>>\${recipient}
\${content}

Available functions:
namespace functions {
  // retrieves documents related to the topic
  type combined_retrieve = (_: {
    query: string,
    file_path_filter?: string,
    limit?: number,
    use_inverted_index?: boolean,
    use_vector_search?: boolean,
    merge_mode?: string,
    return_full_docs?: boolean,
    rerank?: boolean,
    alpha?: number,
    beta?: number
  }) => any;
  
  // remembers previous chats
  type agentic_retrieve = (_: {
    query: string,
    limit?: number
  }) => any;
}
`
    },
    recursive_agent: {
    role: "Recursive Agent",
    system_prompt: `Below is a list of file system, Git, and agent operations you can perform. Choose the best output to answer the user's query:

    - Required Fields:
    - "tool": The name of the tool you wish to execute. This can be one of:
        "agent".
    - "args": A JSON object containing the arguments required by the tool.
    - Payload Examples:
        { "action": "execute", "tool": "agent", "args": { "query": "Your query here", "maxCalls": 100 } }

    Important: You ONLY use the agent tool!

    You NEVER respond using Markdown. You ALWAYS respond using raw JSON choosing the best tool to answer the user's query.
    ALWAYS use the following raw JSON structure (for example for the time tool): { "action": "execute", "tool": "time", "args": {} }
    REMEMBER TO NEVER use markdown formatting and ONLY use raw JSON.`
    },
    tool_calling: {
    role: "Tool Caller",
    system_prompt: `Below is a list of tools and agent operations you can perform. Choose the best tool to answer the user's query. For example:

    - Required Fields:
    - "tool": The name of the tool you wish to execute. This can be one of:
        "agent".
    - "args": A JSON object containing the arguments required by the tool.
    - Payload Examples:
        { "action": "execute", "tool": "agent", "args": { "query": "Your query here", "maxCalls": 15 } }

    You NEVER respond using Markdown. You ALWAYS respond using raw JSON choosing the best tool to answer the user's query.
    ALWAYS use the following raw JSON structure (for example for the time tool): { "action": "execute", "tool": "time", "args": {} }
    REMEMBER TO NEVER use markdown formatting and ONLY use raw JSON.`
    },
}


// A helper function to check if a model is an O1/O3 variant.
function isO1Model(model) {
    const lower = model.toLowerCase();
    return lower.startsWith("o1") || lower.startsWith("o3");
}

// Set default run function on mount
onMounted(() => {
    if (!props.data.run) {
        props.data.run = run
    }
})

// Use config for default key & endpoint
watch(
    () => configStore.config,
    (newConfig) => {
        if (newConfig && newConfig.Completions) {
            if (!props.data.inputs.api_key) {
                props.data.inputs.api_key = newConfig.Completions.APIKey
            }
            if (!props.data.inputs.endpoint) {
                props.data.inputs.endpoint = newConfig.Completions.DefaultHost
            }
        }
    },
    { immediate: true }
)

const agenticRetrieveFunction = {
    name: "agentic_retrieve",
    description: "Gets memories from previous discussions to help remember things.",
    parameters: {
        type: "object",
        properties: {
            query: {
                type: "string",
                description: "The prompt or query to retrieve relevant memories."
            },
            limit: {
                type: "number",
                description: "The number of memories to retrieve.",
                default: 3
            }
        },
        required: ["query"]
    }
}

// ---------------------------
// Combined Retrieve Function
// ---------------------------
const combinedRetrieveFunction = {
    name: "combined_retrieve",
    description: "Retrieves documents using a combined search that uses both an inverted index and vector search.",
    parameters: {
        type: "object",
        properties: {
            query: {
                type: "string",
                description: "The prompt or query to retrieve relevant documents."
            },
            file_path_filter: {
                type: "string",
                description: "An optional filter on file path. Leave empty to search all files.",
                default: ""
            },
            limit: {
                type: "number",
                description: "The number of documents or chunks to retrieve.",
                default: 3
            },
            use_inverted_index: {
                type: "boolean",
                description: "Whether to use the inverted index.",
                default: true
            },
            use_vector_search: {
                type: "boolean",
                description: "Whether to use vector search.",
                default: true
            },
            merge_mode: {
                type: "string",
                description: "The merge mode to combine results. For example, 'weighted'.",
                default: "weighted"
            },
            return_full_docs: {
                type: "boolean",
                description: "Return full documents rather than text chunks.",
                default: true
            },
            rerank: {
                type: "boolean",
                description: "Whether to rerank the results.",
                default: true
            },
            alpha: {
                type: "number",
                description: "The vector weight (alpha) when merge_mode is weighted.",
                default: 0.5
            },
            beta: {
                type: "number",
                description: "The keyword weight (beta) when merge_mode is weighted.",
                default: 0.9
            }
        },
        required: ["query", "limit", "merge_mode"]
    }
}

// ---------------------------
// callCombinedRetrieveAPI
// ---------------------------
async function callCombinedRetrieveAPI(userPrompt) {
    const payload = {
        query: provider.value === 'openai' ? userPrompt : "retrieve: " + userPrompt,
        file_path_filter: "",
        limit: 3,
        use_inverted_index: true,
        use_vector_search: true,
        merge_mode: "weighted",
        return_full_docs: true,
        rerank: true,
        alpha: 0.5,
        beta: 0.9,
    };
    const retrieveEndpoint = "http://localhost:8080/api/sefii/combined-retrieve";
    try {
        const response = await fetch(retrieveEndpoint, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        });
        if (!response.ok) {
            const errorText = await response.text();
            throw new Error(`Retrieve API error (${response.status}): ${errorText}`);
        }
        return await response.json();
    } catch (error) {
        console.error("Error calling combined retrieve API:", error);
        throw error;
    }
}

async function callAgenticMemoryAPI(userPrompt) {
    const payload = {
        query: userPrompt,
        limit: 3
    };
    const retrieveEndpoint = "http://localhost:8080/api/agentic-memory/search";
    try {
        const response = await fetch(retrieveEndpoint, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        });
        if (!response.ok) {
            const errorText = await response.text();
            throw new Error(`Agentic Memory API error (${response.status}): ${errorText}`);
        }
        return await response.json();
    } catch (error) {
        console.error("Error calling agentic memory API:", error);
    }
}



// ---------------------------
// callCompletionsAPI_local
// ---------------------------
async function callCompletionsAPI_local(agentNode, prompt) {
    const responseNodeId = getEdges.value.find((e) => e.source === props.id)?.target;
    const responseNode = responseNodeId ? findNode(responseNodeId) : null;
    let endpoint = agentNode.data.inputs.endpoint;

    if (!enableToolCalls.value) {
        // Direct streaming request without tool/function call parameters.
        let body = {
            max_completion_tokens: agentNode.data.inputs.max_completion_tokens,
            temperature: agentNode.data.inputs.temperature,
            messages: [
            { role: "system", content: agentNode.data.inputs.system_prompt },
            { role: "user", content: prompt }
            ],
            stream: true
        };

        const streamResponse = await fetch(endpoint, {
            method: "POST",
            headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${agentNode.data.inputs.api_key}`,
            },
            body: JSON.stringify(body),
            // Add timeout of 5 minutes (300000 milliseconds)
            signal: AbortSignal.timeout(300000)
        });

        if (!streamResponse.ok) {
            const errorText = await streamResponse.text();
            throw new Error(`API error (${streamResponse.status}): ${errorText}`);
        }

        let buffer = "";
        const reader = streamResponse.body.getReader();
        while (true) {
            const { done, value } = await reader.read();
            if (done) break;
            const chunk = new TextDecoder().decode(value);
            buffer += chunk;
            let start = 0;
            for (let i = 0; i < buffer.length; i++) {
                if (buffer[i] === "\n") {
                    const line = buffer.substring(start, i).trim();
                    start = i + 1;
                    if (line.startsWith("data: ")) {
                        const jsonData = line.substring(6);
                        if (jsonData === "[DONE]") break;
                        try {
                            const parsedData = JSON.parse(jsonData);
                            const delta = parsedData.choices[0]?.delta || {};
                            const tokenContent = (delta.content || "") + (delta.thinking || "");
                            props.data.outputs.response += tokenContent;
                            if (responseNode) {
                                responseNode.data.inputs.response += tokenContent;
                                //responseNode.run();
                            }
                        } catch (e) {
                            console.error("Error parsing response chunk:", e);
                        }
                    }
                }
            }
            buffer = buffer.substring(start);
        }

        //await storeResponseInAgenticMemory(props.data.outputs.response);
        return { response: props.data.outputs.response };
    }

    // Existing two-step workflow with tool/function call enabled.
    let body = {
        model: agentNode.data.inputs.model,
        max_completion_tokens: agentNode.data.inputs.max_completion_tokens,
        temperature: agentNode.data.inputs.temperature,
        messages: [
            {
                role: "system",
                content: agentNode.data.inputs.system_prompt,
            },
            {
                role: "user",
                content: prompt,
            },
        ],
    };

    const responseData = await fetch(endpoint, {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${agentNode.data.inputs.api_key}`,
        },
        body: JSON.stringify(body),
    });

    const result = await responseData.json();
    const message = result.choices[0]?.message;

    if (message && message.tool_calls) {
        const toolCall = message.tool_calls[0];
        const functionName = toolCall?.function?.name;
        if (functionName === "combined_retrieve") {
            const retrieveResult = await callCombinedRetrieveAPI(prompt);
            const documents = retrieveResult.documents || {};
            let documentsString = '';
            if (typeof documents === 'object' && documents !== null) {
                documentsString = Object.entries(documents)
                    .map(([key, value]) => `${key}:\n\n${String(value)}`)
                    .join("\n\n");
            } else {
                documentsString = 'No valid documents found';
            }
            const combinedPrompt = `${prompt}\n\nREFERENCE:\n\n${documentsString}`;
            if (body.messages[1]) {
                body.messages[1].content = combinedPrompt;
            }
        }
        if (functionName === "agentic_retrieve") {
            try {
                const retrieveResult = await callAgenticMemoryAPI(prompt);
                if (retrieveResult && retrieveResult.results) {
                    const documents = retrieveResult.results || {};
                    console.log('documents:', documents);
                    const documentsString = buildReferenceString(documents);
                    console.log('documentsString:', documentsString);
                    const combinedPrompt = `${prompt}\n\nREFERENCE:\n\n${documentsString}`;
                    if (body.messages?.[1]) {
                        body.messages[1].content = combinedPrompt;
                    }
                }
            } catch (error) {
                console.warn("Error retrieving from agentic memory, continuing without retrieval:", error);
            }
        }
    }

    body.stream = true;
    delete body.functions;
    delete body.function_call;
    delete body.tool_calls;
    body.messages[0].content = "Use the provided documents to respond to the user's query. Be thorough and accurate and respond in a structured manner.";

    const streamResponse = await fetch(endpoint, {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${agentNode.data.inputs.api_key}`,
        },
        body: JSON.stringify(body),
    });

    if (!streamResponse.ok) {
        const errorText = await streamResponse.text();
        throw new Error(`API error (${streamResponse.status}): ${errorText}`);
    }

    let buffer = "";
    const reader = streamResponse.body.getReader();
    while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        const chunk = new TextDecoder().decode(value);
        buffer += chunk;
        let start = 0;
        for (let i = 0; i < buffer.length; i++) {
            if (buffer[i] === "\n") {
                const line = buffer.substring(start, i).trim();
                start = i + 1;
                if (line.startsWith("data: ")) {
                    const jsonData = line.substring(6);
                    if (jsonData === "[DONE]") break;
                    try {
                        const parsedData = JSON.parse(jsonData);
                        const delta = parsedData.choices[0]?.delta || {};
                        const tokenContent = (delta.content || "") + (delta.thinking || "");
                        props.data.outputs.response += tokenContent;
                        if (responseNode) {
                            responseNode.data.inputs.response += tokenContent;
                            responseNode.run();
                        }
                    } catch (e) {
                        console.error("Error parsing response chunk:", e);
                    }
                }
            }
        }
        buffer = buffer.substring(start);
    }

    // await storeResponseInAgenticMemory(props.data.outputs.response);
    return { response: props.data.outputs.response };
}

// ---------------------------
// callCompletionsAPI_openai - SIMPLIFIED
// ---------------------------
async function callCompletionsAPI_openai(agentNode, prompt) {
    console.log("callCompletionsAPI_openai called (should only happen if enableToolCalls is false)");
    const responseNodeId = getEdges.value.find((e) => e.source === props.id)?.target;
    const responseNode = responseNodeId ? findNode(responseNodeId) : null;
    let endpoint = agentNode.data.inputs.endpoint;

    // This function now ONLY handles the direct streaming case (no tools)
    let body = {};
    if (isO1Model(agentNode.data.inputs.model)) {
        body = {
            model: agentNode.data.inputs.model,
            max_tokens: agentNode.data.inputs.max_completion_tokens, // Use max_tokens for O1
            temperature: agentNode.data.inputs.temperature,
            messages: [
                { role: "user", content: `${agentNode.data.inputs.system_prompt}\n\n${prompt}` }
            ],
            // reasoning_effort: "high", // Optional for O1
            stream: true
        };
    } else {
        body = {
            model: agentNode.data.inputs.model,
            max_tokens: agentNode.data.inputs.max_completion_tokens, // Use max_tokens
            temperature: agentNode.data.inputs.temperature,
            messages: [
                { role: "system", content: agentNode.data.inputs.system_prompt },
                { role: "user", content: prompt }
            ],
            stream: true
        };
    }

    const streamResponse = await fetch(endpoint, {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${agentNode.data.inputs.api_key}`,
        },
        body: JSON.stringify(body),
    });

    if (!streamResponse.ok) {
        const errorText = await streamResponse.text();
        throw new Error(`API error (${streamResponse.status}): ${errorText}`);
    }

    // --- Streaming logic remains the same ---
    props.data.outputs.response = ''; // Clear previous response before streaming
    if (responseNode) {
        responseNode.data.inputs.response = '';
    }

    let buffer = "";
    const reader = streamResponse.body.getReader();
    while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        const chunk = new TextDecoder().decode(value);
        buffer += chunk;
        let start = 0;
        for (let i = 0; i < buffer.length; i++) {
            if (buffer[i] === "\n") {
                const line = buffer.substring(start, i).trim();
                start = i + 1;
                if (line.startsWith("data: ")) {
                    const jsonData = line.substring(6);
                    if (jsonData === "[DONE]") break;
                    try {
                        const parsedData = JSON.parse(jsonData);
                        const delta = parsedData.choices[0]?.delta || {};
                        // Adjust for potential differences in O1 vs standard response structure if needed
                        const tokenContent = delta.content || "";
                        props.data.outputs.response += tokenContent;
                        if (responseNode) {
                            // Avoid rapid updates if possible, maybe buffer slightly?
                            // For now, direct update:
                            responseNode.data.inputs.response += tokenContent;
                            // Consider if responseNode.run() is needed here or causes issues
                        }
                    } catch (e) {
                        console.error("Error parsing response chunk:", e, "Data:", jsonData);
                    }
                }
            }
        }
        buffer = buffer.substring(start);
    }
    // await storeResponseInAgenticMemory(props.data.outputs.response); // Consider if needed here
    return { response: props.data.outputs.response };
}

// Helper: Build a reference string from retrieved documents,
// excluding keys like 'embedding' and 'links'.
function buildReferenceString(documents) {
    let reference = "";
    Object.entries(documents).forEach(([key, value]) => {
        console.log('key:', key);
        console.log('value:', value);
        if (typeof value === 'object' && value !== null) {
            const filtered = Object.entries(value)
                .filter(([fieldKey]) => fieldKey !== 'embedding' && fieldKey !== 'links')
                .map(([fieldKey, fieldValue]) => `${fieldKey}: ${fieldValue}`)
                .join("\n");
            reference += `${key}:\n\n${filtered}\n\n`;
        } else {
            reference += `${key}:\n\n${String(value)}\n\n`;
        }
    });
    return reference.trim() || 'No valid documents found';
}

async function executeToolCalls(toolCalls) {
  const results = [];
  
  for (const call of toolCalls) {
    try {
      const response = await fetch("/v1/tool/execute", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          tool: call.tool,
          arguments: call.arguments
        }),
      });

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`Tool execution error (${response.status}): ${errorText}`);
      }

      const result = await response.json();
      results.push({
        tool: call.tool,
        output: result.output || JSON.stringify(result)
      });
    } catch (error) {
      console.error(`Error executing tool ${call.tool}:`, error);
      results.push({
        tool: call.tool,
        output: `Error: ${error.message}`,
        error: true
      });
    }
  }

  return results;
}

async function runInitialLLMCall(agentNode, prompt, toolEnabledSystemPrompt) {
  // Make initial call to get tool usage decisions
  const body = {
    model: agentNode.data.inputs.model,
    max_completion_tokens: agentNode.data.inputs.max_completion_tokens,
    temperature: agentNode.data.inputs.temperature,
    messages: [
      { role: "system", content: toolEnabledSystemPrompt },
      { role: "user", content: prompt }
    ],
    stream: false
  };

  const endpoint = agentNode.data.inputs.endpoint;
  const response = await fetch(endpoint, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${agentNode.data.inputs.api_key}`,
    },
    body: JSON.stringify(body)
  });

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`API error (${response.status}): ${errorText}`);
  }

  const result = await response.json();
  const content = result.choices?.[0]?.message?.content || "";
  console.log("Raw LLM content for tool decision:", content); // <-- ADD LOGGING

  try {
    let jsonString = content.trim();

    // Attempt to extract JSON from markdown code blocks
    const markdownMatch = jsonString.match(/```(?:json)?\s*([\s\S]*?)\s*```/);
    if (markdownMatch && markdownMatch[1]) {
      jsonString = markdownMatch[1].trim();
      console.log("Extracted JSON from markdown:", jsonString);
    } else {
       // Simple extraction: find first '[' or '{' and last ']' or '}'
       const firstBracket = jsonString.indexOf('[');
       const firstBrace = jsonString.indexOf('{');
       let start = -1;

       if (firstBracket !== -1 && (firstBrace === -1 || firstBracket < firstBrace)) {
           start = firstBracket;
       } else if (firstBrace !== -1) {
           start = firstBrace;
       }

       if (start !== -1) {
           const lastBracket = jsonString.lastIndexOf(']');
           const lastBrace = jsonString.lastIndexOf('}');
           let end = Math.max(lastBracket, lastBrace);
           if (end > start) {
               jsonString = jsonString.substring(start, end + 1);
               console.log("Attempting extraction between brackets/braces:", jsonString);
           }
       }
    }


    // Try parsing what we extracted/found
    const parsed = JSON.parse(jsonString);

    // Ensure it's an array
    if (Array.isArray(parsed)) {
      console.log("Parsed tool calls (array):", parsed);
      return parsed.filter(call => call.tool && call.arguments); // Basic validation
    } else if (typeof parsed === 'object' && parsed !== null && parsed.tool && parsed.arguments) {
      // If it's a single valid tool call object, wrap it in an array
      console.log("Parsed single tool call (object), wrapping in array:", [parsed]);
      return [parsed];
    }

    console.warn("Parsed JSON is not a valid tool call structure:", parsed);
    return []; // Not a valid tool call structure

  } catch (e) {
    console.warn("Failed to parse tool calls from LLM response content:", e);
    console.log("Content that failed parsing:", content);
    // If parsing fails, assume no valid tool calls
    return [];
  }
}

async function runFinalLLMCall(agentNode, prompt, toolResults, responseNode) {
  // Create a prompt that includes the tool results but instructs the model
  // not to mention that it used tools
  const finalSystemPrompt = 
    "Use these tool results as if you always knew the information. " +
    "Under no circumstances reveal that you used tools or mention tool details. " +
    "You must never say 'according to the tool output.' Just answer the user's query " +
    "with the combined knowledge below.";
  
  const toolResultsContext = toolResults.map(result => 
    `[${result.tool}]\n${result.output}`
  ).join("\n\n");
  
  const finalPrompt = `${prompt}\n\nKnowledge:\n${toolResultsContext}`;
  
  const body = {
    model: agentNode.data.inputs.model,
    max_completion_tokens: agentNode.data.inputs.max_completion_tokens,
    temperature: agentNode.data.inputs.temperature,
    messages: [
      { role: "system", content: finalSystemPrompt },
      { role: "user", content: finalPrompt }
    ],
    stream: true
  };

  const endpoint = agentNode.data.inputs.endpoint;
  const streamResponse = await fetch(endpoint, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${agentNode.data.inputs.api_key}`,
    },
    body: JSON.stringify(body),
  });

  if (!streamResponse.ok) {
    const errorText = await streamResponse.text();
    throw new Error(`API error (${streamResponse.status}): ${errorText}`);
  }

  // Reset response before streaming new content
  props.data.outputs.response = '';
  
  // Process streaming response
  let buffer = "";
  const reader = streamResponse.body.getReader();
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    const chunk = new TextDecoder().decode(value);
    buffer += chunk;
    let start = 0;
    for (let i = 0; i < buffer.length; i++) {
      if (buffer[i] === "\n") {
        const line = buffer.substring(start, i).trim();
        start = i + 1;
        if (line.startsWith("data: ")) {
          const jsonData = line.substring(6);
          if (jsonData === "[DONE]") break;
          try {
            const parsedData = JSON.parse(jsonData);
            const delta = parsedData.choices[0]?.delta || {};
            const tokenContent = (delta.content || "") + (delta.thinking || "");
            props.data.outputs.response += tokenContent;
            if (responseNode) {
              responseNode.data.inputs.response += tokenContent;
              //responseNode.run();
            }
          } catch (e) {
            console.error("Error parsing response chunk:", e);
          }
        }
      }
    }
    buffer = buffer.substring(start);
  }
  
  return { response: props.data.outputs.response };
}

// ---------------------------
// run() method
// ---------------------------
async function run() {
  console.log('Running AgentNode:', props.id);
  try {
    const agentNode = findNode(props.id);
    if (!agentNode) {
        throw new Error(`AgentNode with ID ${props.id} not found.`);
    }
    let finalPrompt = props.data.inputs.user_prompt || ""; // Ensure finalPrompt is initialized

    // Get input from connected source nodes, if any
    const connectedSources = getEdges.value
      .filter((edge) => edge.target === props.id)
      .map((edge) => edge.source);

    if (connectedSources.length > 0) {
      console.log(`Node ${props.id} has connected sources:`, connectedSources);
      for (const sourceId of connectedSources) {
        const sourceNode = findNode(sourceId);
        if (sourceNode && sourceNode.data && sourceNode.data.outputs) {
           // Adjust based on the actual output structure of source nodes
           // Assuming a common structure like sourceNode.data.outputs.response or sourceNode.data.outputs.result.output
           let sourceOutput = "";
           if (sourceNode.data.outputs.response) {
               sourceOutput = sourceNode.data.outputs.response;
           } else if (sourceNode.data.outputs.result && sourceNode.data.outputs.result.output) {
               sourceOutput = sourceNode.data.outputs.result.output;
           } else {
               console.warn(`Source node ${sourceId} has unexpected output structure:`, sourceNode.data.outputs);
           }

           if (sourceOutput) {
               finalPrompt += `\n\n--- Input from Node ${sourceId} ---\n${sourceOutput}`;
           }
        } else {
            console.warn(`Source node ${sourceId} not found or has no data/outputs.`);
        }
      }
      console.log(`Node ${props.id} final prompt after merging inputs:`, finalPrompt);
    }

    const responseNodeId = getEdges.value.find((e) => e.source === props.id)?.target;
    const responseNode = responseNodeId ? findNode(responseNodeId) : null;

    // Clear previous response in this node and the connected response node
    console.log(`Clearing previous responses for node ${props.id} and target ${responseNodeId || 'none'}`);
    props.data.outputs.response = '';
    if (responseNode && responseNode.data && responseNode.data.inputs) {
      responseNode.data.inputs.response = '';
    } else if (responseNodeId) {
        console.warn(`Response node ${responseNodeId} found but missing data.inputs`);
    }

    if (!enableToolCalls.value) {
      // Standard streaming call without tools - USES THE SIMPLIFIED FUNCTIONS
      console.log(`Node ${props.id}: Tool calls disabled, running direct stream...`);
      if (provider.value === 'openai') {
        // Ensure callCompletionsAPI_openai is simplified as per previous instructions
        return await callCompletionsAPI_openai(agentNode, finalPrompt);
      } else {
        // Ensure callCompletionsAPI_local handles its cases correctly
        return await callCompletionsAPI_local(agentNode, finalPrompt);
      }
    }

    // Tool-enabled flow
    console.log(`Node ${props.id}: Tool calls enabled, starting multi-step process...`);

    // 1. Create tool-enabled system prompt
    const toolEnabledSystemPrompt = buildToolEnabledSystemPrompt(agentNode.data.inputs.system_prompt);
    console.log(`Node ${props.id}: Tool-enabled system prompt:`, toolEnabledSystemPrompt);

    // 2. Make initial call to get tool decisions
    console.log(`Node ${props.id}: Calling runInitialLLMCall...`);
    const toolCalls = await runInitialLLMCall(agentNode, finalPrompt, toolEnabledSystemPrompt);
    console.log(`Node ${props.id}: Parsed tool calls from LLM:`, toolCalls);

    let toolResults = []; // Initialize empty results

    if (!toolCalls || toolCalls.length === 0) {
      console.log(`Node ${props.id}: No tool calls detected by LLM or parsing failed. Proceeding without tool execution.`);
      // Skip step 3 (executeToolCalls)
    } else {
      console.log(`Node ${props.id}: Executing tool calls:`, toolCalls);
      // 3. Execute the tools
      toolResults = await executeToolCalls(toolCalls);
      console.log(`Node ${props.id}: Tool execution results:`, toolResults);
    }

    // 4. Final streaming call (ALWAYS runs, with or without tool results)
    console.log(`Node ${props.id}: Running final LLM call...`);
    return await runFinalLLMCall(agentNode, finalPrompt, toolResults, responseNode);

  } catch (error) {
    console.error(`Error in AgentNode run (${props.id}):`, error);
    // Update this node's output with the error
    props.data.outputs.response = `Error: ${error.message}`;

    // Ensure the connected responseNode also shows the error if it exists
    const responseNodeId = getEdges.value.find((e) => e.source === props.id)?.target;
    const responseNode = responseNodeId ? findNode(responseNodeId) : null;
     if (responseNode && responseNode.data && responseNode.data.inputs) {
         responseNode.data.inputs.response = `Error: ${error.message}`;
     } else if (responseNodeId) {
         console.warn(`Response node ${responseNodeId} found but missing data.inputs during error propagation.`);
     }

    // Optionally re-throw or return an error state for the workflow runner
    return { error: error.message }; // Return error state
  }
}

async function storeResponseInAgenticMemory(responseText) {
    const ingestEndpoint = "http://localhost:8080/api/agentic-memory/ingest";
    const payload = {
        content: responseText,
        doc_title: "Agentic Response",
        completions_host: props.data.inputs.endpoint,
        completions_api_key: props.data.inputs.api_key,
        embeddings_host: configStore.config.Embeddings.Host,
        embeddings_api_key: configStore.config.Embeddings.APIKey
    };
    try {
        const res = await fetch(ingestEndpoint, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        });
        const jsonRes = await res.json();
        console.log("Stored response in agentic memory:", jsonRes);
    } catch (err) {
        console.error("Error storing response in agentic memory:", err);
    }
}

async function fetchToolList() {
  try {
    const host = configStore.config.Host;
    const port = configStore.config.Port;
    const url = `http://${host}:${port}/v1/tool/list`;
    const response = await fetch(url);
    if (!response.ok) {
      throw new Error(`Failed to fetch tool list. Status: ${response.status}`);
    }
    const data = await response.json();
    availableTools.value = data.tools || [];
  } catch (error) {
    console.error("Error fetching tool list:", error);
  }
}

// Build a tool-enabled system prompt by appending tool information to the base system prompt
function buildToolEnabledSystemPrompt(baseSystemPrompt) {
    if (!availableTools.value || availableTools.value.length === 0) {
        return baseSystemPrompt;
    }

    let toolPrompt = `${baseSystemPrompt}\n\nYou can call the following tools by outputting a JSON array. Each element should have:
{
  "tool": "<tool_name>",
  "arguments": { ... appropriate arguments ... }
}\n\nTools available:\n`;

    availableTools.value.forEach((tool, index) => {
        const exampleArgs = tool.inputSchema?.properties 
            ? Object.keys(tool.inputSchema.properties).reduce((acc, key) => {
                const prop = tool.inputSchema.properties[key];
                if (prop.type === "string") acc[key] = "example";
                else if (prop.type === "number") acc[key] = 1;
                else if (prop.type === "boolean") acc[key] = true;
                else acc[key] = null;
                return acc;
              }, {})
            : {};

        toolPrompt += `${index + 1}. ${tool.name} - ${tool.description}\n`;
        toolPrompt += `   Example usage: { "tool": "${tool.name}", "arguments": ${JSON.stringify(exampleArgs)} }\n`;
    });

    return toolPrompt;
}

// ---------------------------
// Computed Properties / Watches
// ---------------------------
const model = computed({
    get: () => props.data.inputs.model,
    set: (value) => { props.data.inputs.model = value },
})
const models = computed({
    get: () => props.data.models,
    set: (value) => { props.data.models = value },
})
const system_prompt = computed({
    get: () => props.data.inputs.system_prompt,
    set: (value) => { props.data.inputs.system_prompt = value },
})
const user_prompt = computed({
    get: () => props.data.inputs.user_prompt,
    set: (value) => { props.data.inputs.user_prompt = value },
})
const endpoint = computed({
    get: () => props.data.inputs.endpoint,
    set: (value) => { props.data.inputs.endpoint = value },
})
const api_key = computed({
    get: () => props.data.inputs.api_key,
    set: (value) => { props.data.inputs.api_key = value },
})
const max_completion_tokens = computed({
    get: () => props.data.inputs.max_completion_tokens,
    set: (value) => { props.data.inputs.max_completion_tokens = value },
})
const temperature = computed({
    get: () => props.data.inputs.temperature,
    set: (value) => { props.data.inputs.temperature = value },
})

// Provider detection
const provider = computed({
    get: () => {
        if (props.data.inputs.endpoint === 'https://api.openai.com/v1/chat/completions') {
            return 'openai';
        } else if (props.data.inputs.endpoint === configStore.config.Completions.DefaultHost) {
            if (configStore.config.Completions.Provider === 'llama-server') {
                return 'llama-server';
            } else if (configStore.config.Completions.Provider === 'mlx_lm.server') {
                return 'mlx_lm.server';
            }
        }
        return 'llama-server';
    },
    set: (value) => {
        if (value === 'openai') {
            props.data.inputs.endpoint = 'https://api.openai.com/v1/chat/completions';
        } else {
            props.data.inputs.endpoint = configStore.config.Completions.DefaultHost;
        }
    }
});

// Tool calling
watch(enableToolCalls, (newVal) => {
  if (newVal) {
    fetchToolList();

    console.log("Tool list fetched:", availableTools.value);
    if (availableTools.value.length > 0) {
      props.data.inputs.system_prompt = buildToolEnabledSystemPrompt(props.data.inputs.system_prompt);
    } else {
      props.data.inputs.system_prompt = props.data.inputs.system_prompt;
    }
  } else {
    availableTools.value = [];
  }
});

// If the store's provider changes, reset endpoint
watch(() => configStore.config.Completions.Provider, (newProvider) => {
    if (newProvider) {
        props.data.inputs.endpoint = configStore.config.Completions.DefaultHost;
    }
}, { immediate: true });

// Update system prompt when user picks a new predefined prompt
watch(selectedSystemPrompt, (newKey) => {
    if (systemPromptOptions[newKey]) {
        system_prompt.value = systemPromptOptions[newKey].system_prompt;
    }
}, { immediate: true });

const isHovered = ref(false)
const customStyle = ref({
    width: '320px',
    height: '760px'
})

const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
    width: '12px',
    height: '12px'
}))

function onResize(event) {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
}

// Manage zoom in/out while hovering over text area
const handleTextareaMouseEnter = () => {
    emit('disable-zoom')
    zoomIn(0)
    zoomOut(0)
}

const handleTextareaMouseLeave = () => {
    emit('enable-zoom')
    zoomIn(1)
    zoomOut(1)
}
</script>

<style scoped>
.openai-node {
    width: 100%;
    height: 100%;
    display: flex;
    flex-direction: column;
    box-sizing: border-box;
    background-color: var(--node-bg-color);
    border: 1px solid var(--node-border-color);
    border-radius: 4px;
    color: var(--node-text-color);
}

.node-label {
    color: var(--node-text-color);
    font-size: 16px;
    text-align: center;
    margin-bottom: 10px;
    font-weight: bold;
}

.input-field {
    position: relative;
}

.toggle-password {
    position: absolute;
    right: 10px;
    top: 50%;
    transform: translateY(-50%);
    background: none;
    border: none;
    padding: 0;
    cursor: pointer;
}

.input-text,
.input-select,
.input-textarea {
    margin-top: 5px;
    margin-bottom: 5px;
    background-color: #333;
    border: 1px solid #666;
    color: #eee;
    padding: 4px;
    font-size: 12px;
    width: calc(100% - 8px);
    box-sizing: border-box;
}

.user-prompt-field {
    height: 100%;
    flex: 1;
    display: flex;
    flex-direction: column;
}

.user-prompt-area {
    flex: 1;
    width: 100%;
    resize: none;
    overflow-y: auto;
    min-height: 0;
}

.parameters-accordion {
    min-width: 180px !important;
    margin-bottom: 5px;
    margin-top: 5px;
    border: 1px solid #666;
    border-radius: 4px;
    background-color: #444;
    color: #eee;
    padding: 5px;
    width: 100%;
    box-sizing: border-box;
}

.parameters-accordion summary {
    cursor: pointer;
    padding: 5px;
    font-weight: bold;
}
</style>
