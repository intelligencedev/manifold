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
import { useConfigStore } from '@/stores/configStore'
import { ref, computed, onMounted, watch } from 'vue'
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'

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
const { getEdges, findNode, zoomIn, zoomOut, updateNodeData } = useVueFlow()
const emit = defineEmits(['update:data', 'resize', 'disable-zoom', 'enable-zoom'])

const showApiKey = ref(false);

// New: Predefined System Prompt Options & selection
const selectedSystemPrompt = ref("friendly_assistant");
const systemPromptOptions = {
    friendly_assistant: {
        role: "Friendly Assistant",
        system_prompt: "You are a helpful, friendly, and knowledgeable general-purpose AI assistant. You can answer questions, provide information, engage in conversation, and assist with a wide variety of tasks.  Be concise in your responses when possible, but prioritize clarity and accuracy.  If you don't know something, admit it.  Maintain a conversational and approachable tone."
    },
    search_assistant: {
        role: "Search Assistant",
        system_prompt: "You are a helpful assistant that specializes in generating effective search engine queries.  Given any text input, your task is to create one or more concise and relevant search queries that would be likely to retrieve information related to that text from a search engine (like Google, Bing, etc.).  Consider the key concepts, entities, and the user's likely intent.  Prioritize clarity and precision in the queries."
    },
    research_analyst: {
        role: "Research Analyst",
        system_prompt: "You are a skilled research analyst with deep expertise in synthesizing information. Approach queries by breaking down complex topics, organizing key points hierarchically, evaluating evidence quality, providing multiple perspectives, and using concrete examples. Present information in a structured format with clear sections, use bullet points for clarity, and visually separate different points with markdown. Always cite limitations of your knowledge and explicitly flag speculation."
    },
    creative_writer: {
        role: "Creative Writer",
        system_prompt: "You are an exceptional creative writer. When responding, use vivid sensory details, emotional resonance, and varied sentence structures. Organize your narratives with clear beginnings, middles, and ends. Employ literary techniques like metaphor and foreshadowing appropriately. When providing examples or stories, ensure they have depth and authenticity. Present creative options when asked, rather than single solutions."
    },
    code_expert: {
        role: "Programming Expert",
        system_prompt: "You are a senior software developer with expertise across multiple programming languages. Present code solutions with clear comments explaining your approach. Structure responses with: 1) Problem understanding 2) Solution approach 3) Complete, executable code 4) Explanation of how the code works 5) Alternative approaches. Include error handling in examples, use consistent formatting, and provide explicit context for any code snippets. Test your solutions mentally before presenting them."
    },
    teacher: {
        role: "Educational Expert",
        system_prompt: "You are an experienced teacher skilled at explaining complex concepts. Present information in a structured, progressive manner from foundational to advanced. Use analogies and examples to connect new concepts to familiar ones. Break down complex ideas into smaller components. Incorporate multiple formats (definitions, examples, diagrams described in text) to accommodate different learning styles. Ask thought-provoking questions to deepen understanding. Anticipate common misconceptions and address them proactively."
    },
    data_analyst: {
        role: "Data Analysis Expert",
        system_prompt: "You are a data analysis expert. When working with data, focus on identifying patterns and outliers, considering statistical significance, and exploring causal relationships vs. correlations. Present your analysis with a clear narrative structure that connects data points to insights. Use hypothetical data visualization descriptions when relevant. Consider alternative interpretations of data and potential confounding variables. Clearly communicate limitations and assumptions in any analysis."
    }
};

// The run() logic
onMounted(() => {
    if (!props.data.run) {
        props.data.run = run
    }
})

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
};

async function callCombinedRetrieveAPI(userPrompt) {
    // Build payload with hardcoded configuration.
    const payload = {
        query: userPrompt,
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

    // Specify the endpoint URL.
    // (Make sure the URL matches your backend config/environment.)
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
        const data = await response.json();
        return data;
    } catch (error) {
        console.error("Error calling combined retrieve API:", error);
        throw error;
    }
}


