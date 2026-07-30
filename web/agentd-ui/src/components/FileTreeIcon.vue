<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  name: string;
  isDir: boolean;
  expanded?: boolean;
}>();

const ext = computed(() => {
  const m = props.name.toLowerCase().match(/\.([a-z0-9]+)$/);
  return m ? m[1] : "";
});

type Kind = "dir" | "code" | "data" | "md" | "image" | "file";

const kind = computed<Kind>(() => {
  if (props.isDir) return "dir";
  const e = ext.value;
  if (["png", "jpg", "jpeg", "gif", "svg", "webp", "bmp", "ico", "avif"].includes(e))
    return "image";
  if (
    [
      "js", "ts", "jsx", "tsx", "vue", "go", "py", "java", "c", "cpp", "cc",
      "h", "hpp", "rs", "rb", "php", "swift", "kt", "scala", "sh", "bash", "zsh",
    ].includes(e)
  )
    return "code";
  if (["json", "yaml", "yml", "toml", "ini", "xml", "csv", "tsv"].includes(e))
    return "data";
  if (["md", "markdown", "mdx"].includes(e)) return "md";
  return "file";
});
</script>

<template>
  <svg
    class="shrink-0"
    :class="isDir ? 'text-accent/85' : 'text-faint-foreground'"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.6"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <!-- Folder -->
    <template v-if="kind === 'dir'">
      <template v-if="expanded">
        <path d="M4 8V6a1 1 0 0 1 1-1h3.6a1 1 0 0 1 .7.3L11 7h7a1 1 0 0 1 1 1v1" />
        <path d="M3.4 9.5A1 1 0 0 1 4.4 8.3h15.3a1 1 0 0 1 1 1.2l-1.3 7a1 1 0 0 1-1 .8H5.6a1 1 0 0 1-1-.8l-1.2-6.8z" />
      </template>
      <path v-else d="M3 7a1 1 0 0 1 1-1h4.6a1 1 0 0 1 .7.3L11 8h8a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1z" />
    </template>

    <!-- File base (folded-corner document) -->
    <template v-else>
      <path d="M13.5 3H7a1 1 0 0 0-1 1v16a1 1 0 0 0 1 1h10a1 1 0 0 0 1-1V7.5z" />
      <path d="M13.5 3v4.5H18" />

      <!-- code: < > -->
      <template v-if="kind === 'code'">
        <path d="M10.5 12 9 13.75 10.5 15.5" />
        <path d="M13.5 12 15 13.75 13.5 15.5" />
      </template>
      <!-- data: three dots -->
      <template v-else-if="kind === 'data'">
        <path d="M9.5 14h.01M12 14h.01M14.5 14h.01" stroke-width="2" />
      </template>
      <!-- markdown: two text lines -->
      <template v-else-if="kind === 'md'">
        <path d="M9 12.5h6M9 15.5h3.5" />
      </template>
    </template>

    <!-- Image (standalone) -->
    <template v-if="kind === 'image'">
      <rect x="4" y="5" width="16" height="14" rx="1.5" />
      <circle cx="9" cy="10" r="1.4" />
      <path d="M4.5 17.5 9 13l3 2.5 3.5-4L20 17" />
    </template>
  </svg>
</template>
