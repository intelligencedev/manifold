# manifold

Manifold is an **experimental** platform for enabling long horizon workflow automation using teams of AI assistants. 

Manifold supports OpenAI models such as `gpt-5.2`, Google models such as `gemini-3-pro` and Anthropic models such as `claude-opus-4-5`. Manifold also supports OpenAI API compatible services for users that self host open weight models such as `gpt-oss-120b`, `devstral-2-123b` using [llama.cpp](https://github.com/ggml-org/llama.cpp) or [vllm](https://github.com/vllm-project/vllm).

Disclaimer: As an experimental frontier AI platform, we do not recommend Manifold be deployed in a production environment where stability is required until explicitly noted in this README. 

## Features

### **Agent Chat**
Use a traditional chat view to instruct specialists (agents) to work on objectives.

![chat](docs/img/chat.webp)

_Specialists can collaborate in multi-turn objectives. Manifold is designed to take advantage of the long horizon capabilities of frontier models and can work on complex objectives for hours._

### Image Generation

Manifold supports OpenAI and Google image generation models as well as local image generation using a custom ComfyUI MCP client.

![image generation](docs/img/imggen.webp)
_Example ComfyUI generated image with a custom workflow_

### **Specialist Registry**
Define and configure AI agents (specialists) and build your team of experts.

![specialists](docs/img/specialists.webp)

### **Projects**
Configure projects as agent workspaces.

![projects](docs/img/projects.webp)

### **Integrated tools and MCP Support**
Manifold implements internal tools for agent workflows as well as MCP support to extend the capabilities of your agents. Configure as many MCP servers as you wish. Enable tools individually to easily manage model context limits.

![specialists](docs/img/mcp.webp)

### **Workflow Editor**
Design agent workflows using a visual flow editor.

![specialists](docs/img/flow.webp)

### **Prompts, Datasets and Experiments Playground**
Create, iterate and version custom prompts that can be assigned to your agents. Configure datasets and run experiments to understand how prompts affect agent behaviors.

![specialists](docs/img/playground.webp)

## Quick Start

For step-by-step quick start instructions, see the repository Quick Start guide: [QUICKSTART.md](./QUICKSTART.md)