async function callCompletionsAPI(agentNode, prompt) {
    const responseNodeId = getEdges.value.find((e) => e.source === props.id)?.target;
    const responseNode = responseNodeId ? findNode(responseNodeId) : null;
    let endpoint = agentNode.data.inputs.endpoint;

    // Corrected model check: returns true if model starts with 'o1' or 'o3'
    const isO1Model = (model) => {
        const lower = model.toLowerCase();
        return lower.startsWith("o1") || lower.startsWith("o3");
    };

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
            functions: [combinedRetrieveFunction],
            stream: true,
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
            functions: [combinedRetrieveFunction],
            function_call: "auto",
            //stream: true,
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

    // Check if a function call was returned:
    const message = result.choices[0]?.message;
    if (message && message.function_call) {
        const functionName = message.function_call.name;
        const functionParameters = message.function_call.parameters;

        if (functionName === "combined_retrieve") {
            const retrieveResult = await callCombinedRetrieveAPI(prompt);

            // Example result:
            // {
            //     "documents": {
            //         ".config.yaml": "# Manifold Example Configuration\n\n# Manifold Host\nhost: 'localhost'\nport: 8080\n\n# Manifold storage path: models, database files, etc\ndata_path: '~/.manifold'\n\n# Database Configuration (PGVector)\ndatabase:\n  connection_string: \"postgres://pgadmin:yourpassword@localhost:5432/manifold?sslmode=disable\"  # REPLACE with your actual credentials\n\n# HuggingFace Token\nhf_token: \"...\" \n\n# Anthropic API token\nanthropic_key: \"...\"\n\n# Default Completions Configuration - any openai api compatible backend - lla\nma.cpp (llama-server), vllm, mlx_lm.server, etc\ncompletions:\n  default_host: \"\u003cmy_openai_api_compatible_server\u003e/v1/chat/completions\"\n  # OpenAI API co\n\n---\n\n[host port data_path database_connection_string hf_token anthropic_key ma.cpp (llama-server) vllm mlx_lm.server]\n\nma.cpp (llama-server), vllm, mlx_lm.server, etc\ncompletions:\n  default_host: \"\u003cmy_openai_api_compatible_server\u003e/v1/chat/completions\"\n  # OpenAI API compatible API key, not required for local servers unless configured on that server\n  api_key: \"my_api_key\"\n\n# Embeddings API Configuration\nembeddings:\n  host: \"\u003cmy_openai_api_compatible_server\u003e/v1/embeddings\"\n  # OpenAI API compatible API key, not required for local servers unless configured on that server\n  api_key: \"your_embeddings_api_key\"\n  embe\ndding_vectors: 768 # Size of embedding vectors depending on model\n\n# Reranker llama.cpp endpoint\nreranker:\n  host: \"\u003cmy llama.cpp or other /v1/rerank \n\n---\n\n[llama-server vllm mlx_lm.server completions default_host api_key embeddings host embedding_vectors]\n\ndding_vectors: 768 # Size of embedding vectors depending on model\n\n# Reranker llama.cpp endpoint\nreranker:\n  host: \"\u003cmy llama.cpp or other /v1/rerank endpoint\u003e\"\n\n\n# OBSERVABILITY\n\n# Jaeger endpoint for tracing. Actual Jaeger deployment not required but server will throw error. \n# Leave as-is unless you have a real Jaeger endpoint to configure\njaeger_host: 'localhost:16686'\n\n---\n\n[adding_vectors size embedding vectors reranker llama.cpp endpoint host jaeger_host]\n\n",
            //         "README.md": "\u003cdiv align=\"center\"\u003e\n\n# Manifold\n\n\u003c/div\u003e\n\n![Manifold](docs/manifold_splash.jpg)\nManifold is a powerful platform designed for workflow automation using AI models. It supports text generation, image generation, and retrieval-augment\n\n---\n\n[AI models workflow automation text generation image generation retrieval-augmentation]\n\nManifold is a powerful platform designed for workflow automation using AI models. It supports text generation, image generation, and retrieval-augmented generation, integrating seamlessly with popular AI endpoints including OpenAI, llama.cpp, Apple's MLX LM, Google Gemini, Anthropic Claude, ComfyUI, and MFlux. Additionally, Manifold provides robust semantic search capabilities using PGVector combined with the SEFII (Semantic Embedding Forest with Inverted Index) engine.\n\u003e **Note:** Manifold is under active development, and breaking changes are expected. It is **NOT** production-ready. Contributions are highly encourag\n\n---\n\n[Manifold workflow automation AI models text generation image generation retrieval-augmented generation OpenAI llama.cpp Apple's MLX LM Google Gemini Anthropic Claude]\n\n\u003e **Note:** Manifold is under active development, and breaking changes are expected. It is **NOT** production-ready. Contributions are highly encouraged!\n\n---\n\n## Prerequisites\n\nEnsure the following software is installed before proceeding:\n- **Go:** Version 1.21 or newer ([Download](https://golang.org/dl/)).\n- **Python:** Version 3.10 or newer ([Download](https://www.python.org/downloads\n\n---\n\n[Manifold development breaking changes production-ready contributions Go Python]\n\n- **Go:** Version 1.21 or newer ([Download](https://golang.org/dl/)).\n- **Python:** Version 3.10 or newer ([Download](https://www.python.org/downloads/)).\n- **Node.js:** Version 20 managed via `nvm` ([Installation Guide](https://github.com/nvm-sh/nvm)).\n- **Docker:** Recommended for easy setup of PGVector ([Download](https://www.docker.com/get-started)).\n\n---\n\n## Installation Steps\n\n### 1. Clone the Repository\n```bash\ngit clone \u003crepository_url\u003e  # Replace with actual repository URL\ncd manifold\n```\n\n### 2. Set Up PGVector\n\nPGVector provides efficient similari\n\n---\n\n[Go Python Node.js Docker PGVector]\n\n```bash\ngit clone \u003crepository_url\u003e  # Replace with actual repository URL\ncd manifold\n```\n\n### 2. Set Up PGVector\n\nPGVector provides efficient similarity search for retrieval workflows.\n\n**Docker Installation (Recommended):**\n\n```bash\ndocker run -d \\\n  --name pg-manifold \\\n  -p 5432:5432 \\\n  -v postgres-data:/var/lib/postgresql/data \\\n  -e POSTGRES_USER=myuser \\\n  -e POSTGRES_PASSWORD=changeme \\\n  -e POSTGRES_DB=manifold \\\n  pgvector/pgvector:latest\n```\n\u003e **Important:** Update `myuser` and `changeme` with your preferred username and password.\n\n**Verification:**\n\nVerify your PGVector installation using\n\n---\n\n[git clone repository URL cd manifold Docker Installation (Recommended) docker run -d --name pg-manifold -p 5432:5432 -v postgres-data:/var/lib/postgresql/data -e POSTGRES_USER=myuser]\n\n\u003e **Important:** Update `myuser` and `changeme` with your preferred username and password.\n\n**Verification:**\n\nVerify your PGVector installation using `psql`:\n\n```bash\npsql -h localhost -p 5432 -U myuser -d manifold\n```\n\nYou should see a prompt like `manifold=#`. Type `\\q` to exit.\n\n**Alternate Installation:**\n\nFor non-Docker methods, refer to the [PGVector documentation](https://github.com/pgvector/pgvector#installation).\n\n---\n\n### 3. Configure an Image Generation Backend (Choose One)\n#### Option A: ComfyUI (Cross-platform)\n\n- Follow the [official ComfyUI installation guide](https://github.com/comfyanonymous/ComfyUI#manual-install-w\n\n---\n\n[myuser changeme psql localhost 5432 manifold]\n\n#### Option A: ComfyUI (Cross-platform)\n\n- Follow the [official ComfyUI installation guide](https://github.com/comfyanonymous/ComfyUI#manual-install-windows-linux).\n- No extra configuration needed; Manifold connects via proxy.\n\n#### Option B: MFlux (M-series Macs Only)\n\n- Follow the [MFlux installation guide](https://github.com/filipstrand/mflux).\n\n---\n\n### 4. Build and Run Manifold\n\nExecute the following commands:\n```bash\nnvm use 20\nnpm run build\ngo build -ldflags=\"-s -w\" -trimpath -o ./dist/manifold main.go\ncd dist\n./manifold\n```\n\nThis sequence will:\n\n- Switch \n\n---\n\n[ComfyUI MFlux manual-install windows-linux M-series-Macs-Only installation-guide no-extra-configuration Manifold-proxy-connection]\n\n```bash\nnvm use 20\nnpm run build\ngo build -ldflags=\"-s -w\" -trimpath -o ./dist/manifold main.go\ncd dist\n./manifold\n```\n\nThis sequence will:\n\n- Switch Node.js to version 20.\n- Build frontend assets.\n- Compile the Go backend, generating the executable.\n- Launch Manifold from the `dist` directory.\n\nUpon first execution, Manifold creates necessary directories and files (e.g., `data`).\n\n---\n\n### 5. Configuration (`config.yaml`)\nCreate or update your configuration based on the provided `.config.yaml` example in the repository root:\n\n```yaml\nhost: localhost\nport: 8080\ndata_path\n\n---\n\n[node.js version switch npm run build go compile flags trimpath output directory]\n\nCreate or update your configuration based on the provided `.config.yaml` example in the repository root:\n\n```yaml\nhost: localhost\nport: 8080\ndata_path: ./data\njaeger_host: localhost:6831  # Optional Jaeger tracing\n\n# API Keys (optional integrations)\nanthropic_key: \"...\"\nopenai_api_key: \"...\"\ngoogle_gemini_key: \"...\"\nhf_token: \"...\"\n\n# Database Configuration\ndatabase:\n  connection_string: \"postgres://myuser:changeme@localhost:5432/manifold\"\n# Completion and Embedding Services\ncompletions:\n  default_host: \"http://localhost:8081\"  # Example: llama.cpp server\n  api_key: \"\"\n\nembeddings:\n  hos\n\n---\n\n[configuration .config.yaml host port data_path jaeger_host API Keys anthropic_key openai_api_key google_gemini_key hf_token]\n\n# Completion and Embedding Services\ncompletions:\n  default_host: \"http://localhost:8081\"  # Example: llama.cpp server\n  api_key: \"\"\n\nembeddings:\n  host: \"http://localhost:8081\"  # Example: llama.cpp server\n  api_key: \"\"\n  embedding_vectors: 1024\n```\n\n**Crucial Points:**\n\n- Update database credentials (`myuser`, `changeme`) according to your PGVector setup.\n- Adjust `default_host` and `embeddings.host` based on your chosen model server.\n\n---\n\n## Accessing Manifold\nLaunch your browser and navigate to:\n\n```\nhttp://localhost:8080\n```\n\n\u003e Replace `8080` if you customized your port in `config.yaml`.\n\n---\n\n## Supported\n\n---\n\n[completion embedding default_host api_key host embedding_vectors]\n\nLaunch your browser and navigate to:\n\n```\nhttp://localhost:8080\n```\n\n\u003e Replace `8080` if you customized your port in `config.yaml`.\n\n---\n\n## Supported Endpoints\n\nManifold is compatible with OpenAI-compatible endpoints:\n\n- [llama.cpp Server](https://github.com/ggerganov/llama.cpp/tree/master/examples/server)\n- [Apple MLX LM Server](https://github.com/ml-explore/mlx-examples/blob/main/llms/mlx_lm/SERVER.md)\n\n---\n\n## Troubleshooting Common Issues\n- **Port Conflict:** If port 8080 is occupied, either terminate conflicting processes or choose a new port in `config.yaml`.\n- **PGVector Connectivity\n\n---\n\n[browser navigate localhost port config.yaml llama.cpp Server Apple MLX LM Server troubleshooting]\n\n- **Port Conflict:** If port 8080 is occupied, either terminate conflicting processes or choose a new port in `config.yaml`.\n- **PGVector Connectivity:** Confirm your `database.connection_string` matches PGVector container credentials.\n- **Missing Config File:** Ensure `config.yaml` exists in the correct directory. Manifold will not launch without it.\n\n---\n\n## Contributing\n\nManifold welcomes contributions! Check the open issues for tasks and feel free to submit pull requests.\n\n---\n\n---\n\n[Port Conflict PGVector Connectivity Missing Config File]\n\n"
            //     }
            // }

            // Extract the documents from the retrieve result
            const documents = retrieveResult.documents || {};

            // Convert the documents to a string, with proper error handling
            let documentsString = '';
            if (typeof documents === 'object' && documents !== null) {
                documentsString = Object.entries(documents)
                    .map(([key, value]) => `${key}:\n\n${String(value)}`)
                    .join("\n\n");
            } else {
                console.error('Invalid documents format:', documents);
                documentsString = 'No valid documents found';
            }

            // Combine the retrieve result with the original prompt
            const combinedPrompt = `${prompt}\n\nREFERENCE:\n\n${documentsString}`;

            // Call the completions API again with the combined prompt
            body.messages[1].content = combinedPrompt;
            // body.stream = true;
            // const response = await fetch(endpoint, {
            //     method: "POST",
            //     headers: {
            //         "Content-Type": "application/json",
            //         Authorization: `Bearer ${agentNode.data.inputs.api_key}`,
            //     },
            //     body: JSON.stringify(body),
            // });
            // const result = await response.json();
            // return { response: result.choices[0].message.content };

            // continue outside of the if block


        }
    }


    // Ensure body stream is true and start the stream
    body.stream = true;

    // Remove the function call auto from the body
    delete body.functions;
    delete body.function_call;

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

    const reader = streamResponse.body.getReader();
    let buffer = "";
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
                        const tokenContent =
                            (delta.content || "") + (delta.thinking || "");

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

    return { response: props.data.outputs.response };
}


