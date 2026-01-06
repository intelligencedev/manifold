import { createApp } from "vue";
import { createPinia } from "pinia";
import { VueQueryPlugin, QueryClient } from "@tanstack/vue-query";
import App from "./App.vue";
import router from "./router";
import "./assets/tailwind.css";
import "./assets/vueflow.css";
import "@vue-flow/node-resizer/dist/style.css";
import "./assets/aperture.css";
import { useThemeStore } from "@/stores/theme";

const app = createApp(App);

const pinia = createPinia();
app.use(pinia);
app.use(router);

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

app.use(VueQueryPlugin, {
  queryClient,
});

useThemeStore();

app.mount("#app");
