import {defineStore} from 'pinia'
import {ref} from 'vue'
import {checkUpdate, fetchSettings, saveSettings, type UpdateInfo} from '@/api/system'

export const useUpdateStore = defineStore('updates', () => {
  const info = ref<UpdateInfo | null>(null)
  const checking = ref(false), saving = ref(false), enabled = ref(true)
  async function check(force = false) {
    if (checking.value) return
    checking.value = true
    try { info.value = await checkUpdate(force) }
    catch (e: any) {
      info.value = {current: info.value?.current || '', latest: null, update_available: false,
        error: e.response?.data?.detail || '检测失败，请检查网络后重试'}
    } finally { checking.value = false }
  }
  async function automatic() {
    try {
      enabled.value = (await fetchSettings()).auto_check_update
      if (enabled.value) await check()
    } catch { /* Retry on the next automatic check; manual checks expose errors. */ }
  }
  async function setEnabled(value: boolean) {
    saving.value = true
    try { await saveSettings({auto_check_update: value}); enabled.value = value }
    finally { saving.value = false }
    if (value) void check()
  }
  return {info, checking, saving, enabled, check, automatic, setEnabled}
})
