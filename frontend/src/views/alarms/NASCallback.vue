<template><div class="auth-wrap"><div class="glass" style="padding:32px;max-width:460px"><h2>{{text}}</h2><p>可以关闭此页面，回到闹钟设置。</p></div></div></template>
<script setup lang="ts">
import {ref,onMounted} from 'vue'
import {fnos} from '@/utils/fnos'
import {storage as localStorage} from '@/utils/storage'
const text=ref('正在返回所选文件…')
onMounted(()=>{
 const result=fnos.parseAppAuthCallback(location.href)
 if(result.appName!=='miair-plus'||result.method!=='pickUserFile'||!result.state){text.value='授权结果无效，请重新选择';return}
 const data={type:'miair:nas-file',result}
 window.opener?.postMessage(data,location.origin)
 // Same-origin fallback for mobile browsers without window.opener.
 localStorage.setItem('miair:nas-file-result',JSON.stringify(data))
 text.value=result.status==='success'?'文件已选择':result.status==='cancel'?'已取消选择':'未获得文件授权'
 history.replaceState(null,'',location.pathname)
 if(window.opener)window.close()
})
</script>
