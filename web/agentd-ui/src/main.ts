import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { VueQueryPlugin, QueryClient } from '@tanstack/vue-query'
import App from './App.vue'
import router from './router'
import './assets/tailwind.css'
import './assets/vueflow.css'

const app = createApp(App)

app.use(createPinia())
app.use(router)

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false
    }
  }
})

app.use(VueQueryPlugin, {
  queryClient
})

app.mount('#app')
