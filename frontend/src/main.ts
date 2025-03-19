import './style.css'
import { createApp } from 'vue'
import App from './App.vue'
import { createPinia } from 'pinia'

const app = createApp(App)
const pinia = createPinia()

app.use(pinia) // Install Pinia into the Vue app

app.mount('#app')