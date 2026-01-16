import * as matchers from "@testing-library/jest-dom/matchers";
import { config } from "@vue/test-utils";
import { VueQueryPlugin, QueryClient } from "@tanstack/vue-query";
import { createPinia, setActivePinia } from "pinia";
import { createMemoryHistory, createRouter } from "vue-router";
import { expect } from "vitest";

expect.extend(matchers);

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

void router.push("/");

config.global.plugins = [
	...(config.global.plugins ?? []),
	pinia,
	router,
	[VueQueryPlugin, { queryClient }],
];
