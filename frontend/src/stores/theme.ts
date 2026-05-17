import { defineStore } from "pinia";
import { ref, watch } from "vue";

type ThemeId = "cockpit" | "garden" | "theatre";

export const useThemeStore = defineStore("cockpit-theme", () => {
  const STORAGE_KEY = "cockpit.theme";

  const stored = typeof localStorage !== "undefined" ? (localStorage.getItem(STORAGE_KEY) as ThemeId | null) : null;
  const theme = ref<ThemeId>(stored ?? "cockpit");

  watch(
    theme,
    (value) => {
      document.documentElement.dataset.cockpitTheme = value;
      if (typeof localStorage !== "undefined") localStorage.setItem(STORAGE_KEY, value);
    },
    { immediate: true }
  );

  return { theme };
});
