<template>
 <n-space vertical :size="16">
  <n-card title="投送与连接"><n-spin :show="loading"><n-form label-placement="top" :show-feedback="false"><n-space vertical :size="20">
   <n-alert v-if="!form.ffmpeg_available" type="warning">未检测到 FFmpeg。DLNA 可用；AirPlay 和拖动进度需要安装 FFmpeg 并填写路径。</n-alert>
   <n-form-item label="NAS 局域网 IPv4"><n-input v-model:value="form.hostname" placeholder="留空自动选择，例如 192.168.1.10"/></n-form-item>
   <n-form-item label="DLNA 音频端口"><n-input-number v-model:value="form.dlna_port" :min="1024" :max="65535"/></n-form-item>
   <n-form-item label="AirPlay 1 音频接收"><n-switch v-model:value="form.airplay_enabled"/></n-form-item>
   <n-text depth="3">支持传统 RAOP / ALAC、L16 音频。AirPlay 2、屏幕镜像与 FairPlay 加密流暂不支持。</n-text>
   <n-form-item label="FFmpeg 路径"><n-input v-model:value="form.ffmpeg_path" placeholder="留空从系统查找，例如 /usr/bin/ffmpeg"/></n-form-item>
   <n-form-item label="收到音频地址后自动播放"><n-switch v-model:value="form.auto_play_on_set_uri"/></n-form-item>
   <n-form-item label="发现服务失败后自动重试"><n-switch v-model:value="form.auto_restart"/></n-form-item>
   <n-form-item label="初始音量"><n-input-number v-model:value="form.default_volume" :min="0" :max="100"/></n-form-item>
   <n-form-item label="小米播放接口 audioID"><n-input v-model:value="form.default_audio_id"/></n-form-item>
   <n-form-item label="外观"><n-radio-group v-model:value="app.theme"><n-radio-button value="auto">跟随系统</n-radio-button><n-radio-button value="light">浅色</n-radio-button><n-radio-button value="dark">深色</n-radio-button></n-radio-group></n-form-item>
   <n-space><n-button type="primary" :loading="saving" @click="save">保存设置</n-button><n-button :loading="loading" @click="load">重新读取</n-button></n-space>
  </n-space></n-form></n-spin></n-card>
  <n-card title="MiAir Plus"><n-text>Go 原生预览版 {{ form.version }} · {{ form.engine_version }}</n-text><p>无需 Python 或 Docker。修改网络地址和端口会重新建立投送服务；修改名称在后台更新广播。</p><a href="https://github.com/BearHero520/miair-plus" target="_blank" rel="noopener noreferrer">项目与使用说明 ↗</a></n-card>
 </n-space>
</template>
<script setup lang="ts">
import {onMounted,reactive,ref} from 'vue'
import {NSpace,NCard,NSpin,NForm,NFormItem,NInput,NInputNumber,NSwitch,NButton,NText,NAlert,NRadioGroup,NRadioButton,useMessage} from 'naive-ui'
import {fetchSettings,saveSettings} from '@/api/system'
import {useAppStore} from '@/stores/app'
const app=useAppStore(),message=useMessage(),loading=ref(false),saving=ref(false)
const form=reactive({hostname:'',dlna_port:8311,airplay_enabled:true,ffmpeg_path:'',ffmpeg_available:false,auto_play_on_set_uri:true,auto_restart:true,default_volume:40,default_audio_id:'',version:'',engine_version:''})
async function load(){loading.value=true;try{Object.assign(form,await fetchSettings())}catch(e:any){message.error(e.response?.data?.detail||'读取失败')}finally{loading.value=false}}
async function save(){saving.value=true;try{await saveSettings(form);message.success('已保存，后台正在应用设置')}catch(e:any){message.error(e.response?.data?.detail||'保存失败')}finally{saving.value=false}}
onMounted(load)
</script>
