import { defineConfig, loadEnv } from "vite";
import vue from "@vitejs/plugin-vue";
import vueJsx from "@vitejs/plugin-vue-jsx";
import path from "node:path";
// Ensure global crypto is available for tools that use getRandomValues at build time
import { webcrypto as nodeCrypto } from "node:crypto";
// In Node 20+, globalThis.crypto is a getter-only on some runtimes; avoid direct assignment
try {
  const g: any = globalThis as any;
  if (!g.crypto) {
    // Define a non-configurable property only if not present
    Object.defineProperty(g, "crypto", {
      value: nodeCrypto,
      writable: false,
      configurable: false,
      enumerable: false,
    });
  }
} catch (_) {
  // Fallback: leave as-is; most libs directly import from node:crypto in build
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const proxyTarget = env.VITE_DEV_SERVER_PROXY;
  const isTest =
    mode === "test" ||
    process.env.VITEST === "true" ||
    process.env.VITEST === "1";

  const alias: Record<string, string> = {
    "@": path.resolve(__dirname, "src"),
  };
  if (isTest) {
    alias["vue3-grid-layout-next/dist/style.css"] = path.resolve(
      __dirname,
      "tests/styleMock.css",
    );
  }

  return {
    plugins: [vue(), vueJsx()],
    resolve: {
      alias,
    },
    server: proxyTarget
      ? {
          proxy: {
            "/api": {
              target: proxyTarget,
              changeOrigin: true,
              secure: false,
            },
            "/stt": {
              target: proxyTarget,
              changeOrigin: true,
              secure: false,
            },
            "/audio": {
              target: proxyTarget,
              changeOrigin: true,
              secure: false,
            },
            "/auth": {
              target: proxyTarget,
              changeOrigin: true,
              secure: false,
            },
          },
        }
      : undefined,
    test: {
      environment: "jsdom",
      globals: true,
      setupFiles: "./tests/setupTests.ts",
      include: ["tests/**/*.spec.ts", "tests/**/*.test.ts"],
      exclude: ["e2e/**", "node_modules/**", "dist/**"],
      css: true,
      deps: {
        inline: [/vue3-grid-layout-next/],
      },
    },
  };
});
