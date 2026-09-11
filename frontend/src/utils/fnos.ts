import { TrimApp } from '@trimjs/web-app'
import { reactive } from 'vue'

// Initialize at entry, so the mobile bridge is registered before visiting a feature page.
// Host readiness must never block rendering or standalone browser access.
export const fnos = new TrimApp({ debug: false })
export const fnosHost = reactive({ connected: false, theme: '' as '' | 'dark' | 'light', language: '', systemVersion: '', appVersion: '', error: '' })

export async function refreshFnosHost() {
  if (fnos.isStandaloneWeb) return
  try {
    await waitForFnos()
    const config = await withFnosTimeout(fnos.getPlatformConfig())
    fnosHost.connected = true
    fnosHost.theme = config.theme
    fnosHost.language = config.language
    fnosHost.systemVersion = config.systemVersion
    fnosHost.appVersion = config.appVersion || ''
    fnosHost.error = ''
  } catch (e) { fnosHost.error = e instanceof Error ? e.message : '无法读取飞牛环境' }
}

export async function withFnosTimeout<T>(operation: Promise<T>): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined
  try {
    return await Promise.race([operation, new Promise<never>((_, reject) => {
      timer = setTimeout(() => reject(new Error('飞牛响应超时，请返回应用后重试')), 8000)
    })])
  } finally { clearTimeout(timer) }
}

export async function openFnosSettings() {
  if (fnos.isStandaloneWeb) throw new Error('请从飞牛应用中心进入 MiAir Plus 的设置页面')
  await waitForFnos()
  await withFnosTimeout(fnos.openAppSetting())
}

export async function initializeFnosHost() {
  await refreshFnosHost()
  if (fnosHost.connected && fnos.isWeb && !fnos.isStandaloneWeb) {
    await Promise.all([
      fnos.$on('os/theme', (theme: 'dark' | 'light') => { fnosHost.theme = theme }),
      fnos.$on('os/language', (language: string) => { fnosHost.language = language }),
    ]).catch(() => {})
  }
}
export async function waitForFnos() {
  let timer: ReturnType<typeof setTimeout> | undefined
  try {
    await Promise.race([
      fnos.ready(),
      new Promise<never>((_, reject) => {
        timer = setTimeout(() => reject(new Error('飞牛连接尚未就绪，请重新打开应用，或使用上传音乐')), 8000)
      }),
    ])
  } finally { clearTimeout(timer) }
}
