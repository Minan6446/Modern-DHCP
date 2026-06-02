import { createApp } from 'vue'
import { watch } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import './style.css'
import App from './App.vue'
import i18n, { setI18nLanguage } from './i18n'
import router from './router'
import { useAppStore } from './stores/app'

const app = createApp(App)
const pinia = createPinia()
const appStore = useAppStore(pinia)

Object.entries(ElementPlusIconsVue).forEach(([key, component]) => {
	app.component(key, component)
})

watch(
	() => appStore.systemConfig.language,
	(language) => {
		setI18nLanguage(language)
	},
	{ immediate: true },
)

// Drop any stale theme override that earlier builds may have written
// into localStorage. The system theme is now fixed to the values defined
// in style.css; any inline overrides on documentElement are cleared so
// the page falls back to the stylesheet defaults.
localStorage.removeItem('modern-dns-theme-color')
;(['--app-accent', '--app-accent-soft', '--app-accent-muted'] as const).forEach((p) => {
	document.documentElement.style.removeProperty(p)
})

app.use(ElementPlus)
app.use(pinia)
app.use(i18n)
app.use(router)
app.mount('#app')
