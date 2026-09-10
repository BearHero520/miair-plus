<template><n-config-provider :theme="theme" :theme-overrides="overrides" :locale="zhCN" :date-locale="dateZhCN"><n-global-style /><n-message-provider><n-dialog-provider><router-view /></n-dialog-provider></n-message-provider></n-config-provider></template>
<script setup lang="ts">
import { computed, watchEffect } from 'vue'
import { NConfigProvider, NGlobalStyle, NMessageProvider, NDialogProvider, darkTheme, zhCN, dateZhCN, type GlobalThemeOverrides } from 'naive-ui'
import { useAppStore } from '@/stores/app'
import '@/styles/island.css'
const app = useAppStore()
const theme = computed(() => app.dark ? darkTheme : null)
watchEffect(() => { document.documentElement.dataset.theme = app.dark ? 'dark' : 'light' })
const overrides = computed<GlobalThemeOverrides>(() => ({ common: { primaryColor: app.dark ? '#8ed7b9' : '#217365', primaryColorHover: app.dark ? '#a8e7cb' : '#348976', primaryColorPressed: '#195b50', borderRadius: '12px', fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif' }, Card: { borderRadius: '20px' }, Button: { borderRadiusMedium: '12px', heightMedium: '42px' } }))
</script>
<style>html,body,#app{min-height:100%;margin:0}</style>
