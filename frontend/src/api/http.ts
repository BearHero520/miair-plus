import axios from 'axios'
import { useAuthStore } from '@/stores/auth'
import router from '@/router'
import { appURL, basePath } from '@/utils/basePath'

const http = axios.create({
  baseURL: appURL('api/v1'),
  timeout: 30000,
})

// 自动携带 token
http.interceptors.request.use((config) => {
  if (basePath !== '/') config.headers['X-Miair-Client'] = 'miair-plus'
  const auth = useAuthStore()
  if (auth.token) {
    config.headers[basePath === '/' ? 'Authorization' : 'X-Miair-Authorization'] = `Bearer ${auth.token}`
  }
  return config
})

// 401 统一踢回登录页（防并发重复踢出）
let isLoggingOut = false

http.interceptors.response.use(
  (resp) => {
    if (!String(resp.headers['content-type'] || '').includes('application/json')) {
      return Promise.reject(new Error('飞牛网关返回了异常响应，请关闭应用后从飞牛入口重新打开'))
    }
    // 请求成功时重置锁定标志
    isLoggingOut = false
    return resp
  },
  (error) => {
    if (error.response?.status === 401 && !isLoggingOut) {
      isLoggingOut = true
      const auth = useAuthStore()
      auth.logout()
      if (router.currentRoute.value.name !== 'login') {
        router.push({ name: 'login' })
      }
    }
    return Promise.reject(error)
  },
)

export default http
