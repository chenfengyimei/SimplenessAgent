<script lang="ts" setup>
import { useAppStore } from '../stores/app'
import { timeText } from '../utils'

const store = useAppStore()
</script>

<template>
  <section class="settings">
    <article class="card settings-card">
      <div class="section-title"><p>模型配置</p><h2>添加或更新模型</h2></div>
      <form class="form" @submit.prevent="store.configureModel">
        <label>快速模板
          <select v-model="store.providerTemplate" @change="store.applyProviderTemplate">
            <option value="local">本地模型（Ollama / LM Studio / vLLM）</option>
            <option value="deepseek">DeepSeek API</option>
            <option value="custom">自定义 OpenAI-compatible</option>
          </select>
        </label>
        <p v-if="store.providerTemplate === 'deepseek'" class="hint">已填写 DeepSeek 地址；补充 API Key 即可保存。</p>
        <label>名称<input v-model="store.deploymentName" required></label>
        <label>Base URL<input v-model="store.deploymentEndpoint" required></label>
        <label>模型 ID<input v-model="store.deploymentModel" required></label>
        <label>API Key <small>仅保存到 Windows 凭据管理器</small><input v-model="store.apiKey" type="password" placeholder="sk-…"></label>
        <button class="primary-button" :disabled="store.busy">保存模型</button>
      </form>
    </article>

    <article class="card settings-card">
      <div class="section-title"><p>连接检查</p><h2>{{ store.selectedDeployment?.name ?? '选择一个模型' }}</h2></div>
      <p class="muted">检查会验证服务连通性和模型能力。</p>
      <button class="secondary-button" :disabled="store.busy || !store.deploymentID" @click="store.probeModel">开始能力检查</button>
      <div v-if="store.capability" class="capabilities">
        <span>流式输出 <b>{{ store.capability.supports_streaming ? '支持' : '未支持' }}</b></span>
        <span>工具调用 <b>{{ store.capability.supports_tools ? '支持' : '未支持' }}</b></span>
        <span>上下文窗口 <b>{{ store.capability.reliable_context_tokens || '未报告' }}</b></span>
      </div>
    </article>

    <article class="card workspace-card">
      <div class="section-title split"><div><p>工作目录</p><h2>授权本地目录</h2></div><button class="text-button" @click="store.showWorkspaceForm = !store.showWorkspaceForm">添加工作目录</button></div>
      <form v-if="store.showWorkspaceForm" class="workspace-form" @submit.prevent="store.createWorkspace">
        <input v-model="store.workspaceName" placeholder="显示名称（可选）">
        <input v-model="store.workspacePath" required placeholder="目录绝对路径">
        <button class="secondary-button" :disabled="store.busy">授权</button>
      </form>
      <div class="workspace-list">
        <button v-for="item in store.workspaces" :key="item.id" :class="{ selected: store.workspaceID === item.id }" @click="store.selectWorkspace(item.id)"><b>{{ item.name }}</b><small>{{ item.root_path }}</small></button>
        <p v-if="!store.workspaces.length" class="muted">尚未授权任何目录。</p>
      </div>
    </article>

    <article class="card diagnostics-card">
      <div class="section-title split"><div><p>运行诊断</p><h2>本地日志</h2></div><button class="text-button" @click="store.refreshDiagnosticLogs">刷新</button></div>
      <p class="muted">用于定位发送、模型连接与界面异常。日志仅存本机，密钥会自动脱敏。</p>
      <div class="diagnostic-list">
        <div v-for="(entry, index) in store.diagnosticLogs" :key="`${entry.timestamp}-${index}`" :class="entry.level.toLowerCase()">
          <b>{{ entry.level === 'ERROR' ? '错误' : '信息' }} · {{ entry.component }}</b>
          <span>{{ entry.message }}</span>
          <small>{{ timeText(entry.timestamp) }}</small>
        </div>
        <p v-if="!store.diagnosticLogs.length" class="muted">尚无日志。点击刷新读取最新记录。</p>
      </div>
    </article>
  </section>
</template>
