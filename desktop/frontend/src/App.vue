<script lang="ts" setup>
import { onMounted } from 'vue'
import { useAppStore } from './stores/app'
import { clientLog, timeText } from './utils'
import Sidebar from './components/Sidebar.vue'
import ChatPanel from './components/ChatPanel.vue'
import SettingsPanel from './components/SettingsPanel.vue'
import ArtifactViewer from './components/ArtifactViewer.vue'
import PlanView from './components/PlanView.vue'

const store = useAppStore()

onMounted(() => {
  window.addEventListener('error', (event) => clientLog('error', '前端未捕获异常', { error: event.message, source: event.filename, line: String(event.lineno) }))
  window.addEventListener('unhandledrejection', (event) => clientLog('error', '前端未处理的异步异常', { error: String(event.reason) }))
  store.setupAgentStatusListener()
  store.refresh()
})
</script>

<template>
  <div class="app-shell">
    <Sidebar />

    <main class="main">
      <header class="topbar">
        <span class="crumb">{{ store.activePanel === 'settings' ? '模型与设置' : (store.selectedWorkspace?.name || '选择工作目录') }}</span>
        <span v-if="store.activePanel === 'chat' && store.activeConversation" class="conversation-name">{{ store.activeConversation.conversation.title }}</span>
      </header>

      <div v-if="store.notice" class="banner success">{{ store.notice }}</div>
      <div v-if="store.error" class="banner danger">{{ store.error }}</div>

      <ChatPanel v-if="store.activePanel === 'chat'" />
      <SettingsPanel v-else />

      <ArtifactViewer />
      <PlanView />
    </main>
  </div>
</template>
