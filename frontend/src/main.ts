import { createApp } from 'vue'
import { createPinia } from 'pinia'

import App from './App.vue'
import './styles/theme.css'
import './styles/art-themes.css'
import './styles/base.css'

createApp(App).use(createPinia()).mount('#app')
