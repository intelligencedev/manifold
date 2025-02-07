// frontend/vite.config.ts
import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import { fileURLToPath, URL } from 'node:url'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  build: {
    outDir: 'dist',  // Output directory (make sure this matches your Go embed path)
    emptyOutDir: true, // Clean the output directory before building
    sourcemap: true, // Generate sourcemaps (good for debugging)
  },
  server: {
    proxy: {
      '/api': {  // Proxy API requests to the Go backend
        target: 'http://localhost:8080', // Your Go server address
        changeOrigin: true,
        // If you have any rewrite rules
        // rewrite: (path) => path.replace(/^\/api/, ''),
      },
    },
  },
});