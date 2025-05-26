import './assets/css/tailwindstyles.css'
import { createApp } from 'vue'
import Root from './Root.vue'
import { createPinia } from 'pinia'

const app = createApp(Root)
const pinia = createPinia()

app.use(pinia) // Install Pinia into the Vue app

app.mount('#app')
