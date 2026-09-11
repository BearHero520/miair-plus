<template>
  <n-card title="配置导入与导出">
    <n-space vertical :size="16">
      <n-text depth="3">备份已保存的应用设置、音箱偏好和闹钟规则。账号登录凭据、音乐文件和本机外观不包含在内。</n-text>
      <n-space><n-button :loading="exporting" :disabled="importing || disabled" @click="download">导出配置</n-button><n-button :loading="importing" :disabled="exporting || disabled" @click="input?.click()">导入配置</n-button></n-space>
      <input ref="input" type="file" accept=".json,application/json" hidden @change="choose"/>
      <n-alert v-if="result" type="success">{{result}}</n-alert>
      <n-alert v-if="error" type="error">{{error}}</n-alert>
    </n-space>
    <n-modal :show="!!pending" preset="dialog" title="确认导入配置" positive-text="覆盖并导入" negative-text="取消" :loading="importing" :mask-closable="!importing" :closable="!importing" @positive-click="restore" @negative-click="cancel" @close="cancel" @update:show="v => { if (!v) cancel() }">
      <p>文件：{{filename}}</p><p>来源版本：{{pending?.app_version}} · {{pending?.alarms.length}} 个闹钟</p>
      <p>将覆盖已保存的应用设置和全部闹钟，并恢复已连接音箱的偏好；未连接的音箱偏好会跳过。</p>
      <p>导入后闹钟全部关闭。请检查音箱，重新选择铃声并保存后再启用。建议先导出当前配置。</p>
    </n-modal>
  </n-card>
</template>
<script setup lang="ts">
import {ref} from 'vue'
import {NCard, NSpace, NText, NButton, NAlert, NModal} from 'naive-ui'
import {exportConfig, importConfig, type ConfigBackup} from '@/api/system'
defineProps<{disabled?: boolean}>()
const emit = defineEmits<{imported: []}>()
const input = ref<HTMLInputElement>(), exporting = ref(false), importing = ref(false)
const pending = ref<ConfigBackup | null>(null), filename = ref(''), error = ref(''), result = ref('')
function reason(e: any) { return e.response?.data?.detail || e.message || '操作失败，请重试' }
function cancel() { if (!importing.value) pending.value = null }
async function download() {
  exporting.value = true; error.value = ''; result.value = ''
  try { await exportConfig() } catch (e) { error.value = reason(e) }
  finally { exporting.value = false }
}
async function choose(event: Event) {
  const target = event.target as HTMLInputElement, file = target.files?.[0]
  target.value = ''; error.value = ''; result.value = ''
  if (!file) return
  try {
    if (file.size > 1024 * 1024) throw Error('配置文件不能超过 1 MB')
    const data = JSON.parse(await file.text())
    if (data?.format !== 'miair-plus' || data.schema_version !== 1 || !data.settings || Array.isArray(data.settings) || !Array.isArray(data.alarms)) throw Error('请选择 MiAir Plus 导出的版本 1 JSON 配置文件')
    filename.value = file.name; pending.value = data
  } catch (e) { error.value = e instanceof SyntaxError ? 'JSON 格式错误，请选择完整的配置文件' : reason(e) }
}
async function restore() {
  if (!pending.value || importing.value) return false
  importing.value = true; error.value = ''
  try {
    const response = await importConfig(pending.value)
    result.value = response.message + (response.skipped_speakers ? ` 已跳过 ${response.skipped_speakers} 台未连接音箱。` : '')
    pending.value = null; emit('imported')
  } catch (e) { error.value = reason(e); pending.value = null }
  finally { importing.value = false }
  return false
}
</script>
