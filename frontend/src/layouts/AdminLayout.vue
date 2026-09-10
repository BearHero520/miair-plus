<template>
 <div class="studio-shell">
  <a class="skip-link" href="#main">跳转到主要内容</a>
  <header class="studio-top glass">
   <router-link to="/dashboard" class="studio-brand"><span class="island-symbol"><svg width="36" height="36" viewBox="0 0 64 64" aria-hidden="true"><g fill="none" stroke="currentColor" stroke-width="5" stroke-linecap="round"><path d="M15 26 Q23 18 32 25 T49 24"/><path d="M15 39 Q23 31 32 38 T49 37"/></g></svg></span><span>MiAir <em class="brand-plus">Plus</em><small>AUDIO BRIDGE</small></span></router-link>
   <nav class="studio-nav" aria-label="主导航"><router-link v-for="item in navigation" :key="item.name" :to="{name:item.name}"><n-icon :component="item.icon"/><span>{{item.label}}</span></router-link></nav>
   <div class="studio-tools"><n-button quaternary circle :aria-label="app.dark ? '切换浅色外观' : '切换深色外观'" @click="app.theme = app.dark ? 'light' : 'dark'"><template #icon><n-icon :component="app.dark ? SunnyOutline : MoonOutline"/></template></n-button><HeaderBar/></div>
  </header>
  <main id="main" class="studio-main" tabindex="-1"><div v-if="route.name !== 'dashboard'" class="workspace-heading"><div><span class="eyebrow">{{sections[String(route.name)]}}</span><h1>{{route.meta.title}}</h1><p>{{descriptions[String(route.name)]}}</p></div><span class="workspace-index">{{String(navigation.findIndex(n => n.name === route.name)+1).padStart(2,'0')}} / 05</span></div><router-view/></main>
  <footer class="studio-footer"><span>MiAir Plus · 家中的声音控制台</span><span>2.0.0-alpha.1 · <router-link to="/logs">运行日志</router-link></span></footer>
 </div>
</template>
<script setup lang="ts">
import {useRoute} from 'vue-router'
import {NIcon,NButton} from 'naive-ui'
import {GridOutline,VolumeHighOutline,PersonOutline,SettingsOutline,DocumentTextOutline,SunnyOutline,MoonOutline} from '@vicons/ionicons5'
import {useAppStore} from '@/stores/app'
import HeaderBar from './components/HeaderBar.vue'
const app=useAppStore(),route=useRoute()
const navigation=[{name:'dashboard',label:'总览',icon:GridOutline},{name:'devices',label:'音箱',icon:VolumeHighOutline},{name:'account',label:'账号',icon:PersonOutline},{name:'settings',label:'设置',icon:SettingsOutline},{name:'logs',label:'日志',icon:DocumentTextOutline}]
const sections:Record<string,string>={devices:'SPEAKERS',account:'CONNECTION',settings:'PREFERENCES',logs:'ACTIVITY'}
const descriptions:Record<string,string>={devices:'每台音箱独立设置，投送时一眼找到。',account:'连接小米账号，选择加入MiAir Plus的音箱。',settings:'管理投送、网络和自动恢复。',logs:'发现异常时，从这里了解发生了什么。'}
</script>
