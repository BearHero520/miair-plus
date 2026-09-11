<template>
  <n-card title="软件更新">
    <n-space vertical :size="16">
      <div class="update-option"><div><strong>自动检测更新</strong><p>打开应用时及使用期间每小时检测正式版本。</p></div><n-switch :value="updates.enabled" :loading="updates.saving" aria-label="自动检测更新" @update:value="toggle"/></div>
      <n-text>当前版本 {{updates.info?.current || version}}</n-text>
      <n-alert v-if="updates.info" :type="updates.info.error ? 'warning' : updates.info.update_available ? 'info' : 'success'">
        {{updates.info.error || (updates.info.update_available ? `发现新版本 ${updates.info.latest}` : '当前已是最新版本')}}
      </n-alert>
      <n-text v-if="updates.info?.checked_at" depth="3">上次检测：{{new Date(updates.info.checked_at).toLocaleString()}}</n-text>
      <n-space><n-button :loading="updates.checking" @click="updates.check(true)">立即检测</n-button><n-button tag="a" :href="updates.info?.release_url || 'https://github.com/BearHero520/miair-plus/releases'" target="_blank" rel="noopener noreferrer">查看版本与下载</n-button></n-space>
      <n-collapse v-if="updates.info?.notes"><n-collapse-item title="更新说明" name="notes"><div class="release-notes">{{updates.info.notes}}</div></n-collapse-item></n-collapse>
    </n-space>
  </n-card>
</template>
<script setup lang="ts">
import {NCard, NSpace, NText, NSwitch, NAlert, NButton, NCollapse, NCollapseItem, useMessage} from 'naive-ui'
import {useUpdateStore} from '@/stores/updates'
import {version} from '../../package.json'
const updates = useUpdateStore(), message = useMessage()
async function toggle(value: boolean) {
  try { await updates.setEnabled(value) }
  catch (e: any) { message.error(e.response?.data?.detail || '保存自动检测设置失败') }
}
</script>
<style scoped>
.update-option{display:flex;align-items:center;justify-content:space-between;gap:20px}.update-option p{margin:6px 0 0;font-size:12px;color:var(--muted)}.release-notes{white-space:pre-wrap;overflow-wrap:anywhere;max-height:320px;overflow:auto}
</style>
