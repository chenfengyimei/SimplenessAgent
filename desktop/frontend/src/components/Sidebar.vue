<script lang="ts" setup>
import { useAppStore } from '../stores/app'
import { clientLog } from '../utils'

const store = useAppStore()
</script>

<template>
  <aside class="sidebar">
    <div class="brand"><div class="brand-mark">S</div><div><strong>Simpleness</strong><small>智能工作台</small></div></div>
    <button class="new-chat" :class="{ active: store.isNewConversation && store.activePanel === 'chat' }" @click="store.newConversation"><span>✎</span> 新对话</button>
    <nav><button :class="{ active: store.activePanel === 'settings' }" @click="store.openSettings"><span>◇</span> 模型与设置</button></nav>
    <section class="projects">
      <p>项目</p>
      <div v-for="group in store.groupedConversations" :key="group.workspace.id" class="project-group">
        <div class="project-head" :class="{ selected: store.workspaceID === group.workspace.id }">
          <button @click="store.selectWorkspace(group.workspace.id)"><span>▱</span><b>{{ group.workspace.name }}</b><em>{{ group.conversations.length }}</em></button>
          <button class="collapse" @click="store.toggleWorkspace(group.workspace.id)">{{ store.collapsed[group.workspace.id] ? '›' : '⌄' }}</button>
        </div>
        <div v-show="!store.collapsed[group.workspace.id]" class="conversation-list">
          <button v-for="conversation in group.conversations" :key="conversation.id" :class="{ selected: store.activeConversation?.conversation.id === conversation.id }" @click="store.openConversation(conversation)">{{ conversation.title || '未命名会话' }}</button>
          <small v-if="!group.conversations.length">该目录尚无会话</small>
        </div>
      </div>
      <p v-if="!store.workspaces.length" class="empty-project">请先在"模型与设置"授权工作目录。</p>
    </section>
    <div class="core-status"><i></i> 核心服务已连接</div>
  </aside>
</template>
