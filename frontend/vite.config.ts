// frontend/vite.config.ts
import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import { fileURLToPath, URL } from 'node:url';
import path from 'path';

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
      // Ensure module resolution for dagre is unified
      dagre: path.resolve(__dirname, 'node_modules/@dagrejs/dagre'),
    },
  },
  optimizeDeps: {
    include: ['dagre', '@dagrejs/dagre'], // pre-bundle both names
    force: true, // Force re-optimization
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    sourcemap: true,
    rollupOptions: {
      output: {
        manualChunks(id) {
          // Include dagre in the main bundle
          if (id.includes('dagre') || id.includes('@dagrejs/dagre')) {
            return 'index'; // This puts it in the main chunk
          }
        }
      }
    }
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
});