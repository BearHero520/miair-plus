<template>
 <n-space vertical :size="16">
  <n-card title="小米账号"><n-space justify="space-between" align="center"><n-text>{{ status.logged_in ? '已连接 · '+status.user_id : '尚未连接' }}</n-text><n-button v-if="status.has_account" type="error" secondary :loading="busy" @click="remove">断开账号</n-button></n-space><p>使用小米账号授权音箱播放，凭据只保存到 NAS 本地。不会保存账号密码。</p></n-card>
  <n-card title="连接账号"><n-tabs type="segment">
   <n-tab-pane name="qr" tab="米家扫码"><n-space vertical align="center"><n-button :loading="busy" type="primary" @click="start">{{ qr ? '重新获取二维码' : '获取二维码' }}</n-button><img v-if="qr" :src="qr" alt="米家登录二维码" width="200" height="200"/><n-text>{{ qrMessage || '使用米家 App 扫码并确认登录' }}</n-text></n-space></n-tab-pane>
   <n-tab-pane name="password" tab="账号密码"><n-form label-placement="top"><n-form-item label="小米账号"><n-input v-model:value="username" autocomplete="username"/></n-form-item><n-form-item label="密码"><n-input v-model:value="password" type="password" show-password-on="click" autocomplete="current-password"/></n-form-item></n-form><n-button :loading="busy" type="primary" @click="login">连接</n-button><p>遇到验证码或二次验证时，请改用米家扫码。</p></n-tab-pane>
   <n-tab-pane name="token" tab="手动令牌"><n-form label-placement="top"><n-form-item label="User ID"><n-input v-model:value="userId"/></n-form-item><n-form-item label="Pass Token"><n-input v-model:value="passToken" type="password" show-password-on="click"/></n-form-item></n-form><n-button :loading="busy" type="primary" @click="tokenLogin">验证并连接</n-button></n-tab-pane>
  </n-tabs></n-card>
  <n-card title="选择音箱"><n-space vertical :size="16"><n-button :loading="devicesLoading" :disabled="!status.has_account" @click="loadDevices">刷新设备列表</n-button><n-alert v-if="deviceError" type="warning">{{deviceError}}</n-alert><n-empty v-if="!devices.length" description="连接账号后刷新音箱列表"/><n-checkbox-group v-model:value="selected"><n-space vertical><n-checkbox v-for="d in devices" :key="d.miotDID" :value="d.miotDID" :label="d.name+' · '+d.hardware"/></n-space></n-checkbox-group><n-button type="primary" :loading="saving" :disabled="!devices.length" @click="save">保存选择</n-button></n-space></n-card>
 </n-space>
</template>
<script setup lang="ts">
import {onMounted,onUnmounted,ref} from 'vue'
import {NSpace,NCard,NText,NButton,NTabs,NTabPane,NForm,NFormItem,NInput,NCheckboxGroup,NCheckbox,NEmpty,NAlert,useMessage,useDialog} from 'naive-ui'
import {fetchAccountStatus,startQRCode,pollQRCode,passwordLogin,setManualToken,deleteAccount,type AccountStatus} from '@/api/account'
import {fetchCloudDevices,type CloudDevice} from '@/api/speakers'
import {fetchSettings,saveSettings} from '@/api/system'
const message=useMessage(),dialog=useDialog(),busy=ref(false),saving=ref(false),devicesLoading=ref(false)
const status=ref<Partial<AccountStatus>>({}),username=ref(''),password=ref(''),userId=ref(''),passToken=ref(''),qr=ref(''),qrMessage=ref(''),deviceError=ref('')
const devices=ref<CloudDevice[]>([]),selected=ref<string[]>([])
let stopped=false,generation=0,timer:ReturnType<typeof setTimeout>|undefined
async function init(){status.value=await fetchAccountStatus();const s=await fetchSettings();selected.value=Object.values(s.speakers).filter(x=>x.enabled).map(x=>x.did)}
async function loadDevices(){devicesLoading.value=true;try{const r=await fetchCloudDevices();devices.value=r.devices;deviceError.value=r.error||''}catch(e:any){deviceError.value=e.response?.data?.detail||'读取设备失败'}finally{devicesLoading.value=false}}
async function connected(){await init();await loadDevices();message.success('小米账号已连接，请选择音箱并保存')}
async function start(){busy.value=true;const current=++generation;clearTimeout(timer);try{const r=await startQRCode();if(!r.success||!r.session_id)throw Error(r.error||'二维码获取失败');qr.value=r.qrcode_url||'';qrMessage.value='等待扫码确认';const id=r.session_id;const poll=async()=>{if(stopped||current!==generation)return;try{const result=await pollQRCode(id);if(stopped||current!==generation)return;qrMessage.value=result.message;if(result.state==='confirmed'){qr.value='';await connected();return}if(result.state!=='waiting')return}catch{qrMessage.value='连接暂时中断，正在重试'}if(!stopped&&current===generation)timer=setTimeout(poll,2000)};timer=setTimeout(poll,1500)}catch(e:any){message.error(e.message||'二维码获取失败')}finally{busy.value=false}}
async function login(){busy.value=true;try{const r=await passwordLogin(username.value,password.value);password.value='';if(!r.success)throw Error(r.error||r.message||'连接失败');await connected()}catch(e:any){message.error(e.message)}finally{busy.value=false}}
async function tokenLogin(){busy.value=true;try{const r=await setManualToken(userId.value,passToken.value);if(!r.success)throw Error(r.error||'令牌无效');passToken.value='';await connected()}catch(e:any){message.error(e.message)}finally{busy.value=false}}
async function save(){saving.value=true;try{const speakers=Object.fromEntries(devices.value.map(d=>[d.miotDID,{enabled:selected.value.includes(d.miotDID)}]));await saveSettings({speakers} as any);message.success('已保存，后台正在更新投送服务')}catch(e:any){message.error(e.response?.data?.detail||'保存失败')}finally{saving.value=false}}
function remove(){dialog.warning({title:'断开小米账号',content:'将移除本地凭据并停止已启用音箱的投送服务。',positiveText:'断开',negativeText:'取消',onPositiveClick:async()=>{busy.value=true;try{++generation;clearTimeout(timer);await deleteAccount();qr.value='';devices.value=[];await init()}catch(e:any){message.error(e.response?.data?.detail||'断开失败')}finally{busy.value=false}}})}
onMounted(async()=>{try{await init();if(status.value.has_account)await loadDevices()}catch{message.error('账号状态读取失败')}})
onUnmounted(()=>{stopped=true;++generation;clearTimeout(timer)})
</script>
