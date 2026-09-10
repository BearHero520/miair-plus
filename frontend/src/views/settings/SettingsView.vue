<template>
 <n-space vertical :size="16">
  <n-card title="投送与连接"><n-spin :show="loading"><n-form label-placement="top" :show-feedback="false"><n-space vertical :size="20">
   <n-alert v-if="!form.ffmpeg_available" type="warning">音频组件未就绪，请检查安装包是否完整，或清空自定义路径后重试。</n-alert>
   <n-form-item label="NAS 局域网 IPv4"><n-input v-model:value="form.hostname" placeholder="留空自动选择，例如 192.168.1.10"/></n-form-item>
   <n-form-item label="DLNA 音频端口"><n-input-number v-model:value="form.dlna_port" :min="1024" :max="65535"/></n-form-item>
   <n-form-item label="AirPlay 音频接收"><n-switch v-model:value="form.airplay_enabled"/></n-form-item>
   <template v-if="form.airplay_enabled">
    <n-form-item label="使用 AirPlay 2 接收组件（预览）"><n-switch v-model:value="form.airplay2_enabled"/></n-form-item>
    <template v-if="form.airplay2_enabled">
     <n-form-item label="AirPlay 2 目标音箱"><n-select v-model:value="form.airplay2_target" :options="speakerOptions"/></n-form-item>
     <n-alert :type="form.airplay2_error ? 'warning' : form.airplay2_running ? 'success' : 'info'">{{ form.airplay2_error || (form.airplay2_running ? '原生接收组件已就绪，等待手机投送' : '保存后在后台启动接收组件') }}</n-alert>
     <n-text depth="3">一个 AirPlay 2 入口对应所选音箱，其余音箱保留传统 AirPlay。所有音箱的 DLNA 独立可用。切换目标或名称会重建 AirPlay 2 连接；不支持屏幕镜像，跨小米音箱的同步多房间播放不作保证。</n-text>
    </template>
    <n-text v-else depth="3">传统 AirPlay 支持 RAOP / ALAC、L16 音频。启用 AirPlay 2 可使用安装包内的 Shairport Sync 接收组件。</n-text>
   </template>
   <div class="component-status"><span><strong>FFmpeg 音频组件</strong><small>{{form.ffmpeg_available ? (form.ffmpeg_source==='bundled'?'正在使用安装包内置组件':'正在使用系统或自定义组件') : '组件不可用'}}</small></span><n-tag :type="form.ffmpeg_available?'success':'warning'" size="small">{{form.ffmpeg_available?'已就绪':'待检查'}}</n-tag></div>
   <n-collapse><n-collapse-item title="高级：自定义 FFmpeg" name="ffmpeg"><n-form-item label="可执行文件路径"><n-input v-model:value="form.ffmpeg_path" placeholder="留空优先使用内置 FFmpeg"/></n-form-item><n-text depth="3">一般无需修改。自定义路径无效时，请清空并保存。</n-text></n-collapse-item></n-collapse>
   <n-form-item label="收到音频地址后自动播放"><n-switch v-model:value="form.auto_play_on_set_uri"/></n-form-item>
   <n-form-item label="发现服务失败后自动重试"><n-switch v-model:value="form.auto_restart"/></n-form-item>
   <n-form-item label="初始音量"><n-input-number v-model:value="form.default_volume" :min="0" :max="100"/></n-form-item>
   <n-form-item label="小米播放接口 audioID"><n-input v-model:value="form.default_audio_id"/></n-form-item>
   <n-form-item label="外观"><n-radio-group v-model:value="app.theme"><n-radio-button value="auto">跟随系统</n-radio-button><n-radio-button value="light">浅色</n-radio-button><n-radio-button value="dark">深色</n-radio-button></n-radio-group></n-form-item>
   <n-space><n-button type="primary" :loading="saving" @click="save">保存设置</n-button><n-button :loading="loading" @click="load">重新读取</n-button></n-space>
  </n-space></n-form></n-spin></n-card>

 </n-space>
</template>
<script setup lang="ts">
import {computed,onMounted,onUnmounted,reactive,ref} from 'vue'
import {NSpace,NCard,NSpin,NForm,NFormItem,NInput,NInputNumber,NSwitch,NSelect,NButton,NText,NAlert,NRadioGroup,NRadioButton,NTag,NCollapse,NCollapseItem,useMessage} from 'naive-ui'
import {fetchSettings,saveSettings} from '@/api/system'
import {useAppStore} from '@/stores/app'
const app=useAppStore(),message=useMessage(),loading=ref(false),saving=ref(false)
const form=reactive({hostname:'',dlna_port:8311,airplay_enabled:true,airplay2_enabled:false,airplay2_target:'',airplay2_running:false,airplay2_error:'',speakers:{} as Record<string,{enabled:boolean;name:string;dlna_name:string}>,ffmpeg_path:'',ffmpeg_available:false,ffmpeg_source:'missing',ffmpeg_resolved:'',auto_play_on_set_uri:true,auto_restart:true,default_volume:40,default_audio_id:'',version:'',engine_version:''})
const speakerOptions=computed(()=>[{label:'自动选择第一台已启用音箱',value:''},...Object.entries(form.speakers).filter(([,s])=>s.enabled).map(([did,s])=>({label:s.dlna_name||s.name,value:did}))])
async function load(){loading.value=true;try{Object.assign(form,await fetchSettings())}catch(e:any){message.error(e.response?.data?.detail||'读取失败')}finally{loading.value=false}}
async function save(){saving.value=true;try{const {speakers,...patch}=form;await saveSettings(patch);message.success('已保存，后台正在应用设置')}catch(e:any){message.error(e.response?.data?.detail||'保存失败')}finally{saving.value=false}}
onMounted(load)
const timer=setInterval(async()=>{try{const s=await fetchSettings();form.airplay2_running=!!s.airplay2_running;form.airplay2_error=s.airplay2_error||''}catch{}},5000)
onUnmounted(()=>clearInterval(timer))
</script>
