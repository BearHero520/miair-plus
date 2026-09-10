<template>
 <div>
  <div class="device-toolbar"><n-input v-model:value="query" clearable placeholder="查找音箱名称或型号" aria-label="查找音箱"/><n-button :loading="loading" @click="load(false)">刷新</n-button><n-button type="primary" @click="router.push('/account')">添加音箱</n-button></div>
  <n-alert v-if="loadError" type="warning" class="status-error">{{loadError}}</n-alert>
  <div v-if="!speakers.length" class="room-empty glass"><span class="tile-icon"><n-icon :size="32"><VolumeHighOutline/></n-icon></span><h2>{{loading ? '正在读取音箱…' : '还没有启用的音箱'}}</h2><p>在账号页面选择设备后，就可以设置投送名称。</p><n-button round @click="router.push('/account')">前往选择音箱</n-button></div>
  <p v-else-if="!filtered.length">没有找到匹配的音箱，试试其他名称。</p>
  <div class="device-grid"><article v-for="row in filtered" :key="row.did" class="device-tile glass"><div class="tile-top"><span class="tile-icon"><n-icon :size="28"><VolumeHighOutline/></n-icon></span><span class="status-pill" :class="{live:row.transport_state==='PLAYING'||row.airplay_active}">{{row.transport_state==='PLAYING'||row.airplay_active ? '播放中' : '待投送'}}</span></div><h2>{{row.dlna_name}}</h2><p class="device-model">{{getDeviceModelInfo(row.hardware).model}} · {{row.hardware}}</p>
   <form class="rename-form" @submit.prevent="doRename(row)"><label :for="'name-'+row.did">投送时显示的名称</label><div class="rename-input"><n-input :input-props="{id: 'name-'+row.did}" :value="editing[row.did] ?? row.dlna_name" :disabled="saving[row.did]" :maxlength="40" @update:value="v => editing[row.did]=v"/><n-button attr-type="submit" type="primary" :loading="saving[row.did]" :disabled="!canSave(row)">{{row.name_sync==='failed' && !changed(row) ? '重试同步' : '保存'}}</n-button></div><div class="save-feedback" :class="{failed:row.name_sync==='failed'}" role="status" aria-live="polite">{{saving[row.did] ? '正在保存名称…' : row.name_sync==='pending' ? '名称已保存 · AirPlay 广播同步中，可继续其他操作' : row.name_sync==='failed' ? '名称已保存，AirPlay 同步失败，请重试同步。' : feedback[row.did] || (row.name_sync==='synced' ? '名称已保存并同步，投送列表可能需要重新打开。' : '建议使用房间名称，例如：客厅 · 小爱')}}</div></form>
   <div class="compatibility-row"><div><strong>兼容模式</strong><p>播放异常时开启，优先使用通用接口。</p></div><n-switch :value="row.compatibility_mode" :loading="switching[row.did]" :disabled="saving[row.did] || row.name_sync==='pending'" :aria-label="row.dlna_name+'兼容模式'" @update:value="v => toggle(row,v)"/></div><div class="tile-bottom"><span>DLNA / AirPlay</span><span>在音乐 App 中选择此音箱投送</span></div>
  </article></div>
 </div>
</template>
<script setup lang="ts">
import {computed,onMounted,onUnmounted,ref} from 'vue'
import {useRouter} from 'vue-router'
import {NAlert,NInput,NButton,NIcon,NSwitch,useMessage} from 'naive-ui'
import {VolumeHighOutline} from '@vicons/ionicons5'
import {fetchSpeakers,renameSpeaker,setCompatibilityMode,type SpeakerStatus} from '@/api/speakers'
import {getDeviceModelInfo} from '@/utils/deviceModel'
const router=useRouter(),message=useMessage()
const speakers=ref<SpeakerStatus[]>([]),loading=ref(false),loadError=ref(''),query=ref('')
const editing=ref<Record<string,string>>({}),saving=ref<Record<string,boolean>>({}),switching=ref<Record<string,boolean>>({}),feedback=ref<Record<string,string>>({})
const filtered=computed(()=>speakers.value.filter(s=>(s.dlna_name+' '+s.hardware+' '+getDeviceModelInfo(s.hardware).model).toLowerCase().includes(query.value.toLowerCase())))
const changed=(row:SpeakerStatus)=>(editing.value[row.did]??row.dlna_name).trim()!==row.dlna_name
const canSave=(row:SpeakerStatus)=>!saving.value[row.did]&&!switching.value[row.did]&&(changed(row)||row.name_sync==='failed')
let fetching=false
let stopped=false, timer:ReturnType<typeof setTimeout>|undefined, revision=0
function poll(){clearTimeout(timer);if(!stopped)timer=setTimeout(async()=>{await load(true);poll()},2500)}
async function load(silent=false){if(fetching||Object.values(saving.value).some(Boolean)||Object.values(switching.value).some(Boolean))return;fetching=true;if(!silent)loading.value=true;const started=revision;try{const data=await fetchSpeakers();if(!stopped && started===revision){speakers.value=data;loadError.value=''}}catch(e:any){loadError.value=e.response?.data?.detail||'音箱状态读取失败，请检查连接后刷新。'}finally{fetching=false;loading.value=false}}
async function doRename(row:SpeakerStatus){if(!canSave(row))return;const name=(editing.value[row.did]??row.dlna_name).trim();if(!name){message.warning('请输入投送名称');return}if(Array.from(name).length>40){message.warning('名称最多 40 个字符');return}saving.value[row.did]=true;revision++;try{const result=await renameSpeaker(row.did,name);row.dlna_name=result.dlna_name;row.name_sync=result.name_sync;delete editing.value[row.did];feedback.value[row.did]='名称已保存，重新打开投送列表可查看。';message.success('投送名称已保存')}catch(e:any){message.error(e.response?.data?.detail||'保存失败，请重试')}finally{revision++;saving.value[row.did]=false}}
async function toggle(row:SpeakerStatus,value:boolean){switching.value[row.did]=true;revision++;try{await setCompatibilityMode(row.did,value);row.compatibility_mode=value;message.success('设置已保存，服务正在重新连接')}catch(e:any){message.error(e.response?.data?.detail||'切换失败')}finally{revision++;switching.value[row.did]=false}}
onMounted(()=>{load();poll()});onUnmounted(()=>{stopped=true;clearTimeout(timer)})
</script>
