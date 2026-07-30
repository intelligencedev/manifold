export const llmProviderOptions = [
  { value: "openai", label: "OpenAI / compatible", backend: "openai" },
  { value: "anthropic", label: "Anthropic", backend: "anthropic" },
  { value: "google", label: "Google", backend: "google" },
  { value: "openrouter", label: "OpenRouter", backend: "openai" },
  {
    value: "llamacpp",
    label: "llama.cpp (OpenAI-compatible)",
    backend: "openai",
  },
  {
    value: "local",
    label: "Local (OpenAI-compatible)",
    backend: "openai",
  },
] as const;

export type LLMProvider = (typeof llmProviderOptions)[number]["value"];

export function llmProviderBackend(
  provider: string,
): "openai" | "anthropic" | "google" {
  return (
    llmProviderOptions.find((option) => option.value === provider)?.backend ??
    "openai"
  );
}
