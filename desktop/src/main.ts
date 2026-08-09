import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { initClientLog } from '@/services/clientLog'
import App from './App.vue'
import './styles/global.css'
import './styles/companion-ui.css'

void initClientLog()

createApp(App).use(createPinia()).mount('#app')
