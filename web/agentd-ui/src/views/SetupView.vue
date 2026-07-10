<template>
  <div
    class="mx-auto flex min-h-screen max-w-xl flex-col justify-center gap-8 px-6 py-12"
  >
    <div class="space-y-2">
      <p
        class="font-mono text-[11px] uppercase tracking-[0.16em] text-faint-foreground"
      >
        Manifold
      </p>
      <h1 class="text-3xl font-semibold text-foreground">Welcome</h1>
      <p class="text-sm text-subtle-foreground">
        Connect an LLM provider to start. Credentials stay on this machine in
        your local config.
      </p>
    </div>

    <div
      v-if="loadError"
      class="rounded-md border border-danger/60 bg-danger/10 p-3 text-sm text-danger-foreground"
    >
      {{ loadError }}
    </div>

    <form class="space-y-5" @submit.prevent="submit">
      <div class="space-y-1">
        <label
          for="provider"
          class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
          >Provider</label
        >
        <select
          id="provider"
          v-model="form.provider"
          class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
        >
          <option value="openai">OpenAI / compatible</option>
          <option value="anthropic">Anthropic</option>
          <option value="google">Google</option>
          <option value="openrouter">OpenRouter</option>
          <option value="llamacpp">llama.cpp (OpenAI-compatible)</option>
          <option value="local">Local (OpenAI-compatible)</option>
        </select>
      </div>

      <div class="space-y-1">
        <label
          for="api-key"
          class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
          >API key</label
        >
        <input
          id="api-key"
          v-model="form.apiKey"
          type="password"
          autocomplete="off"
          :placeholder="
            form.provider === 'local' || form.provider === 'llamacpp'
              ? 'Optional for most local servers'
              : 'sk-…'
          "
          class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
        />
      </div>

      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div class="space-y-1">
          <label
            for="model"
            class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
            >Model</label
          >
          <input
            id="model"
            v-model="form.model"
            type="text"
            :placeholder="modelPlaceholder"
            class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
          />
        </div>
        <div class="space-y-1">
          <label
            for="base-url"
            class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
            >Base URL</label
          >
          <input
            id="base-url"
            v-model="form.baseUrl"
            type="url"
            :placeholder="baseUrlPlaceholder"
            class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
          />
        </div>
      </div>

      <p class="text-xs text-subtle-foreground">
        Memory stays off by default. Enable it later in Settings — embeddings
        only apply once memory is turned on.
      </p>

      <div
        v-if="saveError"
        class="rounded-md border border-danger/60 bg-danger/10 p-3 text-sm text-danger-foreground"
      >
        {{ saveError }}
      </div>

      <button
        type="submit"
        class="w-full rounded bg-accent px-4 py-2.5 text-sm font-semibold text-accent-foreground hover:bg-accent/90 disabled:opacity-60"
        :disabled="saving"
      >
        {{ saving ? "Saving…" : "Continue" }}
      </button>
    </form>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useRouter } from "vue-router";
import {
  completeSetup,
  fetchSetupStatus,
  type SetupCompleteRequest,
} from "@/api/client";

const router = useRouter();
const saving = ref(false);
const loadError = ref("");
const saveError = ref("");

const form = reactive({
  provider: "openai",
  apiKey: "",
  model: "",
  baseUrl: "",
});

const modelPlaceholder = computed(() => {
  switch (form.provider) {
    case "anthropic":
      return "claude-sonnet-4-6";
    case "google":
      return "gemini-2.5-pro";
    case "openrouter":
      return "openai/gpt-4o-mini";
    case "local":
    case "llamacpp":
      return "local-model";
    default:
      return "gpt-5-mini";
  }
});

const baseUrlPlaceholder = computed(() => {
  switch (form.provider) {
    case "anthropic":
      return "https://api.anthropic.com";
    case "google":
      return "https://generativelanguage.googleapis.com/";
    case "openrouter":
      return "https://openrouter.ai/api";
    case "llamacpp":
      return "http://localhost:8080/v1 (required)";
    case "local":
      return "http://localhost:11434/v1";
    default:
      return "https://api.openai.com/v1";
  }
});

onMounted(async () => {
  try {
    const status = await fetchSetupStatus();
    if (status.ready) {
      await router.replace({ name: "chat" });
      return;
    }
    if (status.provider) form.provider = status.provider;
    if (status.model) form.model = status.model;
  } catch (error: any) {
    loadError.value =
      error?.response?.data?.error ||
      error?.message ||
      "Unable to load setup status";
  }
});

async function submit() {
  if (saving.value) return;
  saving.value = true;
  saveError.value = "";
  try {
    const payload: SetupCompleteRequest = {
      provider: form.provider,
      apiKey: form.apiKey.trim(),
      model: form.model.trim() || undefined,
      baseUrl: form.baseUrl.trim() || undefined,
    };
    const result = await completeSetup(payload);
    if (!result.ready) {
      saveError.value = "Setup did not complete. Check the API key and try again.";
      return;
    }
    await router.replace({ name: "chat" });
  } catch (error: any) {
    saveError.value =
      error?.response?.data?.error ||
      error?.message ||
      "Failed to save setup";
  } finally {
    saving.value = false;
  }
}
</script>
