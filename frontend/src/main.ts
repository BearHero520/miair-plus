import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { initializeFnosHost } from './utils/fnos'

const app = createApp(App)
const boot = window as typeof window & { miairBoot?: { ready(): void; fail(error?: unknown): void; phase(value: string): void } }
boot.miairBoot?.phase('初始化路由与登录状态')
app.config.errorHandler = (error) => {
  console.error(error)
  boot.miairBoot?.fail(error)
}
router.onError(error => boot.miairBoot?.fail(error))
app.use(createPinia())
app.use(router)
router.isReady().then(() => {
  boot.miairBoot?.phase('渲染管理界面')
  app.mount('#app')
  boot.miairBoot?.ready()
  void initializeFnosHost()
}).catch(error => boot.miairBoot?.fail(error))
