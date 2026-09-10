<template>
 <div class="app-workspace">
  <a class="skip-link" href="#main">跳转到主要内容</a>
  <aside class="app-sidebar glass">
   <router-link to="/dashboard" class="sidebar-brand" aria-label="MiAir Plus 首页"><n-icon :size="28"><RadioOutline/></n-icon><span>MiAir <em>Plus</em></span></router-link>
   <nav class="sidebar-nav" aria-label="主导航"><router-link v-for="item in navigation" :key="item.name" :to="{name:item.name}" :title="item.label"><n-icon :component="item.icon" :size="21"/><span>{{item.label}}</span></router-link></nav>
   <div class="sidebar-bottom"><span class="sidebar-label">MiAir Plus</span><small>2.0.0-alpha.7</small></div>
  </aside>
  <div class="app-content">
   <header class="app-topbar"><span>{{route.meta.title}}</span><div class="studio-tools"><n-button quaternary circle :aria-label="app.dark ? '切换浅色外观' : '切换深色外观'" @click="app.theme=app.dark?'light':'dark'"><template #icon><n-icon :component="app.dark?SunnyOutline:MoonOutline"/></template></n-button><HeaderBar/></div></header>
   <main id="main" class="app-main" tabindex="-1"><div v-if="route.name!=='dashboard'" class="workspace-heading"><h1>{{route.meta.title}}</h1><p>{{descriptions[String(route.name)]}}</p></div><router-view/></main>
  </div>
 </div>
</template>
<script setup lang="ts">
import {useRoute} from 'vue-router'
import {NIcon,NButton} from 'naive-ui'
import {InformationCircleOutline,AlarmOutline,GridOutline,VolumeHighOutline,PersonOutline,SettingsOutline,DocumentTextOutline,SunnyOutline,MoonOutline,RadioOutline} from '@vicons/ionicons5'
import {useAppStore} from '@/stores/app'
import HeaderBar from './components/HeaderBar.vue'
const app=useAppStore(),route=useRoute()
const navigation=[{name:'dashboard',label:'总览',icon:GridOutline},{name:'devices',label:'音箱',icon:VolumeHighOutline},{name:'alarms',label:'闹钟',icon:AlarmOutline},{name:'account',label:'账号',icon:PersonOutline},{name:'settings',label:'设置',icon:SettingsOutline},{name:'logs',label:'日志',icon:DocumentTextOutline},{name:'about',label:'关于',icon:InformationCircleOutline}]
const descriptions:Record<string,string>={about:'项目信息与开发者',alarms:'为日常与假日分别设置提醒',devices:'管理音箱与投送名称',account:'连接小米账号，选择音箱',settings:'投送、连接与外观',logs:'查看投送记录，定位连接与播放问题'}
</script>
