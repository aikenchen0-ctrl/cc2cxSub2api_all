import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import './style.css'

const savedTheme = sessionStorage.getItem('agentapi_theme')
document.documentElement.classList.toggle('dark', savedTheme === 'dark')

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.mount('#app')
