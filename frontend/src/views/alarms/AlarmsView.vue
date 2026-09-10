<template>
 <div class="alarm-toolbar"><p>用家里的声音，叫醒新的一天。</p><n-button type="primary" @click="edit()">新建闹钟</n-button></div>
 <n-alert v-if="error" type="error" style="margin-bottom:16px">{{error}}</n-alert>
 <div v-if="!rows.length" class="glass compact-empty"><n-icon :size="36"><AlarmOutline/></n-icon><h3>设置第一个闹钟</h3><p>选择音箱、时间和 NAS 铃声。停止后不会自动重播。</p><n-button @click="edit()">添加闹钟</n-button></div>
 <div class="alarm-grid">
  <section v-for="row in rows" :key="row.alarm.id" class="glass alarm-card">
   <div class="section-heading"><strong>{{row.alarm.name}}</strong><n-switch :value="row.alarm.enabled" :disabled="busy||active(row.state)" :aria-label="`启用${row.alarm.name}`" @update:value="toggle(row.alarm,$event)"/></div>
   <div class="alarm-clock">{{row.alarm.time}}<small>{{ruleLabel(row.alarm)}}</small></div>
   <p>{{speakerName(row.alarm.speaker)}} · {{row.alarm.volume}}% 音量 · 最长 {{row.alarm.minutes}} 分钟</p>
   <p class="alarm-sound">{{audioName(row.alarm.sound)}}</p>
   <n-alert v-if="!row.calendar_ready" type="warning" style="margin-top:12px">当前年份日历未收录，该规则暂停触发。请更新日历。</n-alert>
   <p v-if="row.error" class="alarm-error">{{row.error}}</p>
   <div v-if="active(row.state)" class="alarm-actions"><n-tag type="warning">{{states[row.state]}}</n-tag><n-button type="primary" :loading="busy" @click="action(row.alarm.id,'stop')">停止</n-button><n-button :disabled="busy||row.state==='stopping'" @click="action(row.alarm.id,'snooze')">延后 5 分钟</n-button></div>
   <div v-else class="alarm-actions"><n-button :disabled="busy" @click="edit(row.alarm)">编辑</n-button><n-popconfirm @positive-click="action(row.alarm.id,'test')"><template #trigger><n-button :disabled="busy">试听</n-button></template>将在这台音箱上立即播放铃声，继续？</n-popconfirm><n-popconfirm @positive-click="remove(row.alarm.id)"><template #trigger><n-button quaternary :disabled="busy">删除</n-button></template>删除此闹钟？</n-popconfirm></div>
   <p v-if="row.alarm.enabled&&row.next.length" class="alarm-next">接下来：{{row.next.join(' / ')}}</p>
  </section>
 </div>
 <section class="glass alarm-help"><strong>铃声与停止方式</strong><p>可从电脑或手机上传音乐，也可通过飞牛选择 NAS 中的文件并授权。已导入音乐保存在 <b>miair-plus / ringtones</b>，可从铃声库复用。支持 MP3、WAV、FLAC、OGG、AAC，单文件最大 100 MB。</p><p>铃声播放一次，播完或到最长时长后结束。可在此页停止或延后，也可尝试“小爱同学，停止播放”或机身暂停键，语音／按键效果取决于音箱型号。本功能独立于小爱内置闹钟，需要 NAS 与音箱在线。</p><div class="alarm-calendar"><span>中国大陆调休日历：{{Object.keys(calendar).join('、')||'未载入'}}</span><n-button size="small" :loading="calendarBusy" @click="refreshCalendar(false)">更新今年</n-button><n-button size="small" :loading="calendarBusy" @click="refreshCalendar(true)">获取明年</n-button></div><p>法定工作日含周末补班；休息日含周末与放假调休；仅节假日只在公告放假日期响铃。日历未收录的年份不会猜测。</p></section>
 <n-modal v-model:show="show" preset="card" :title="form.id?'编辑闹钟':'新建闹钟'" class="alarm-modal" :mask-closable="!busy" :closable="!busy">
  <n-form label-placement="top" :disabled="busy">
   <n-form-item label="名称"><n-input v-model:value="form.name" maxlength="32" placeholder="例如：起床时间"/></n-form-item>
   <div class="alarm-fields"><n-form-item label="响铃时间"><input v-model="form.time" class="alarm-time-input" type="time" aria-label="响铃时间" required/></n-form-item><n-form-item label="重复规则"><n-select v-model:value="form.rule" :options="rules"/></n-form-item></div>
   <n-form-item v-if="form.rule==='weekly'" label="星期"><n-checkbox-group v-model:value="selectedDays"><n-space><n-checkbox v-for="(day,i) in dayNames" :key="day" :value="i" :label="day"/></n-space></n-checkbox-group></n-form-item>
   <n-form-item label="时区"><n-select v-model:value="form.timezone" :options="[{label:'北京时间 · Asia/Shanghai',value:'Asia/Shanghai'}]"/></n-form-item>
   <n-form-item label="目标音箱"><n-select v-model:value="form.speaker" :options="speakerOptions" placeholder="请选择已启用的音箱"/></n-form-item>
   <n-form-item label="闹钟音乐"><div class="alarm-music-source"><n-input :value="audioName(form.sound)" readonly placeholder="上传音乐或从 NAS 选择"/><div class="alarm-actions"><n-button :loading="uploading" :disabled="nasBusy||busy" @click="uploadInput?.click()">上传音乐</n-button><n-button :loading="nasBusy" :disabled="uploading||busy" @click="selectNAS">从 NAS 选择</n-button><n-button quaternary :disabled="uploading||busy" @click="openFiles('.')">铃声库</n-button></div><input ref="uploadInput" type="file" accept=".mp3,.wav,.flac,.ogg,.aac" hidden @change="uploadMusic"/><p v-if="uploading">正在上传 {{uploadProgress}}%</p><p v-if="nasBusy">请在飞牛页面选择文件并授权，完成后返回这里。<n-button text @click="cancelNAS">取消</n-button></p></div></n-form-item>
   <div class="alarm-fields"><n-form-item label="响铃音量（%）"><n-input-number v-model:value="form.volume" :min="1" :max="100"/></n-form-item><n-form-item label="最长响铃（分钟）"><n-input-number v-model:value="form.minutes" :min="1" :max="10"/></n-form-item></div>
   <n-alert v-if="saveError" type="error" style="margin-bottom:14px">{{saveError}}</n-alert>
   <p v-if="busy">正在保存并准备铃声，请稍候…</p><n-button type="primary" block :loading="busy" :disabled="uploading||nasBusy" @click="save">保存闹钟</n-button>
  </n-form>
 </n-modal>
 <n-modal v-model:show="picker" preset="card" title="选择 NAS 铃声" class="alarm-modal">
  <p>miair-plus / ringtones / {{folder==='.'?'':folder}}</p><n-button v-if="folder!=='.'" size="small" @click="openFiles(parentFolder)">返回上一级</n-button>
  <n-alert v-if="fileError" type="warning">{{fileError}}</n-alert>
  <div class="alarm-file-list"><n-button v-for="file in files" :key="file.path" quaternary block @click="pick(file)">{{file.directory?'文件夹 · ':''}}{{file.name}}</n-button><p v-if="!files.length&&!fileError">目录里还没有音频。请先通过飞牛文件管理复制铃声，再刷新。</p></div>
  <n-button :loading="filesBusy" @click="openFiles(folder)">刷新文件</n-button>
 </n-modal>
