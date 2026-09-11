<template>
 <div class="about-page">
  <section class="glass about-intro">
   <n-icon :size="38" class="about-symbol"><RadioOutline/></n-icon>
   <h2>MiAir Plus</h2><span class="about-version">{{version}} · 正式版</span>
   <p>让小爱音箱连接更多音乐。</p>
   <p>DLNA / AirPlay 音乐投送，自定义音乐闹钟。<br>支持音乐上传、NAS 选曲，以及工作日和节假日提醒。</p>
  </section>
  <section class="glass about-details" aria-label="开发者与项目信息">
   <div class="about-row"><span>开发者</span><a href="https://github.com/BearHero520" target="_blank" rel="noopener noreferrer">BearHero520 ↗</a></div>
   <a class="about-row about-link" href="https://github.com/BearHero520/miair-plus" target="_blank" rel="noopener noreferrer"><span>项目主页</span><strong>GitHub ↗</strong></a>
   <a class="about-row about-link" href="https://github.com/BearHero520/miair-plus/issues" target="_blank" rel="noopener noreferrer"><span>问题反馈</span><strong>提交问题或建议 ↗</strong></a>
   <a class="about-row about-link" href="https://github.com/BearHero520/miair-plus/releases" target="_blank" rel="noopener noreferrer"><span>版本发布</span><strong>查看更新 ↗</strong></a>
   <a class="about-row about-link" href="https://github.com/BearHero520/FnDepot" target="_blank" rel="noopener noreferrer"><span>飞牛应用源</span><strong>BearHero 应用源 ↗</strong></a>
  </section>
  <section class="glass about-details" aria-label="飞牛环境">
   <div class="about-row"><span>飞牛环境</span><strong>{{fnos.isStandaloneWeb?'独立浏览器':fnos.isWeb?'飞牛网页桌面':'飞牛手机 App'}}</strong></div>
   <template v-if="!fnos.isStandaloneWeb">
    <div class="about-row"><span>系统版本</span><strong>{{fnosHost.systemVersion||'未读取'}}</strong></div>
    <div class="about-row" v-if="fnosHost.appVersion"><span>App 版本</span><strong>{{fnosHost.appVersion}}</strong></div>
    <div class="about-row"><span>{{fnosHost.error||'主题与版本来自飞牛宿主'}}</span><n-button :loading="hostBusy" @click="refreshHost">刷新</n-button><n-button @click="openSettings">应用设置</n-button></div>
   </template>
  </section>
  <section class="about-license"><h3>开源与致谢</h3><p>本项目采用 <a href="https://github.com/BearHero520/miair-plus/blob/main/LICENSE" target="_blank" rel="noopener noreferrer">GPL-3.0</a> 许可证。感谢 MiAir、miair-next 与相关开源组件。</p><a href="https://github.com/BearHero520/miair-plus/blob/main/UPSTREAM.md" target="_blank" rel="noopener noreferrer">查看来源与许可 ↗</a></section>
 </div>
</template>
<script setup lang="ts">
import {NIcon,NButton,useMessage} from 'naive-ui'
import {ref} from 'vue'
import {fnos,fnosHost,refreshFnosHost,openFnosSettings} from '@/utils/fnos'
const hostBusy=ref(false),message=useMessage()
async function refreshHost(){hostBusy.value=true;try{await refreshFnosHost()}finally{hostBusy.value=false}}
async function openSettings(){try{await openFnosSettings()}catch(e){message.warning(e instanceof Error?e.message:'无法打开飞牛设置')}}
import {RadioOutline} from '@vicons/ionicons5'
import {version} from '../../../package.json'
</script>
<style scoped>
.about-page{max-width:720px;margin:0 auto}.about-intro{text-align:center;padding:32px 24px}.about-symbol{color:var(--accent);margin-bottom:12px}.about-intro h2{font-size:26px;margin-bottom:8px}.about-version{display:inline-block;color:var(--accent);background:var(--soft);border-radius:20px;padding:4px 12px;font-size:12px;margin-bottom:14px}.about-details{margin-top:20px;padding:8px 24px}.about-row{display:flex;align-items:center;justify-content:space-between;gap:20px;padding:18px 0;font-size:14px}.about-row+.about-row{border-top:1px solid var(--line)}.about-row>span{color:var(--muted)}.about-row strong{font-weight:500}.about-link:hover strong{text-decoration:underline}.about-license{padding:22px 8px}.about-license h3{font-size:14px;margin:0 0 8px}.about-license>a{font-size:12px;display:inline-block;margin-top:8px}@media(max-width:700px){.about-intro{padding:26px 16px}.about-details{padding:4px 16px}.about-row{gap:12px;flex-wrap:wrap;font-size:13px}}
</style>
