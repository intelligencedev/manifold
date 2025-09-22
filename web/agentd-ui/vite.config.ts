import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueJsx from '@vitejs/plugin-vue-jsx'
import path from 'node:path'
// Ensure global crypto is available for tools that use getRandomValues at build time
import { webcrypto as nodeCrypto } from 'node:crypto'
// In Node 20+, globalThis.crypto is a getter-only on some runtimes; avoid direct assignment
try {
  const g: any = globalThis as any
  if (!g.crypto) {
    // Define a non-configurable property only if not present
    Object.defineProperty(g, 'crypto', {
      value: nodeCrypto,
      writable: false,
      configurable: false,
      enumerable: false
    })
  }
} catch (_) {
  // Fallback: leave as-is; most libs directly import from node:crypto in build
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const proxyTarget = env.VITE_DEV_SERVER_PROXY

  return {
    plugins: [vue(), vueJsx()],
    resolve: {
      alias: {
        '@': path.resolve(__dirname, 'src')
      }
    },
    server: proxyTarget
      ? {
          proxy: {
            '/api': {
              target: proxyTarget,
              changeOrigin: true,
              secure: false
            }
          }
        }
      : undefined,
    test: {
      environment: 'jsdom',
      globals: true,
      setupFiles: './tests/setupTests.ts'
    }
  }
})