</template>
<script setup lang="ts">
import {computed,onMounted,onUnmounted,reactive,ref,watch} from 'vue'
import {NAlert,NButton,NCheckbox,NCheckboxGroup,NForm,NFormItem,NIcon,NInput,NInputNumber,NModal,NPopconfirm,NSelect,NSpace,NSwitch,NTag,useMessage} from 'naive-ui'
import {AlarmOutline} from '@vicons/ionicons5'
import http from '@/api/http'
import {TrimApp,type AppAuthResult} from '@trimjs/web-app'
type Alarm={id:string;name:string;time:string;timezone:string;rule:string;days:number;speaker:string;sound:string;volume:number;minutes:number;enabled:boolean}
type Row={alarm:Alarm;state:string;error:string;next:string[];calendar_ready:boolean}
type FileItem={name:string;path:string;directory:boolean}
const rows=ref<Row[]>([]),speakers=ref<{did:string;dlna_name:string;name:string;enabled:boolean}[]>([]),calendar=ref<Record<string,string[]>>({}),error=ref(''),saveError=ref(''),show=ref(false),busy=ref(false),calendarBusy=ref(false),picker=ref(false),folder=ref('.'),files=ref<FileItem[]>([]),fileError=ref(''),filesBusy=ref(false),message=useMessage()
const fresh=():Alarm=>({id:'',name:'起床闹钟',time:'07:00',timezone:'Asia/Shanghai',rule:'workday',days:62,speaker:'',sound:'',volume:30,minutes:3,enabled:true})
const sdk=new TrimApp({debug:false})
const uploadInput=ref<HTMLInputElement>(),uploading=ref(false),uploadProgress=ref(0),nasBusy=ref(false)
let nasState='',nasWindow:Window|null=null,uploadAbort:AbortController|undefined
const form=reactive(fresh()),dayNames=['周日','周一','周二','周三','周四','周五','周六']
const rules=[{label:'法定工作日（含调休补班）',value:'workday'},{label:'休息日（周末及节假日）',value:'restday'},{label:'仅法定节假日',value:'holiday'},{label:'指定星期 / 每天',value:'weekly'}]
const selectedDays=computed({get:()=>dayNames.map((_,i)=>i).filter(i=>form.days&(1<<i)),set:(days:(string|number)[])=>{form.days=days.reduce<number>((mask,d)=>mask|(1<<Number(d)),0)}})
const parentFolder=computed(()=>folder.value.split('/').slice(0,-1).join('/')||'.')
const speakerOptions=computed(()=>speakers.value.filter(s=>s.enabled).map(s=>({label:s.dlna_name||s.name,value:s.did})))
const states:Record<string,string>={starting:'正在连接',ringing:'正在响铃',stopping:'正在停止',stop_failed:'停止未确认'}
const active=(state:string)=>['starting','ringing','stopping','stop_failed'].includes(state)
const audioName=(path:string)=>path.split('/').pop()?.replace(/^[a-f0-9]{12}-/,'')||''
const reason=(e:any)=>e?.response?.data?.error||e?.message||'操作失败，请重试'
const speakerName=(id:string)=>speakerOptions.value.find(s=>s.value===id)?.label||'音箱不可用'
const ruleLabel=(a:Alarm)=>a.rule==='weekly'?a.days===127?'每天':dayNames.filter((_,i)=>a.days&(1<<i)).join('、'):rules.find(r=>r.value===a.rule)?.label
let timer:ReturnType<typeof setInterval>|undefined,inFlight=false
async function load(){if(inFlight)return;inFlight=true;try{const response=await http.get('/alarms');rows.value=response.data.alarms;calendar.value=response.data.calendar;error.value=''}catch(e){error.value=reason(e)}finally{inFlight=false}}
function edit(a?:Alarm){Object.assign(form,fresh(),a||{});saveError.value='';show.value=true}
async function save(){if(uploading.value||nasBusy.value)return;busy.value=true;saveError.value='';try{await http.post('/alarms',form,{timeout:60000});show.value=false;await load();message.success('闹钟已保存')}catch(e){saveError.value=reason(e)}finally{busy.value=false}}
async function toggle(a:Alarm,enabled:boolean){busy.value=true;try{await http.post('/alarms',{...a,enabled});await load()}catch(e){message.error(reason(e))}finally{busy.value=false}}
async function action(id:string,action:string){busy.value=true;try{await http.post(`/alarms/${id}/${action}`);await load()}catch(e){message.error(reason(e));await load()}finally{busy.value=false}}
async function remove(id:string){busy.value=true;try{await http.delete(`/alarms/${id}`);await load()}catch(e){message.error(reason(e))}finally{busy.value=false}}
async function openFiles(path:string){picker.value=true;filesBusy.value=true;fileError.value='';files.value=[];try{const r=await http.get('/alarms/files',{params:{path}});files.value=r.data.files;folder.value=r.data.path}catch(e){fileError.value=reason(e)}finally{filesBusy.value=false}}
function pick(file:FileItem){if(file.directory){void openFiles(file.path)}else{form.sound=file.path;picker.value=false}}
async function refreshCalendar(next:boolean){calendarBusy.value=true;try{await http.post('/alarms/calendar',{year:new Date().getFullYear()+(next?1:0)});await load();message.success('日历已更新')}catch(e){message.warning(reason(e))}finally{calendarBusy.value=false}}
async function uploadMusic(event:Event){
 const input=event.target as HTMLInputElement,file=input.files?.[0];if(!file)return
 if(file.size>100*1024*1024||file.size===0){message.error('请选择 100 MB 以内的音频文件');input.value='';return}
 uploading.value=true;uploadProgress.value=0;saveError.value='';uploadAbort=new AbortController()
 try{const body=new FormData();body.append('file',file);const r=await http.post('/alarms/upload',body,{timeout:180000,signal:uploadAbort.signal,onUploadProgress:event=>{uploadProgress.value=Math.round(100*event.loaded/(event.total||file.size))}});form.sound=r.data.sound;message.success('音乐已上传，保存闹钟时将准备铃声')}catch(e){saveError.value=reason(e)}finally{uploading.value=false;input.value=''}
}
function cancelNAS(){nasState='';nasBusy.value=false;if(nasWindow&&!nasWindow.closed)nasWindow.close();nasWindow=null}
async function importNASResult(result:AppAuthResult){
 if(!nasState||result.state!==nasState||result.appName!=='miair-plus'||result.method!=='pickUserFile')return
 nasState='';localStorage.removeItem('miair:nas-file-result')
 try{if(result.status==='cancel')return;if(result.status!=='success'||!result.path?.[0])throw Error('未获得 NAS 文件授权');const r=await http.post('/alarms/import-nas',{path:result.path[0]},{timeout:120000});form.sound=r.data.sound;message.success('NAS 音乐已选取')}catch(e){saveError.value=reason(e)}finally{nasBusy.value=false;nasWindow=null}
}
function nasMessage(event:MessageEvent){if(event.origin!==location.origin||event.source!==nasWindow||event.data?.type!=='miair:nas-file')return;void importNASResult(event.data.result)}
function nasStorage(event:StorageEvent){if(event.key!=='miair:nas-file-result'||!event.newValue)return;try{const v=JSON.parse(event.newValue);if(v.type==='miair:nas-file')void importNASResult(v.result)}catch{}}
async function selectNAS(){
 saveError.value='';nasBusy.value=true
 nasState=Array.from(crypto.getRandomValues(new Uint8Array(24)),b=>b.toString(16).padStart(2,'0')).join('')
 // Open synchronously in the click handler to preserve the browser user gesture.
 nasWindow=window.open('','miair-nas-picker','width=860,height=680')
 if(!nasWindow){nasBusy.value=false;saveError.value='请允许弹出窗口，或使用上传音乐';return}
 try{
  const auth=await sdk.buildAppAuthUrl('pickUserFile',{appName:'miair-plus',directory:false,accept:['.mp3','.wav','.flac','.ogg','.aac'],sidebarGroup:['myFiles','otherShare','favorites'],redirectUri:location.origin+'/nas-file-callback',state:nasState})
  const url=new URL(auth);url.hostname=location.hostname;url.port=location.protocol==='https:'?'5001':'5000'
  nasWindow.location.href=url.href
 }catch(e){cancelNAS();saveError.value=reason(e)}
}
watch(show,value=>{if(!value){uploadAbort?.abort();cancelNAS();picker.value=false}})
onMounted(async()=>{window.addEventListener('message',nasMessage);window.addEventListener('storage',nasStorage);await load();try{speakers.value=(await http.get('/speakers')).data}catch(e){error.value=reason(e)};timer=setInterval(()=>{if(!document.hidden)void load()},3000)})
onUnmounted(()=>{uploadAbort?.abort();if(timer)clearInterval(timer);window.removeEventListener('message',nasMessage);window.removeEventListener('storage',nasStorage);cancelNAS()})
</script>
<style scoped>
.alarm-toolbar,.alarm-calendar{display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin-bottom:18px}.alarm-toolbar p{flex:1}.alarm-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:18px}.alarm-card{padding:23px}.alarm-clock{font-size:40px;letter-spacing:-1px;font-variant-numeric:tabular-nums;line-height:1.4}.alarm-clock small{display:block;font-size:12px;letter-spacing:0;color:var(--accent);margin:4px 0 10px}.alarm-actions{display:flex;gap:8px;flex-wrap:wrap;margin-top:18px}.alarm-sound{overflow-wrap:anywhere}.alarm-next{font-size:11px;margin-top:14px;border-top:1px solid var(--line);padding-top:10px}.alarm-help{padding:22px;margin-top:24px}.alarm-help strong{font-size:14px}.alarm-help p{margin:8px 0;font-size:12px}.alarm-calendar{font-size:12px;margin:15px 0 0}.alarm-error{color:#e45656}.alarm-fields{display:grid;grid-template-columns:1fr 1fr;gap:16px}.alarm-music-source{width:100%}.alarm-music-source .alarm-actions{margin-top:8px}.alarm-file-choice{display:flex;gap:10px;width:100%}.alarm-time-input{font:inherit;background:var(--surface);color:var(--ink);border:1px solid var(--line);border-radius:6px;padding:9px;width:100%}.alarm-file-list{max-height:300px;overflow:auto;margin:12px 0}.alarm-file-list .n-button{justify-content:flex-start}.alarm-modal{width:min(560px,calc(100vw - 32px))!important}.alarm-fields .n-input-number{width:100%}@media(max-width:760px){.alarm-grid{grid-template-columns:1fr}.alarm-fields{grid-template-columns:1fr}.alarm-clock{font-size:34px}}
</style>
