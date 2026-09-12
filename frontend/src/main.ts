import { createApp } from 'vue'
import { createPinia } from 'pinia'

import App from './App.vue'
// Imported for its side effect: resolving the locale and stamping it onto the document has to…
import './i18n'
import './styles/theme.css'
import './styles/art-themes.css'
import './styles/base.css'

createApp(App).use(createPinia()).mount('#app')