async function run() {
    console.log('Running AgentNode:', props.id);

    try {
        const agentNode = findNode(props.id);
        let finalPrompt = props.data.inputs.user_prompt;

        const connectedSources = getEdges.value
            .filter((edge) => edge.target === props.id)
            .map((edge) => edge.source);

        if (connectedSources.length > 0) {
            console.log('Connected sources:', connectedSources);
            for (const sourceId of connectedSources) {
                const sourceNode = findNode(sourceId);
                if (sourceNode) {
                    console.log('Processing source node:', sourceNode.id);
                    finalPrompt += `\n\n${sourceNode.data.outputs.result.output}`;
                }
            }
            console.log('Processed prompt:', finalPrompt);
        }

        return await callCompletionsAPI(agentNode, finalPrompt);
    } catch (error) {
        console.error('Error in AgentNode run:', error);
        return { error };
    }
}

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

const handleTextareaMouseEnter = () => {
    emit('disable-zoom')
    zoomIn(0);
    zoomOut(0);
};

const handleTextareaMouseLeave = () => {
    emit('enable-zoom')
    zoomIn(1);
    zoomOut(1);
};

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

watch(() => configStore.config.Completions.Provider, (newProvider) => {
    if (newProvider) {
        props.data.inputs.endpoint = configStore.config.Completions.DefaultHost;
    }
}, { immediate: true });

// Update the system prompt textbox when the dropdown selection changes
watch(selectedSystemPrompt, (newKey) => {
    if (systemPromptOptions[newKey]) {
        system_prompt.value = systemPromptOptions[newKey].system_prompt;
    }
}, { immediate: true });
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
