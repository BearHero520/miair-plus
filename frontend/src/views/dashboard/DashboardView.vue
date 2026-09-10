<template>
 <div class="studio-dashboard">
  <div class="workspace-heading"><div><span class="eyebrow">HOME AUDIO / 总览</span><h1>声音，各就各位。</h1><p>你的音箱和投送状态，都在这里。</p></div><n-button secondary round :loading="loading" @click="refresh"><template #icon><n-icon><RefreshOutline/></n-icon></template>刷新</n-button></div>
  <n-alert v-if="error" type="warning">{{error}}</n-alert>
  <section class="signal-bar glass" aria-label="连接状态"><div><span class="signal-dot" :class="{live:connected && state?.dlna_running}"/><span>投送服务<strong>{{!connected ? '连接中断' : state?.dlna_running ? '正在运行' : '等待配置'}}</strong></span></div><div><span>已启用音箱<strong>{{connected ? state?.renderers_count ?? '—' : '—'}} <small>台</small></strong></span></div><div><span>正在播放<strong>{{connected ? playingCount : '—'}} <small>台</small></strong></span></div><router-link to="/account"><span>小米账号<strong>{{!system ? '读取中' : system.logged_in ? '已连接 ↗' : '连接账号 ↗'}}</strong></span></router-link></section>
  <div class="home-workspace">
   <section class="room-workspace"><div class="section-heading"><h2>音箱空间 <span class="count-tag">{{state?.speakers.length ?? 0}}</span></h2><router-link to="/devices">管理音箱 ↗</router-link></div>
    <div v-if="!state?.speakers.length" class="room-empty glass"><div class="room-drawing" aria-hidden="true"><span class="speaker-outline"><i/><i/><i/><i/><i/></span><span class="room-rings"/></div><span class="eyebrow">YOUR FIRST SPEAKER</span><h2>{{!connected ? '正在连接服务' : '给好声音，留一个位置'}}</h2><p>{{!connected ? '连接恢复后，会自动更新音箱状态。' : '连接小米账号，选择一台音箱。\n然后在音乐 App 中选择它，即可投送。'}}</p><n-button type="primary" round @click="router.push('/account')">{{system?.has_account ? '选择音箱' : '连接小米账号'}}<template #icon><n-icon><ArrowForwardOutline/></n-icon></template></n-button></div>
    <div v-else class="room-grid"><router-link v-for="speaker in state.speakers" :key="speaker.did" to="/devices" class="room-tile glass"><div class="tile-top"><span class="tile-icon"><n-icon :size="30"><VolumeHighOutline/></n-icon></span><span class="status-pill" :class="{live:connected && (speaker.transport_state === 'PLAYING' || speaker.airplay_active)}">{{!connected ? '连接中断' : speaker.transport_state === 'PLAYING' || speaker.airplay_active ? '播放中' : '待投送'}}</span></div><h3>{{speaker.dlna_name}}</h3><p>{{speaker.now_playing?.title || '还没有播放内容'}}</p><div class="tile-bottom"><span>{{speaker.airplay_active ? 'AirPlay' : 'DLNA'}}</span><span>管理音箱 →</span></div></router-link></div>
   </section>
   <aside class="listening-rail"><section class="current-panel glass"><div class="section-heading"><span class="eyebrow">此刻播放</span><n-icon><RadioOutline/></n-icon></div><div class="record-disc" :class="{active:!!current}" aria-hidden="true"><span>听</span></div><h2>{{current?.title || '等待下一首'}}</h2><p>{{current ? [current.artist,current.speakerName].filter(Boolean).join(' · ') : '从 QQ 音乐、网易云等 App\n发起 DLNA 音频投送。'}}</p><p class="cast-hint">播放、暂停和切歌，直接在音乐 App 中操作。</p></section><section class="connection-note"><span class="eyebrow">连接小提示</span><p>手机、音箱和 NAS 需在同一局域网。找不到设备时，先检查设置中的局域网 IP。</p><router-link to="/settings">检查连接设置 ↗</router-link></section></aside>
  </div>
  <div class="system-strip"><span>运行 {{uptime}}</span><span>内存 {{system?.memory_mb == null ? '—' : system.memory_mb.toFixed(1) + ' MB'}}</span><span>局域网 {{system?.hostname || '—'}}</span></div>
 </div>
</template>
<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { NAlert, NButton, NIcon } from 'naive-ui'
import { RefreshOutline, VolumeHighOutline, ArrowForwardOutline, RadioOutline } from '@vicons/ionicons5'
import { useWebSocket } from '@/composables/useWebSocket'
import { fetchStatus, type SystemStatus } from '@/api/system'
const router = useRouter()
const { connected, status: state } = useWebSocket()
const system = ref<SystemStatus | null>(null)
const loading = ref(false)
const error = ref('')
const playingCount = computed(() => state.value?.speakers.filter(s => s.transport_state === 'PLAYING' || s.airplay_active).length ?? 0)
const current = computed(() => { if (!connected.value) return null; const sp = state.value?.speakers.find(s => s.now_playing?.playing); return sp?.now_playing ? { ...sp.now_playing, speakerName: sp.dlna_name } : null })
const uptime = computed(() => { const s = system.value?.uptime_seconds; return s == null ? '—' : s < 60 ? '不到 1 分钟' : s < 3600 ? `${Math.floor(s / 60)} 分钟` : `${Math.floor(s / 3600)} 小时 ${Math.floor(s % 3600 / 60)} 分钟` })
async function refresh() { if (loading.value) return; loading.value = true; try { system.value = await fetchStatus(); error.value = '' } catch { error.value = '暂时无法读取服务状态，请检查连接后重试。' } finally { loading.value = false } }
let timer: ReturnType<typeof setInterval>
onMounted(() => { refresh(); timer = setInterval(refresh, 30000) })
onUnmounted(() => clearInterval(timer))
</script>
