import { defineConfig, loadEnv } from "vite";
import vue from "@vitejs/plugin-vue";
import path from "node:path";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const proxyTarget = env.VITE_DEV_SERVER_PROXY || "http://localhost:32180";
  return {
    plugins: [vue()],
    resolve: { alias: { "@": path.resolve(__dirname, "src") } },
    server: {
      port: 32181,
      proxy: {
        "/api": { target: proxyTarget, changeOrigin: true, secure: false },
        "/agent": { target: proxyTarget, changeOrigin: true, secure: false },
        "/auth": { target: proxyTarget, changeOrigin: true, secure: false },
        "/openapi.json": { target: proxyTarget, changeOrigin: true, secure: false },
        "/audio": { target: proxyTarget, changeOrigin: true, secure: false },
        "/stt": { target: proxyTarget, changeOrigin: true, secure: false }
      }
    },
    test: { environment: "jsdom", globals: true }
  };
});
