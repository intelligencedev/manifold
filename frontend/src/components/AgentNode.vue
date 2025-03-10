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
                system_prompt: 'You are a helpful assistant.',
                user_prompt: 'Write a haiku about manifolds.',
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
    mcp_client: {
    role: "MCP Client",
    system_prompt: `Below is a list of file system, Git, and agent operations you can perform. Choose the best output to answer the user's query:

    1. listTools
    - Purpose: Returns a list of all registered file system, Git, and agent tools.
    - Payload Example:
    { "action": "listTools" }

    2. execute
    - Purpose: Executes a specific operation.
    - Required Fields:
    - "tool": The name of the tool you wish to execute. This can be one of:
        "agent".
    - "args": A JSON object containing the arguments required by the tool.
    - Payload Examples:
        { "action": "execute", "tool": "agent", "args": { "query": "Your query here", "maxCalls": 5 } }

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

const mcpServerFunctions = {
    "tools": [
        {
            "description": "Performs basic mathematical operations",
            "inputSchema": {
                "$schema": "https://json-schema.org/draft/2020-12/schema",
                "properties": {
                    "a": {
                        "description": "First number",
                        "type": "number"
                    },
                    "b": {
                        "description": "Second number",
                        "type": "number"
                    },
                    "operation": {
                        "description": "The mathematical operation to perform",
                        "enum": [
                            "add",
                            "subtract",
                            "multiply",
                            "divide"
                        ],
                        "type": "string"
                    }
                },
                "required": [
                    "operation",
                    "a",
                    "b"
                ],
                "type": "object"
            },
            "name": "calculate"
        },
        {
            "description": "Says hello to the provided name",
            "inputSchema": {
                "$schema": "https://json-schema.org/draft/2020-12/schema",
                "properties": {
                    "name": {
                        "description": "The name to say hello to",
                        "type": "string"
                    }
                },
                "required": [
                    "name"
                ],
                "type": "object"
            },
            "name": "hello"
        },
        {
            "description": "Returns the current time",
            "inputSchema": {
                "$schema": "https://json-schema.org/draft/2020-12/schema",
                "properties": {
                    "format": {
                        "description": "Optional time format (default: RFC3339)",
                        "type": "string"
                    }
                },
                "type": "object"
            },
            "name": "time"
        }
    ]
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
            model: agentNode.data.inputs.model,
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
        functions: [combinedRetrieveFunction, agenticRetrieveFunction],
        function_call: { name: "agentic_retrieve" }
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
// callCompletionsAPI_openai
// ---------------------------
async function callCompletionsAPI_openai(agentNode, prompt) {
    const responseNodeId = getEdges.value.find((e) => e.source === props.id)?.target;
    const responseNode = responseNodeId ? findNode(responseNodeId) : null;
    let endpoint = agentNode.data.inputs.endpoint;

    if (!enableToolCalls.value) {
        let body = {};
        if (isO1Model(agentNode.data.inputs.model)) {
            body = {
                model: agentNode.data.inputs.model,
                max_completion_tokens: agentNode.data.inputs.max_completion_tokens,
                temperature: agentNode.data.inputs.temperature,
                messages: [
                    { role: "user", content: `${agentNode.data.inputs.system_prompt}\n\n${prompt}` }
                ],
                reasoning_effort: "high",
                stream: true
            };
        } else {
            body = {
                model: agentNode.data.inputs.model,
                max_completion_tokens: agentNode.data.inputs.max_completion_tokens,
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
        //await storeResponseInAgenticMemory(props.data.outputs.response);
        return { response: props.data.outputs.response };
    }

    let body = {};
    if (isO1Model(agentNode.data.inputs.model)) {
        body = {
            model: agentNode.data.inputs.model,
            max_completion_tokens: agentNode.data.inputs.max_completion_tokens,
            temperature: agentNode.data.inputs.temperature,
            messages: [
                {
                    role: "user",
                    content: `${agentNode.data.inputs.system_prompt}\n\n${prompt}`,
                },
            ],
            functions: [combinedRetrieveFunction, agenticRetrieveFunction],
            reasoning_effort: "high",
            stream: false,
        };
    } else {
        body = {
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
            functions: [mcpServerFunctions],
            function_call: "auto",
            stream: false,
        };
    }

    const responseData = await fetch(endpoint, {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${agentNode.data.inputs.api_key}`,
        },
        body: JSON.stringify(body),
    });
    const result = await responseData.json();
    const message = result.choices?.[0]?.message;
    if (message && message.function_call) {
        const functionName = message.function_call.name;
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
            if (body.messages?.[1]) {
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
    body.messages[0].content = "Use the provided documents to respond to the user's query.";
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

// ---------------------------
// run() method
// ---------------------------
async function run() {
    console.log('Running AgentNode:', props.id);
    try {
        const agentNode = findNode(props.id);
        let finalPrompt = props.data.inputs.user_prompt;

        const connectedSources = getEdges.value
            .filter((edge) => edge.target === props.id)
            .map((edge) => edge.source);

        if (connectedSources.length > 0) {
            for (const sourceId of connectedSources) {
                const sourceNode = findNode(sourceId);
                if (sourceNode) {
                    finalPrompt += `\n\n${sourceNode.data.outputs.result.output}`;
                }
            }
        }

        if (provider.value === 'openai') {
            return await callCompletionsAPI_openai(agentNode, finalPrompt);
        } else {
            return await callCompletionsAPI_local(agentNode, finalPrompt);
        }
    } catch (error) {
        console.error('Error in AgentNode run:', error);
        return { error };
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
