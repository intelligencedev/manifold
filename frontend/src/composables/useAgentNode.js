import { ref, computed, watch, onMounted } from "vue";
import { useConfigStore } from "@/stores/configStore";
import { useWorkflowStore } from "@/stores/workflowStore";
import { useCodeEditor } from "./useCodeEditor";
import { useVueFlow } from "@vue-flow/core";
import { useNodeBase } from "./useNodeBase";
import { useSystemPromptOptions } from "./systemPrompts";

/**
 * Composable for managing AgentNode state and functionality
 */
export function useAgentNode(props, emit) {
  // Access VueFlow API for node updates
  const { getEdges, findNode, updateNodeData } = useVueFlow();
  const configStore = useConfigStore();
  const { setEditorCode } = useCodeEditor();
  const workflowStore = useWorkflowStore();

  // State variables
  const showApiKey = ref(false);
  const enableToolCalls = ref(false);
  const { isHovered, customStyle, resizeHandleStyle, computedContainerStyle } =
    useNodeBase(props, emit);
  const { systemPromptOptions, systemPromptOptionsList } =
    useSystemPromptOptions();
    
  // Loading state for model fetching
  const isLoadingModel = ref(false);

  // Helper function to fetch model ID from llama-server
  async function fetchLlamaServerModel() {
    isLoadingModel.value = true;
    try {
      // Derive the models endpoint from the chat completions endpoint
      let modelsEndpoint = props.data.inputs.endpoint;
      
      // Extract the base URL from the endpoint
      const endpointParts = modelsEndpoint.split('/');
      const apiIndex = endpointParts.findIndex(part => part === 'api' || part === 'v1');
      
      let baseUrl;
      if (apiIndex !== -1) {
        // If '/api/' or '/v1/' is found in the path, use everything before including that part
        baseUrl = endpointParts.slice(0, apiIndex).join('/');
        modelsEndpoint = `${baseUrl}/models`;
      } else {
        // If not found, assume we need to replace the endpoint path
        const urlObj = new URL(modelsEndpoint);
        urlObj.pathname = '/models';
        modelsEndpoint = urlObj.toString();
      }

      console.log("Fetching model from:", modelsEndpoint);
      
      const response = await fetch(modelsEndpoint);
      if (!response.ok) {
        throw new Error(`Failed to fetch models: ${response.statusText}`);
      }
      
      const data = await response.json();
      
      if (data && data.data && data.data.length > 0) {
        // Get the first model ID
        const modelId = data.data[0].id;
        console.log("Found model:", modelId);
        model.value = modelId;
      }
    } catch (error) {
      console.error("Error fetching llama-server model:", error);
    } finally {
      isLoadingModel.value = false;
    }
  }

  // Helper function to create an event stream splitter
  // This is suitable for SSE format where events are `data: <payload>\n\n`
  function createEventStreamSplitter() {
    let buffer = "";
    return new TransformStream({
      transform(chunk, controller) {
        buffer += chunk;
        let idx;
        while ((idx = buffer.indexOf("\n\n")) !== -1) {
          const event = buffer
            .slice(0, idx)
            .replace(/^data:\s*/gm, "")
            .trim();
          if (event) {
            // Ensure non-empty event after processing
            controller.enqueue(event);
          }
          buffer = buffer.slice(idx + 2);
        }
      },
      flush(controller) {
        // Handle any remaining data in the buffer when the stream closes
        if (buffer.trim()) {
          const event = buffer.replace(/^data:\s*/gm, "").trim();
          if (event) {
            controller.enqueue(event);
          }
        }
      },
    });
  }

  // Helper functions for Gemini API
  function parseIncompleteJson(jsonString) {
    try {
      const validJson = JSON.parse(jsonString);
      return { valid: true, completeObject: validJson };
    } catch (e) {
      // Attempt to fix the JSON string
      let fixedJsonString = jsonString;
      if (e.message.includes("Unexpected end of JSON input")) {
        // Try to close any unclosed braces or brackets
        const openBraces = (fixedJsonString.match(/{/g) || []).length;
        const closeBraces = (fixedJsonString.match(/}/g) || []).length;
        const openBrackets = (fixedJsonString.match(/\[/g) || []).length;
        const closeBrackets = (fixedJsonString.match(/\]/g) || []).length;
        fixedJsonString += "}".repeat(openBraces - closeBraces);
        fixedJsonString += "]".repeat(openBrackets - closeBrackets);
      }
      try {
        const fixedJson = JSON.parse(fixedJsonString);
        return { valid: true, completeObject: fixedJson };
      } catch (e) {
        return { valid: false, completeObject: null };
      }
    }
  }

  function getCompleteJsonLength(jsonString) {
    let openBraces = 0;
    let openBrackets = 0;
    for (let i = 0; i < jsonString.length; i++) {
      if (jsonString[i] === "{") openBraces++;
      if (jsonString[i] === "}") openBraces--;
      if (jsonString[i] === "[") openBrackets++;
      if (jsonString[i] === "]") openBrackets--;
      if (openBraces === 0 && openBrackets === 0) {
        return i + 1;
      }
    }
    return jsonString.length; // Return total length if no complete object is found
  }

  // Function to update response in real-time as stream content comes in
  function onResponseUpdate(content, fullResponse) {
    // Update the UI with the streamed response
    props.data.outputs = {
      ...props.data.outputs,
      response: content,
      result: {
        // Keep result.output consistent with the latest full content
        output: fullResponse,
      },
    };

    // Also propagate updates to connected ResponseNode components
    const edges = getEdges.value.filter((edge) => edge.source === props.id);
    edges.forEach((edge) => {
      const targetId = edge.target;
      const node = findNode(targetId);
      if (node && node.data?.inputs) {
        const updated = {
          ...node.data,
          inputs: { ...node.data.inputs, response: content },
        };
        updateNodeData(targetId, updated);
      }
    });
  }

  // Predefined system prompts
  const selectedSystemPrompt = computed({
    get: () => props.data.selectedSystemPrompt || "",
    set: (val) => {
      props.data.selectedSystemPrompt = val;
    },
  });

  // Computed properties for form binding
  const model = computed({
    get: () => props.data.inputs.model,
    set: (value) => {
      props.data.inputs.model = value;
    },
  });

  const system_prompt = computed({
    get: () => props.data.inputs.system_prompt,
    set: (value) => {
      props.data.inputs.system_prompt = value;
    },
  });

  const user_prompt = computed({
    get: () => props.data.inputs.user_prompt,
    set: (value) => {
      props.data.inputs.user_prompt = value;
    },
  });

  const endpoint = computed({
    get: () => props.data.inputs.endpoint,
    set: (value) => {
      props.data.inputs.endpoint = value;
    },
  });

  const api_key = computed({
    get: () => props.data.inputs.api_key,
    set: (value) => {
      props.data.inputs.api_key = value;
    },
  });

  const max_completion_tokens = computed({
    get: () => props.data.inputs.max_completion_tokens,
    set: (value) => {
      props.data.inputs.max_completion_tokens = value;
    },
  });

  const temperature = computed({
    get: () => props.data.inputs.temperature,
    set: (value) => {
      props.data.inputs.temperature = value;
    },
  });

  const presence_penalty = computed({
    get: () => props.data.inputs.presence_penalty,
    set: (value) => { props.data.inputs.presence_penalty = value }
  });
  const top_p = computed({
    get: () => props.data.inputs.top_p,
    set: (value) => { props.data.inputs.top_p = value }
  });
  const top_k = computed({
    get: () => props.data.inputs.top_k,
    set: (value) => { props.data.inputs.top_k = value }
  });
  const min_p = computed({
    get: () => props.data.inputs.min_p,
    set: (value) => { props.data.inputs.min_p = value }
  });

  // Provider options and selection
  const providerOptions = [
    { value: "llama-server", label: "llama-server" },
    { value: "mlx_lm.server", label: "mlx_lm.server" },
    { value: "openai", label: "openai" },
    { value: "anthropic", label: "anthropic" },
    { value: "google", label: "google" },
  ];

  // Add models list for Anthropic
  const claudeModels = [
    "claude-3-7-sonnet-latest",
    "claude-3-5-sonnet-latest",
    "claude-3-5-haiku-latest",
  ];

  // Add models list for Google Gemini
  const geminiModels = [
    "gemini-2.5-flash-preview-05-20",
    "gemini-2.5-pro-preview-05-06",
    "gemini-2.0-flash-lite",
  ];

  // Computed property to dynamically show the appropriate models based on provider
  const modelOptions = computed(() => {
    if (provider.value === "anthropic") {
      return claudeModels.map((model) => ({ value: model, label: model }));
    } else if (provider.value === "google") {
      return geminiModels.map((model) => ({ value: model, label: model }));
    } else {
      // Return OpenAI or local models based on existing logic
      return [];
    }
  });

  // Provider detection and setting
  const provider = computed({
    get: () => {
      if (
        props.data.inputs.endpoint ===
        "https://api.openai.com/v1/chat/completions"
      ) {
        return "openai";
      } else if (props.data.inputs.endpoint === "/api/anthropic/messages") {
        return "anthropic";
      } else if (props.data.inputs.endpoint === "/api/gemini/generate") {
        return "google";
      } else if (
        props.data.inputs.endpoint ===
        configStore.config?.Completions?.DefaultHost
      ) {
        if (configStore.config?.Completions?.Provider === "llama-server") {
          return "llama-server";
        } else if (
          configStore.config?.Completions?.Provider === "mlx_lm.server"
        ) {
          return "mlx_lm.server";
        }
      }

      if (
        props.data.inputs.endpoint?.includes("localhost") ||
        props.data.inputs.endpoint?.includes("127.0.0.1")
      ) {
        if (props.data._lastLocalProvider === "mlx_lm.server") {
          return "mlx_lm.server";
        }
        return "llama-server"; // Default local to llama-server
      }
      // Default to openai if endpoint looks like openai, otherwise llama-server as a general default
      return props.data.inputs.endpoint?.includes("openai.com")
        ? "openai"
        : "llama-server";
    },
    set: (value) => {
      if (value !== "openai") {
        props.data._lastLocalProvider = value;
      }

      if (value === "openai") {
        props.data.inputs.endpoint =
          "https://api.openai.com/v1/chat/completions";
      } else if (value === "anthropic") {
        props.data.inputs.endpoint = "/api/anthropic/messages";
      } else if (value === "google") {
        props.data.inputs.endpoint = "/api/gemini/generate";
      } else if (
        !props.data.inputs.endpoint ||
        props.data.inputs.endpoint ===
          "https://api.openai.com/v1/chat/completions" ||
        props.data.inputs.endpoint === "/api/anthropic/messages" ||
        props.data.inputs.endpoint === "/api/gemini/generate"
      ) {
        // If current endpoint is empty or OpenAI/Anthropic, set to default local
        props.data.inputs.endpoint =
          configStore.config?.Completions?.DefaultHost ||
          "http://localhost:32186/v1/chat/completions";
      }
      // Otherwise, keep user's custom endpoint
    },
  });

  // Node functionality
  async function run() {
    console.log("Running AgentNode:", props.id);
    props.data.outputs.error = null;
    props.data.outputs.response = "";
    onResponseUpdate("", ""); // Clear connected nodes too

    let result = { content: "" };

    try {
      let finalPrompt = props.data.inputs.user_prompt;
      const currentProvider = provider.value;

      // --- aggregate text from all connected source nodes ---
      const incomingEdges = getEdges.value.filter(
        (edge) => edge.target === props.id,
      );

      // --- Regular API call with streaming ---
      let visionContent = null;
      const imageDataUrls = getEdges.value
        .filter((edge) => edge.target === props.id)
        .map((edge) => findNode(edge.source))
        .filter(
          (node) => node?.data?.isImage && node.data.outputs?.result?.dataUrl,
        )
        .map((node) => node.data.outputs.result.dataUrl);

      if (imageDataUrls.length) {
        visionContent = [];
        // For llama-server and mlx_lm.server, pass images directly without uploading
        if (["llama-server", "mlx_lm.server"].includes(currentProvider)) {
          for (const dataUrl of imageDataUrls) {
            visionContent.push({ type: "image_url", image_url: { url: dataUrl } });
          }
        } else {
          // For other providers (OpenAI, etc.), try to upload images first
          const uploadEndpoint = `${new URL(props.data.inputs.endpoint).origin}/api/upload`;
          for (const dataUrl of imageDataUrls) {
            let fileUrl = dataUrl;
            if (dataUrl.startsWith("data:")) {
              try {
                const blob = await (await fetch(dataUrl)).blob();
                const formData = new FormData();
                formData.append("file", blob);
                const uploadResp = await fetch(uploadEndpoint, { method: "POST", body: formData });
                if (uploadResp.ok) {
                  const json = await uploadResp.json();
                  fileUrl = json.url;
                } else {
                  console.error("Image upload failed:", uploadResp.statusText);
                  // Fall back to using the data URL directly
                  fileUrl = dataUrl;
                }
              } catch (e) {
                console.error("Error uploading image:", e);
                // Fall back to using the data URL directly
                fileUrl = dataUrl;
              }
            }
            visionContent.push({ type: "image_url", image_url: { url: fileUrl } });
          }
        }
        // Always append the user prompt as a text part after all images
        visionContent.push({ type: "text", text: finalPrompt });
      } else {
        for (const edge of incomingEdges) {
          const sourceNode = findNode(edge.source);
          if (sourceNode?.data?.outputs?.result?.output) {
            finalPrompt += `\n\n${sourceNode.data.outputs.result.output}`;
          }
        }
      }

      let requestBody = {
        messages: [
          {
            role: "user",
            content: visionContent ? visionContent : finalPrompt,
          },
        ],
        temperature: props.data.inputs.temperature ?? 0.7,
      };

      const modelName = props.data.inputs.model.toLowerCase();

      // Only include model parameter for OpenAI provider
      if (currentProvider === "openai") {
        requestBody.model = props.data.inputs.model;

        if (modelName.startsWith("o") && /^o[0-9]/.test(modelName)) {
          requestBody.max_completion_tokens =
            props.data.inputs.max_completion_tokens || 1000;
          requestBody.reasoning_effort = "high";
        } else {
          requestBody.max_completion_tokens =
            props.data.inputs.max_completion_tokens || 1000;
        }
      } else {
        // For non-OpenAI providers
        requestBody.max_completion_tokens =
          props.data.inputs.max_completion_tokens || 1000;
      }

      // Add extra LLM params for openai, llama-server, mlx_lm.server
      if (["openai", "llama-server", "mlx_lm.server"].includes(currentProvider)) {
        if (props.data.inputs.presence_penalty !== undefined && props.data.inputs.presence_penalty !== null && props.data.inputs.presence_penalty !== '') requestBody.presence_penalty = props.data.inputs.presence_penalty;
        if (props.data.inputs.top_p !== undefined && props.data.inputs.top_p !== null && props.data.inputs.top_p !== '') requestBody.top_p = props.data.inputs.top_p;
        if (props.data.inputs.min_p !== undefined && props.data.inputs.min_p !== null && props.data.inputs.min_p !== '') requestBody.min_p = props.data.inputs.min_p;
        // Only include top_k for mlx_lm.server
        if (currentProvider === 'mlx_lm.server' && props.data.inputs.top_k !== undefined && props.data.inputs.top_k !== null && props.data.inputs.top_k !== '') requestBody.top_k = props.data.inputs.top_k;
      }

      // Add vision-specific parameters when images are present
      if (visionContent && ["llama-server", "mlx_lm.server"].includes(currentProvider)) {
        requestBody.cache_prompt = true;
        requestBody.stream = true;
      }

      const canStream =
        currentProvider === "openai" ||
        currentProvider === "llama-server" ||
        currentProvider === "mlx_lm.server";

      // --- Handle Anthropic/Claude Provider ---
      if (currentProvider === "anthropic") {
        // Build Anthropic request body
        const anthropicRequestBody = {
          model: props.data.inputs.model,
          max_tokens: parseInt(props.data.inputs.max_completion_tokens || 1024),
          messages: [{ role: "user", text: finalPrompt }],
          stream: true,
        };

        // Add system prompt if provided
        if (
          props.data.inputs.system_prompt &&
          props.data.inputs.system_prompt.trim() !== ""
        ) {
          anthropicRequestBody.system = [
            props.data.inputs.system_prompt.trim(),
          ];
        }

        // Call Anthropic API
        const response = await fetch(props.data.inputs.endpoint, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "x-api-key": props.data.inputs.api_key,
            "anthropic-version": "2023-06-01",
          },
          body: JSON.stringify(anthropicRequestBody),
          signal: workflowStore.signal,
        });

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(`API error (${response.status}): ${errorText}`);
        }

        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let responseText = "";

        // Stream response and update UI
        while (true) {
          if (workflowStore.stopRequested) {
            await reader.cancel();
            break;
          }
          const { done, value } = await reader.read();
          if (done) break;
          const chunk = decoder.decode(value, { stream: true });
          responseText += chunk;
          onResponseUpdate(responseText, responseText);
        }

        props.data.outputs = {
          ...props.data.outputs,
          response: responseText,
          result: { output: responseText },
        };

        return { content: responseText };
      }
      // --- Handle Google Gemini Provider ---
      else if (currentProvider === "google") {
        // Include system prompt in the user prompt for Gemini (as it doesn't have a separate system prompt)
        let geminiPrompt = finalPrompt;
        if (
          props.data.inputs.system_prompt &&
          props.data.inputs.system_prompt.trim() !== ""
        ) {
          geminiPrompt = `${props.data.inputs.system_prompt.trim()}\n\n${finalPrompt}`;
        }

        // Construct the request body
        const requestBody = {
          contents: [
            {
              role: "user",
              parts: [{ text: geminiPrompt }],
            },
          ],
        };

        const proxyBody = {
          model: props.data.inputs.model,
          contents: requestBody.contents,
        };

        const response = await fetch(props.data.inputs.endpoint, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify(proxyBody),
          signal: workflowStore.signal,
        });

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(`API error (${response.status}): ${errorText}`);
        }

        const reader = response.body.getReader();
        let buffer = "";
        let startedArray = false;
        let accumulatedResponse = "";

        while (true) {
          if (workflowStore.stopRequested) {
            await reader.cancel();
            break;
          }
          const { done, value } = await reader.read();
          if (done) break;

          buffer += new TextDecoder().decode(value);

          // Wait for the opening '[' of the array
          if (!startedArray) {
            const idx = buffer.indexOf("[");
            if (idx !== -1) {
              buffer = buffer.slice(idx + 1);
              startedArray = true;
            } else {
              continue;
            }
          }

          while (true) {
            if (workflowStore.stopRequested) {
              break;
            }
            // Trim any leading whitespace or commas
            buffer = buffer.trimStart();
            if (buffer.startsWith("]")) {
              buffer = buffer.slice(1);
              startedArray = false;
              break;
            }

            const startIdx = buffer.indexOf("{");
            if (startIdx === -1) break;

            const jsonLength = getCompleteJsonLength(buffer.slice(startIdx));
            if (jsonLength > buffer.slice(startIdx).length) {
              // Incomplete object, wait for more data
              break;
            }

            const jsonStr = buffer.slice(startIdx, startIdx + jsonLength);
            try {
              const obj = JSON.parse(jsonStr);
              const responseContent =
                obj.candidates?.[0]?.content?.parts?.[0]?.text || "";

              accumulatedResponse += responseContent;
              onResponseUpdate(accumulatedResponse, accumulatedResponse);
            } catch (e) {
              console.error("Error parsing Gemini chunk:", e);
            }

            buffer = buffer.slice(startIdx + jsonLength);
            // Remove trailing comma after the object if present
            if (buffer.startsWith(",")) {
              buffer = buffer.slice(1);
            }
          }
        }

        props.data.outputs = {
          ...props.data.outputs,
          response: accumulatedResponse,
          result: { output: accumulatedResponse },
        };

        return { content: accumulatedResponse };
      } else if (canStream) {
        requestBody.stream = true;

        const headers = {
          "Content-Type": "application/json",
          Accept: "text/event-stream",
        };
        if (currentProvider === "openai" && props.data.inputs.api_key) {
          headers["Authorization"] = `Bearer ${props.data.inputs.api_key}`;
        }

        const sseResp = await fetch(props.data.inputs.endpoint, {
          method: "POST",
          headers: headers,
          body: JSON.stringify(requestBody),
          signal: workflowStore.signal,
        });

        if (!sseResp.ok) {
          const errorText = await sseResp.text();
          throw new Error(`API error (${sseResp.status}): ${errorText}`);
        }

        const reader = sseResp.body
          .pipeThrough(new TextDecoderStream())
          .pipeThrough(createEventStreamSplitter())
          .getReader();

        let accumulatedContent = "";
        try {
          while (true) {
            if (workflowStore.stopRequested) {
              await reader.cancel();
              break;
            }
            const { value, done } = await reader.read();
            if (done) break;

            if (value.trim() === "[DONE]") {
              await reader.cancel();
              break;
            }

            try {
              const chunkData = JSON.parse(value);
              let deltaContent = "";
              if (
                chunkData.choices &&
                chunkData.choices[0] &&
                chunkData.choices[0].delta
              ) {
                deltaContent = chunkData.choices[0].delta.content || "";
              }

              if (deltaContent) {
                // Check and remove any stop tokens that might have leaked through
                if (deltaContent.includes("<|im_end|>")) {
                  deltaContent = deltaContent.replace(/<\|im_end\|>/g, "");
                }
                accumulatedContent += deltaContent;
                onResponseUpdate(accumulatedContent, accumulatedContent);
              }
            } catch (e) {
              console.warn(
                "Failed to parse stream chunk as JSON:",
                value,
                e.message,
              );
            }
          }
        } catch (e) {
          console.error("Error reading chat completion stream:", e.message);
          throw e;
        }

        props.data.outputs = {
          ...props.data.outputs,
          response: accumulatedContent,
          result: { output: accumulatedContent },
        };
        result = { content: accumulatedContent };
      } else if (currentProvider === "anthropic") {
        return await handleAnthropicProvider(
          props,
          finalPrompt,
          onResponseUpdate,
        );
      } else {
        // --- Fallback to non-streaming for other providers ---
        const response = await fetch(props.data.inputs.endpoint, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${props.data.inputs.api_key}`,
          },
          body: JSON.stringify(requestBody),
          signal: workflowStore.signal,
        });

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(`API error (${response.status}): ${errorText}`);
        }

        const responseData = await response.json();
        const responseText =
          (responseData.choices && responseData.choices[0]?.message?.content) ||
          "";

        props.data.outputs = {
          ...props.data.outputs,
          response: responseText,
          result: { output: responseText },
        };
        onResponseUpdate(responseText, responseText);
        result = { content: responseText };
      }
      return result;
    } catch (error) {
      console.error("Error in AgentNode run:", props.id, error);
      const errorMessage = error.message || "Unknown error occurred";

      props.data.outputs.error = errorMessage;
      const errorResponse = JSON.stringify(
        {
          error: errorMessage,
          details: error.cause ? String(error.cause) : undefined,
          partialResponse: props.data.outputs.response,
        },
        null,
        2,
      );
      props.data.outputs.response = errorResponse;

      const targetEdges = getEdges.value.filter(
        (edge) => edge.source === props.id,
      );
      targetEdges.forEach((edge) => {
        const connectedNode = findNode(edge.target);
        if (connectedNode && connectedNode.data && connectedNode.data.inputs) {
          connectedNode.data.inputs.response = errorResponse;
        }
      });
      return { error: errorMessage, content: props.data.outputs.response };
    }
  }

  /**
   * Sends code to the code editor
   */
  function sendToCodeEditor() {
    if (props.data.outputs && props.data.outputs.response) {
      const responseText = props.data.outputs.response;
      const codeBlockRegex =
        /```(?:javascript|js|python|go|typescript|ts|html|css|json|yaml|sh|bash)?\s*([\s\S]*?)```/gi;
      let allCode = "";
      let match;
      while ((match = codeBlockRegex.exec(responseText)) !== null) {
        allCode += match[1].trim() + "\n\n";
      }

      if (allCode.trim()) {
        setEditorCode(allCode.trim());
      } else {
        setEditorCode(responseText);
      }
    }
  }

  // Event handlers
  function onResize(event) {
    const minHeight = 800;
    console.log("Resizing AgentNode:", props.id, minHeight);
    customStyle.value.width = `${event.width}px`;
    customStyle.value.height = `${event.height}px`;
    // Emit resize event with minimum height
    emit("resize", { event, minHeight });
  }

  function handleTextareaMouseEnter() {
    emit("disable-zoom");
  }

  function handleTextareaMouseLeave() {
    emit("enable-zoom");
  }

  // Lifecycle hooks and watchers
  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run;
    }
    if (props.data.style) {
      customStyle.value.width = props.data.style.width || "380px";
      customStyle.value.height = props.data.style.height || "906px";
    }
  });

  watch(
    () => configStore.config,
    (newConfig) => {
      if (newConfig && newConfig.Completions) {
        if (!props.data.inputs.api_key && newConfig.Completions.APIKey) {
          props.data.inputs.api_key = newConfig.Completions.APIKey;
        }
        if (!props.data.inputs.endpoint && newConfig.Completions.DefaultHost) {
          props.data.inputs.endpoint = newConfig.Completions.DefaultHost;
        }
      }
    },
    { immediate: true, deep: true },
  );

  watch(
    () => configStore.config?.Completions?.Provider,
    (newProvider) => {
      if (newProvider && provider.value !== "openai") {
        props.data.inputs.endpoint = configStore.config.Completions.DefaultHost;
      }
    },
    { immediate: true },
  );

  watch(selectedSystemPrompt, (newKey) => {
    if (systemPromptOptions[newKey]) {
      system_prompt.value = systemPromptOptions[newKey].system_prompt;
    }
  });

  // Update model when provider changes
  watch(provider, (newProvider) => {
    if (newProvider === "anthropic") {
      // Set a default Claude model if current model is not in claudeModels
      if (!claudeModels.includes(model.value)) {
        model.value = claudeModels[0]; // Default to first Claude model
      }
    } else if (newProvider === "google") {
      // Set a default Gemini model if current model is not in geminiModels
      if (!geminiModels.includes(model.value)) {
        model.value = geminiModels[0]; // Default to first Gemini model
      }
    } else if ((newProvider === "llama-server" || newProvider === "mlx_lm.server") && props.data.inputs.endpoint) {
      // Fetch model ID from local servers when provider is selected
      fetchLlamaServerModel();
    }
  });
  

  // Watch provider changes to swap API key accordingly
  watch(provider, (newProvider) => {
    if (newProvider === 'google') {
      if (configStore.config?.google_gemini_key) {
        props.data.inputs.api_key = configStore.config.GoogleGeminiKey;
      }
    } else if (newProvider === 'openai' || newProvider === 'anthropic' || newProvider === 'llama-server' || newProvider === 'mlx_lm.server') {
      // Reset to general API key for other providers
      if (configStore.config?.Completions?.APIKey) {
        props.data.inputs.api_key = configStore.config.Completions.APIKey;
      }
    }
  });
  if (!props.data.style) {
    props.data.style = {
      border: "1px solid #666",
      borderRadius: "12px",
      backgroundColor: "#333",
      color: "#eee",
      width: "380px",
      height: "906px",
    };
  }
  customStyle.value.width = props.data.style.width || "380px";
  customStyle.value.height = props.data.style.height || "906px";

  return {
    showApiKey,
    enableToolCalls,
    agentMode: ref(false), // Keep this ref but set to false always
    selectedSystemPrompt,
    isHovered,
    systemPromptOptionsList,
    providerOptions,
    provider,
    endpoint,
    api_key,
    model,
    max_completion_tokens,
    temperature,
    presence_penalty,
    top_p,
    top_k,
    min_p,
    system_prompt,
    user_prompt,
    resizeHandleStyle,
    computedContainerStyle,
    run,
    onResize,
    handleTextareaMouseEnter,
    handleTextareaMouseLeave,
    sendToCodeEditor,
    modelOptions,
    isLoadingModel, // Expose loading state
    fetchLlamaServerModel, // Expose fetch function
  };
}
