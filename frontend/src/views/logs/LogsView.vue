<template>
 <div>
  <n-alert v-if="error" type="warning" class="status-error">{{error}}</n-alert>
  <section class="diagnostic-grid" aria-label="诊断概况"><div v-for="check in checks" :key="check.name" class="glass diagnostic-card"><span>{{check.name}}</span><strong>{{{ok:'已就绪',warn:'需检查',error:'异常',off:'已关闭'}[check.state]}}</strong><p>{{check.message}}</p><router-link v-if="check.state==='warn'||check.state==='error'" :to="check.action">查看设置 →</router-link></div></section>
  <section class="glass log-panel">
   <div class="log-toolbar"><n-input v-model:value="query" clearable placeholder="搜索音箱、操作或失败原因" aria-label="搜索日志"/><n-select v-model:value="level" :options="levels" aria-label="日志级别"/><n-select v-model:value="module" :options="modules" aria-label="日志模块"/><n-button :type="paused?'primary':'default'" @click="paused=!paused">{{paused?'继续刷新':'暂停刷新'}}</n-button><n-button :loading="loading" @click="refresh(true)">刷新</n-button><n-button :loading="downloading" @click="download">下载诊断报告</n-button></div>
   <div class="log-summary"><span>{{paused?'已暂停刷新':error?'连接异常':'每 3 秒刷新'}} · 显示 {{filtered.length}} / {{entries.length}} 条</span><span>最近 {{capacity}} 条 · 日志文件轮转保留 · 敏感信息已脱敏</span></div>
   <n-alert v-if="storageError" type="warning">日志持久化失败：{{storageError}}</n-alert>
   <div class="log-feed" aria-label="运行日志" tabindex="0">
    <div v-if="!filtered.length" class="log-empty">{{entries.length?'没有符合筛选条件的记录':'暂无记录。启用音箱或发起一次投送后，可在这里查看过程。'}}</div>
    <article v-for="entry in filtered" :key="entry.id" class="log-row"><div class="log-row-head"><time :datetime="entry.time">{{new Date(entry.time).toLocaleString('zh-CN',{hour12:false})}}</time><span class="level-badge" :class="'level-'+entry.level">{{{info:'信息',warn:'警告',error:'错误'}[entry.level]||entry.level}}</span><span>{{entry.module}}</span></div><p>{{entry.message}}</p><div v-if="entry.details" class="log-details"><span v-for="(value,key) in entry.details" :key="key">{{key}}：{{value}}</span></div></article>
   </div>
  </section>
 </div>
</template>
<script setup lang="ts">
import {computed,onMounted,onUnmounted,ref} from 'vue'
import {NInput,NSelect,NButton,NAlert,useMessage} from 'naive-ui'
import http from '@/api/http'
type Entry={id:number;time:string;level:string;module:string;message:string;details?:Record<string,string>}
type Check={name:string;state:'ok'|'warn'|'error'|'off';message:string;action:string}
const entries=ref<Entry[]>([]),checks=ref<Check[]>([]),capacity=ref(1000),storageError=ref(''),error=ref(''),query=ref(''),level=ref('all'),module=ref('all'),paused=ref(false),loading=ref(false),downloading=ref(false),message=useMessage()
const levels=[{label:'全部级别',value:'all'},{label:'警告与错误',value:'problems'},{label:'错误',value:'error'},{label:'警告',value:'warn'},{label:'信息',value:'info'}]
const modules=computed(()=>[{label:'全部模块',value:'all'},...Array.from(new Set(entries.value.map(e=>e.module))).sort().map(v=>({label:v,value:v}))])
const filtered=computed(()=>entries.value.filter(e=>(level.value==='all'||level.value==='problems'&&e.level!=='info'||e.level===level.value)&&(module.value==='all'||e.module===module.value)&&JSON.stringify([e.message,e.details,e.module]).toLowerCase().includes(query.value.toLowerCase())).slice().reverse())
async function refresh(force=false){if(loading.value||paused.value&&!force)return;loading.value=true;try{const {data}=await http.get('/diagnostics');entries.value=data.entries;checks.value=data.checks;capacity.value=data.capacity;storageError.value=data.storage_error||'';error.value=''}catch{error.value='日志读取失败，请检查服务连接；当前保留上次读取的记录。'}finally{loading.value=false}}
async function download(){downloading.value=true;try{const {data}=await http.get('/diagnostics/download',{responseType:'blob'});const url=URL.createObjectURL(data);const a=document.createElement('a');a.href=url;a.download='miair-plus-diagnostics.json';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000)}catch{message.error('诊断报告下载失败')}finally{downloading.value=false}}
let timer:ReturnType<typeof setInterval>
onMounted(()=>{refresh();timer=setInterval(()=>refresh(),3000)})
onUnmounted(()=>clearInterval(timer))
</script>
