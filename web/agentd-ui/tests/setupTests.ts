import "@testing-library/jest-dom/vitest";
import { config } from "@vue/test-utils";
import { VueQueryPlugin, QueryClient } from "@tanstack/vue-query";
import { createPinia, setActivePinia } from "pinia";
import { createMemoryHistory, createRouter } from "vue-router";

const pinia = createPinia();
setActivePinia(pinia);

const router = createRouter({
  history: createMemoryHistory(),
  routes: [{ path: "/", component: { template: "<div />" } }],
});

const queryClient = new QueryClient();

if (!globalThis.ResizeObserver) {
  globalThis.ResizeObserver = class ResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as typeof ResizeObserver;
}

if (!HTMLElement.prototype.scrollTo) {
  HTMLElement.prototype.scrollTo = () => {};
}

function createStorageMock(): Storage {
  const store = new Map<string, string>();
  return {
    get length() {
      return store.size;
    },
    clear() {
      store.clear();
    },
    getItem(key: string) {
      return store.has(key) ? store.get(key)! : null;
    },
    key(index: number) {
      return Array.from(store.keys())[index] ?? null;
    },
    removeItem(key: string) {
      store.delete(key);
    },
    setItem(key: string, value: string) {
      store.set(key, String(value));
    },
  } as Storage;
}

if (
  !globalThis.localStorage ||
  typeof globalThis.localStorage.clear !== "function"
) {
  Object.defineProperty(globalThis, "localStorage", {
    value: createStorageMock(),
    configurable: true,
  });
}

if (
  !globalThis.sessionStorage ||
  typeof globalThis.sessionStorage.clear !== "function"
) {
  Object.defineProperty(globalThis, "sessionStorage", {
    value: createStorageMock(),
    configurable: true,
  });
}

void router.push("/");

config.global.plugins = [
  ...(config.global.plugins ?? []),
  pinia,
  router,
  [VueQueryPlugin, { queryClient }],
];
