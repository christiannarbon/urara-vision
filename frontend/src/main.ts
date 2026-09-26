import { createApp } from 'vue'
import { createPinia } from 'pinia'

import App from './App.vue'
import { createAppRouter, installAuthGuard } from './router'
// Imported for its side effect: resolving the locale and stamping it onto the document has to…
import './i18n'
import './styles/theme.css'
import './styles/art-themes.css'
import './styles/base.css'

const router = createAppRouter()
installAuthGuard(router)
createApp(App).use(createPinia()).use(router).mount('#app')
