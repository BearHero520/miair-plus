<template>
 <div class="compact-home">
  <div class="workspace-heading"><div><h1>总览</h1><p>家里的音箱，随时可投送。</p></div><n-button secondary :loading="loading" aria-label="刷新总览" @click="refresh"><template #icon><n-icon><RefreshOutline/></n-icon></template>刷新</n-button></div>
  <n-alert v-if="error" type="warning" class="status-error">{{error}}</n-alert>
  <section class="overview-stats" aria-label="连接状态">
   <div class="glass overview-stat"><n-icon :size="22"><RadioOutline/></n-icon><span>投送服务<strong>{{!connected?'连接中':state?.dlna_running?'运行中':'等待配置'}}</strong></span></div>
   <router-link to="/devices" class="glass overview-stat"><n-icon :size="22"><VolumeHighOutline/></n-icon><span>已启用音箱<strong>{{connected?rooms.length:'—'}}<small> 台</small></strong></span></router-link>
   <router-link to="/account" class="glass overview-stat"><n-icon :size="22"><PersonOutline/></n-icon><span>小米账号<strong>{{!system?'读取中':system.logged_in?'已连接':'未连接'}}</strong></span></router-link>
  </section>
  <section class="glass overview-speakers"><div class="section-heading"><h2>我的音箱</h2><router-link to="/devices">管理音箱 →</router-link></div>
   <div v-if="!rooms.length" class="compact-empty"><n-icon :size="34"><VolumeHighOutline/></n-icon><h3>{{connected?'还没有启用音箱':'正在连接服务'}}</h3><p>连接账号后，选择要接收投送的音箱。</p><n-button type="primary" @click="router.push('/account')">{{system?.has_account?'选择音箱':'连接账号'}}</n-button></div>
   <router-link v-for="speaker in rooms" :key="speaker.did" to="/devices" class="overview-speaker"><span class="tile-icon"><n-icon :size="23"><VolumeHighOutline/></n-icon></span><span class="speaker-summary"><strong>{{speaker.dlna_name}}</strong><small>{{speaker.now_playing?.title||'等待投送'}}</small></span><span class="status-pill" :class="{live:connected&&speaker.transport_state==='PLAYING'}">{{!connected?'连接中断':speaker.transport_state==='PLAYING'?'播放中':'待投送'}}</span></router-link>
  </section>
  <p class="overview-foot">NAS {{system?.hostname||'—'}}<span v-if="system?.ffmpeg_available">音频组件已就绪</span></p>
 </div>
</template>
<script setup lang="ts">
import {computed,onMounted,onUnmounted,ref} from 'vue'
import {useRouter} from 'vue-router'
import {NAlert,NButton,NIcon} from 'naive-ui'
import {RefreshOutline,VolumeHighOutline,RadioOutline,PersonOutline} from '@vicons/ionicons5'
import {useWebSocket} from '@/composables/useWebSocket'
import {fetchStatus,type SystemStatus} from '@/api/system'
const router=useRouter(),{connected,status:state}=useWebSocket()
const system=ref<SystemStatus|null>(null),loading=ref(false),error=ref('')
const rooms=computed(()=>state.value?.speakers.filter(s=>s.enabled)||[])
async function refresh(){if(loading.value)return;loading.value=true;try{system.value=await fetchStatus();error.value=''}catch{error.value='读取状态失败，请检查服务连接。'}finally{loading.value=false}}
let timer:ReturnType<typeof setInterval>
onMounted(()=>{refresh();timer=setInterval(refresh,15000)})
onUnmounted(()=>clearInterval(timer))
</script>
