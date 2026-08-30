import { createApp } from 'vue'
import { createPinia } from 'pinia'

import App from './App.vue'
// Imported for its side effect: resolving the locale and stamping it onto the
// document has to happen before the first render, not on the first component
// that asks for a string.
import './i18n'
import './styles/theme.css'
import './styles/art-themes.css'
import './styles/base.css'

createApp(App).use(createPinia()).mount('#app')
