import { defineConfig } from "@playwright/test";

export default defineConfig({
  use: { baseURL: "http://127.0.0.1:32181" },
  testDir: "e2e",
});
