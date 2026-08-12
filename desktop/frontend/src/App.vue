<script lang="ts" setup>
import { computed, nextTick, onMounted, ref } from 'vue'

type Workspace = { id: string; name: string; root_path: string }
type Deployment = { deployment_id: string; name: string; endpoint: string; model: string }
type Capability = { supports_tools: boolean; supports_streaming: boolean; reliable_context_tokens: number }
type Conversation = { id: string; workspace_id: string; title: string; goal: string; status: string; updated_at: string }
type Message = { message_id: string; conversation_id: string; turn_task_id?: string; role: 'user' | 'assistant'; content: string; created_at: string }
type Event = { event_type: string; timestamp: string; sequence: number; payload?: Record<string, unknown> }
type Step = { step_id: string; title?: string; status: string; artifact_ids: string[]; evidence_ids: string[] }
type Snapshot = { task: { id: string; title: string; goal: string; status: string }; plan?: { summary: string; steps: { step_id: string; title: string; goal: string }[] }; steps: Step[]; events: Event[] }
type Turn = { snapshot: Snapshot; report: { summary: string; tool_name: string; files: string[]; truncated: boolean } }
type ConversationView = { conversation: Conversation; messages: Message[]; turns: Turn[] }

const workspaces = ref<Workspace[]>([])
const deployments = ref<Deployment[]>([])
const conversations = ref<Conversation[]>([])
const activeConversation = ref<ConversationView | null>(null)
const activePanel = ref<'chat' | 'settings'>('chat')
const workspaceID = ref('')
const deploymentID = ref('')
const prompt = ref('')
const busy = ref(false)
const error = ref('')
const notice = ref('')
const collapsed = ref<Record<string, boolean>>({})
const chatBody = ref<HTMLElement | null>(null)
const showWorkspaceForm = ref(false)
const workspaceName = ref('')
const workspacePath = ref('')
const deploymentName = ref('本地模型')
const deploymentEndpoint = ref('http://127.0.0.1:8080/v1')
const deploymentModel = ref('')
const apiKey = ref('')
const providerTemplate = ref<'local' | 'deepseek' | 'custom'>('local')
const capability = ref<Capability | null>(null)
const storageKey = 'simplenessagent.selected-deployment-id'

const selectedWorkspace = computed(() => workspaces.value.find((item) => item.id === workspaceID.value) ?? null)
const selectedDeployment = computed(() => deployments.value.find((item) => item.deployment_id === deploymentID.value) ?? null)
const groupedConversations = computed(() => workspaces.value.map((workspace) => ({ workspace, conversations: conversations.value.filter((item) => item.workspace_id === workspace.id) })))
const isNewConversation = computed(() => !activeConversation.value)
const canSend = computed(() => Boolean(prompt.value.trim() && workspaceID.value && !busy.value))
const turnMap = computed(() => new Map((activeConversation.value?.turns ?? []).map((turn) => [turn.snapshot.task.id, turn])))

function fail(cause: unknown) { error.value = String(cause); notice.value = '' }
function clearFeedback() { error.value = ''; notice.value = '' }
function chooseDeployment(id: string) { deploymentID.value = id; capability.value = null; if (id) localStorage.setItem(storageKey, id) }
function toggleWorkspace(id: string) { collapsed.value[id] = !collapsed.value[id] }
function statusText(status: string) { return ({ CREATED: '已创建', READY: '就绪', RUNNING: '执行中', COMPLETED: '已完成', FAILED: '失败' } as Record<string, string>)[status] ?? status }
function eventText(event: string) { return ({ TASK_CREATED: '创建执行回合', TASK_STATUS_CHANGED: '更新任务状态', PLAN_CREATED: '生成只读计划', STEP_STATUS_CHANGED: '更新步骤状态', ARTIFACT_SAVED: '保存执行产物', EVIDENCE_SAVED: '保存验证证据', FINAL_REPORT_CREATED: '生成最终报告' } as Record<string, string>)[event] ?? event.replaceAll('_', ' ') }
function timeText(value: string) { return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '' }
function turnFor(message: Message) { return message.turn_task_id ? turnMap.value.get(message.turn_task_id) : undefined }
function artifactCount(turn?: Turn) { return turn?.snapshot.steps.reduce((total, step) => total + step.artifact_ids.length, 0) ?? 0 }
function evidenceCount(turn?: Turn) { return turn?.snapshot.steps.reduce((total, step) => total + step.evidence_ids.length, 0) ?? 0 }

async function scrollChat() { await nextTick(); chatBody.value?.scrollTo({ top: chatBody.value.scrollHeight, behavior: 'smooth' }) }
async function refresh() {
  try {
    const [workspaceItems, deploymentItems, conversationItems] = await Promise.all([window.go.main.App.ListWorkspaces(), window.go.main.App.ListDeployments(), window.go.main.App.ListConversations()])
    workspaces.value = workspaceItems as Workspace[]
    deployments.value = deploymentItems as Deployment[]
    conversations.value = conversationItems as Conversation[]
    if (!workspaceID.value && workspaces.value.length) workspaceID.value = workspaces.value[0].id
    if (!deployments.value.some((item) => item.deployment_id === deploymentID.value)) {
      const saved = localStorage.getItem(storageKey) ?? ''
      chooseDeployment(deployments.value.some((item) => item.deployment_id === saved) ? saved : (deployments.value[0]?.deployment_id ?? ''))
    }
  } catch (cause) { fail(cause) }
}
async function openConversation(conversation: Conversation) {
  try {
    busy.value = true
    clearFeedback()
    activeConversation.value = await window.go.main.App.GetConversation(conversation.id) as ConversationView
    workspaceID.value = conversation.workspace_id
    activePanel.value = 'chat'
    await scrollChat()
  } catch (cause) { fail(cause) } finally { busy.value = false }
}
function newConversation() { activeConversation.value = null; prompt.value = ''; clearFeedback(); activePanel.value = 'chat' }
async function sendMessage() {
  const text = prompt.value.trim()
  if (!canSend.value) return
  prompt.value = ''
  clearFeedback()
  try {
    busy.value = true
    const result = activeConversation.value
      ? await window.go.main.App.SendConversationMessage(activeConversation.value.conversation.id, text, deploymentID.value)
      : await window.go.main.App.StartConversation(workspaceID.value, text, deploymentID.value)
    activeConversation.value = result as ConversationView
    notice.value = '本轮已完成，执行过程和结果已保存到会话。'
    await refresh()
  } catch (cause) { fail(cause) } finally { busy.value = false; await scrollChat() }
}
function applyProviderTemplate() {
  if (providerTemplate.value === 'deepseek') { deploymentName.value = 'DeepSeek API'; deploymentEndpoint.value = 'https://api.deepseek.com'; deploymentModel.value = 'deepseek-chat' }
  else if (providerTemplate.value === 'local') { deploymentName.value = '本地模型'; deploymentEndpoint.value = 'http://127.0.0.1:8080/v1'; deploymentModel.value = '' }
  else { deploymentName.value = '自定义模型'; deploymentEndpoint.value = ''; deploymentModel.value = '' }
}
async function configureModel() {
  try {
    busy.value = true
    const item = await window.go.main.App.ConfigureOpenAICompatibleDeployment(deploymentName.value, deploymentEndpoint.value, deploymentModel.value, apiKey.value) as Deployment
    chooseDeployment(item.deployment_id); apiKey.value = ''; notice.value = '模型配置已保存，密钥仅保存在 Windows 凭据管理器。'; await refresh()
  } catch (cause) { fail(cause) } finally { busy.value = false }
}
async function probeModel() {
  try { busy.value = true; capability.value = await window.go.main.App.ProbeDeployment(deploymentID.value) as Capability; notice.value = '能力检查完成。' }
  catch (cause) { fail(cause) } finally { busy.value = false }
}
async function createWorkspace() {
  try {
    busy.value = true
    const item = await window.go.main.App.CreateWorkspace(workspaceName.value, workspacePath.value) as Workspace
    workspaceID.value = item.id; workspaceName.value = ''; workspacePath.value = ''; showWorkspaceForm.value = false; notice.value = '工作目录已添加。'; await refresh()
  } catch (cause) { fail(cause) } finally { busy.value = false }
}
onMounted(refresh)
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar">
      <div class="brand"><div class="brand-mark">S</div><div><strong>Simpleness</strong><small>智能工作台</small></div></div>
      <button class="new-chat" @click="newConversation"><span>⌕</span> 新对话</button>
      <nav><button :class="{ active: activePanel === 'chat' }" @click="activePanel = 'chat'"><span>◉</span> 对话</button><button :class="{ active: activePanel === 'settings' }" @click="activePanel = 'settings'"><span>◇</span> 模型与设置</button></nav>
      <section class="projects"><p>项目</p><div v-for="group in groupedConversations" :key="group.workspace.id" class="project-group"><button class="project-head" @click="toggleWorkspace(group.workspace.id)"><span>{{ collapsed[group.workspace.id] ? '›' : '⌄' }}</span><b>▱ {{ group.workspace.name }}</b><em>{{ group.conversations.length }}</em></button><div v-show="!collapsed[group.workspace.id]" class="conversation-list"><button v-for="conversation in group.conversations" :key="conversation.id" :class="{ selected: activeConversation?.conversation.id === conversation.id }" @click="openConversation(conversation)">{{ conversation.title || '未命名会话' }}</button><small v-if="!group.conversations.length">还没有会话</small></div></div><p v-if="!workspaces.length" class="empty-project">请先在“模型与设置”添加工作目录。</p></section>
      <div class="core-status"><i></i> 核心服务已连接</div>
    </aside>

    <main class="main">
      <header class="topbar"><div><p class="eyebrow">SIMPLENESS AGENT</p><h1>{{ activePanel === 'chat' ? (activeConversation?.conversation.title || '新对话') : '模型与设置' }}</h1><p class="subtitle">{{ activePanel === 'chat' ? '选择工作目录后直接交代任务；每一轮输入、回复与操作都会留在当前会话。' : '配置模型连接、管理 API 密钥和已授权的工作目录。' }}</p></div><div v-if="activePanel === 'chat'" class="header-selects"><select v-model="workspaceID"><option disabled value="">选择工作目录</option><option v-for="item in workspaces" :key="item.id" :value="item.id">{{ item.name }}</option></select><select :value="deploymentID" @change="chooseDeployment(($event.target as HTMLSelectElement).value)"><option value="">不使用模型（只读侦察）</option><option v-for="item in deployments" :key="item.deployment_id" :value="item.deployment_id">{{ item.name }} · {{ item.model }}</option></select></div></header>
      <div v-if="notice" class="banner success">{{ notice }}</div><div v-if="error" class="banner danger">{{ error }}</div>

      <section v-if="activePanel === 'chat'" class="conversation-shell">
        <div ref="chatBody" class="chat-body">
          <div v-if="isNewConversation" class="welcome"><div class="welcome-icon">✦</div><h2>从一个问题开始</h2><p>新建会话后，后续消息都会保存在同一个会话里。Agent 会把规划、工具操作和可复核结果穿插显示在回复中。</p><div class="suggestions"><button @click="prompt = '分析当前工作目录的项目结构，并列出关键文件'">分析项目结构</button><button @click="prompt = '查看工作目录并总结下一步开发建议'">查看开发建议</button><button @click="prompt = '读取 README 并说明如何运行项目'">阅读 README</button></div></div>
          <article v-for="message in activeConversation?.messages ?? []" :key="message.message_id" class="message" :class="message.role"><div class="avatar">{{ message.role === 'user' ? '你' : 'S' }}</div><div class="message-content"><div class="message-meta"><b>{{ message.role === 'user' ? '你' : 'Simpleness' }}</b><time>{{ timeText(message.created_at) }}</time></div><p>{{ message.content }}</p><section v-if="message.role === 'assistant' && turnFor(message)" class="turn-result"><div class="turn-head"><div><span class="tool-dot">✓</span><b>本轮执行结果</b></div><span class="status-pill" :class="turnFor(message)?.snapshot.task.status.toLowerCase()">{{ statusText(turnFor(message)?.snapshot.task.status ?? '') }}</span></div><p class="report-summary">{{ turnFor(message)?.report.summary }}</p><div class="result-stats"><span>已调用：{{ turnFor(message)?.report.tool_name || '只读工具' }}</span><span>{{ artifactCount(turnFor(message)) }} 个产物</span><span>{{ evidenceCount(turnFor(message)) }} 条证据</span></div><details><summary>查看 Agent 操作记录</summary><ol class="operation-log"><li v-for="event in turnFor(message)?.snapshot.events" :key="event.sequence"><b>{{ eventText(event.event_type) }}</b><time>{{ timeText(event.timestamp) }}</time></li></ol></details><details v-if="turnFor(message)?.report.files.length"><summary>查看发现的文件（{{ turnFor(message)?.report.files.length }}）</summary><div class="file-list"><code v-for="file in turnFor(message)?.report.files" :key="file">{{ file }}</code></div></details></section></div></article>
          <div v-if="busy" class="thinking"><i></i><i></i><i></i> Agent 正在整理本轮结果…</div>
        </div>
        <div class="composer"><textarea v-model="prompt" :disabled="busy" @keydown.ctrl.enter.prevent="sendMessage" placeholder="输入你的任务；Ctrl + Enter 发送"></textarea><div class="composer-footer"><small>{{ selectedWorkspace ? `工作目录：${selectedWorkspace.name}` : '请选择工作目录' }}<template v-if="selectedDeployment"> · 模型：{{ selectedDeployment.name }}</template></small><button class="send-button" :disabled="!canSend" @click="sendMessage">发送 <span>↵</span></button></div></div>
      </section>

      <section v-else class="settings">
        <article class="card settings-card"><div class="section-title"><p>模型配置</p><h2>添加或更新模型</h2></div><form class="form" @submit.prevent="configureModel"><label>快速模板<select v-model="providerTemplate" @change="applyProviderTemplate"><option value="local">本地模型（Ollama / LM Studio / vLLM）</option><option value="deepseek">DeepSeek API</option><option value="custom">自定义 OpenAI-compatible</option></select></label><p v-if="providerTemplate === 'deepseek'" class="hint">已填写 DeepSeek 地址；补充 API Key 即可保存。</p><label>名称<input v-model="deploymentName" required></label><label>Base URL<input v-model="deploymentEndpoint" required></label><label>模型 ID<input v-model="deploymentModel" required></label><label>API Key <small>仅保存到 Windows 凭据管理器</small><input v-model="apiKey" type="password" placeholder="sk-…"></label><button class="send-button" :disabled="busy">保存模型</button></form></article>
        <article class="card settings-card"><div class="section-title"><p>连接检查</p><h2>{{ selectedDeployment?.name ?? '选择一个模型' }}</h2></div><p class="muted">检查会验证服务连通性和模型能力。普通只读侦察不依赖模型也能运行。</p><button class="secondary-button" :disabled="busy || !deploymentID" @click="probeModel">开始能力检查</button><div v-if="capability" class="capabilities"><span>流式输出 <b>{{ capability.supports_streaming ? '支持' : '未支持' }}</b></span><span>工具调用 <b>{{ capability.supports_tools ? '支持' : '未支持' }}</b></span><span>上下文窗口 <b>{{ capability.reliable_context_tokens || '未报告' }}</b></span></div></article>
        <article class="card workspace-card"><div class="section-title split"><div><p>工作目录</p><h2>授权本地目录</h2></div><button class="text-button" @click="showWorkspaceForm = !showWorkspaceForm">添加工作目录</button></div><form v-if="showWorkspaceForm" class="workspace-form" @submit.prevent="createWorkspace"><input v-model="workspaceName" placeholder="显示名称（可选）"><input v-model="workspacePath" required placeholder="目录绝对路径"><button class="secondary-button" :disabled="busy">授权</button></form><div class="workspace-list"><div v-for="item in workspaces" :key="item.id"><b>{{ item.name }}</b><small>{{ item.root_path }}</small></div><p v-if="!workspaces.length" class="muted">尚未授权任何目录。</p></div></article>
      </section>
    </main>
  </div>
</template>

<style>
:root { font-family: Inter, "Microsoft YaHei", sans-serif; color: #e8eef9; background: #0b1019; font-synthesis: none; }
* { box-sizing: border-box; } body { margin: 0; min-width: 960px; background: #0b1019; } button, input, select, textarea { font: inherit; } button { cursor: pointer; }
.app-shell { min-height: 100vh; display: grid; grid-template-columns: 250px minmax(0, 1fr); background: radial-gradient(circle at 62% -20%, #183354 0, #0d1420 34%, #0b1019 72%); }
.sidebar { min-height: 100vh; border-right: 1px solid #263247; background: rgba(12, 18, 28, .92); padding: 22px 12px 16px; display: flex; flex-direction: column; }
.brand { display: flex; gap: 10px; align-items: center; padding: 3px 12px 24px; }.brand-mark { width: 34px; height: 34px; display: grid; place-items: center; border-radius: 10px; color: #072338; font-weight: 800; background: linear-gradient(135deg, #82e7d0, #5bb8ff); }.brand strong { display: block; font-size: 16px; }.brand small { display: block; color: #8291a9; font-size: 11px; margin-top: 2px; }
.new-chat { color: #eaf4ff; background: #26354b; border: 1px solid #334761; border-radius: 9px; width: 100%; padding: 11px 13px; text-align: left; margin-bottom: 11px; }.new-chat span { margin-right: 8px; color: #87ddff; font-weight: 900; }
nav { display: grid; gap: 3px; } nav button { color: #b3c1d5; background: transparent; border: 0; border-radius: 8px; padding: 10px 12px; text-align: left; } nav button span { color: #70c8ff; margin-right: 12px; } nav button.active { background: #26354b; color: #fff; }
.projects { flex: 1; padding: 22px 6px; overflow: auto; }.projects > p { margin: 0 6px 9px; color: #7d8aa0; font-size: 12px; }.project-group { margin-bottom: 10px; }.project-head { width: 100%; display: flex; gap: 7px; align-items: center; color: #c4d0df; border: 0; background: transparent; padding: 6px; text-align: left; }.project-head > span { color: #7f91a9; font-size: 18px; line-height: 12px; }.project-head b { font-size: 13px; font-weight: 500; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.project-head em { color: #6f8095; font-size: 11px; font-style: normal; }.conversation-list { margin: 2px 0 0 19px; display: grid; gap: 2px; }.conversation-list button { text-align: left; color: #aebcd1; border: 0; border-radius: 7px; padding: 8px 9px; background: transparent; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }.conversation-list button:hover, .conversation-list button.selected { background: #243247; color: #fff; }.conversation-list small, .empty-project { padding: 7px 9px; color: #718096; font-size: 12px; }
.core-status { color: #8da1b8; font-size: 12px; padding: 10px 12px 2px; }.core-status i { display: inline-block; width: 8px; height: 8px; border-radius: 50%; background: #5fe2a0; box-shadow: 0 0 8px #5fe2a0; margin-right: 7px; }
.main { min-width: 0; padding: 0 36px 32px; }.topbar { min-height: 112px; border-bottom: 1px solid #2a384c; display: flex; justify-content: space-between; align-items: center; gap: 24px; }.eyebrow { font-size: 11px; letter-spacing: .11em; color: #62c6ff; font-weight: 800; margin: 0 0 5px; }.topbar h1 { margin: 0; font-size: 27px; letter-spacing: -.04em; }.subtitle { color: #91a2b9; margin: 7px 0 0; font-size: 13px; }.header-selects { display: flex; gap: 9px; }.header-selects select, .form input, .form select, .workspace-form input { background: #141d2a; color: #e6effc; border: 1px solid #32425a; border-radius: 8px; padding: 10px 11px; outline: none; }.header-selects select { max-width: 250px; }
.banner { margin-top: 16px; padding: 11px 13px; border-radius: 8px; font-size: 13px; }.banner.success { background: #123227; border: 1px solid #2d7554; color: #baf5d4; }.banner.danger { background: #3d1c29; border: 1px solid #924157; color: #ffd3dc; }
.conversation-shell { width: min(100%, 1050px); height: calc(100vh - 145px); min-height: 620px; display: grid; grid-template-rows: 1fr auto; margin: 20px auto 0; border: 1px solid #2a394e; border-radius: 14px; overflow: hidden; background: rgba(16, 24, 36, .88); box-shadow: 0 16px 50px rgba(0,0,0,.2); }.chat-body { overflow-y: auto; padding: 32px max(28px, 7%); scroll-behavior: smooth; }.welcome { max-width: 680px; margin: 14vh auto 0; text-align: center; }.welcome-icon { width: 50px; height: 50px; display: grid; place-items: center; margin: auto; border-radius: 16px; background: linear-gradient(135deg, #78e7d4, #5aaaff); color: #0d2539; font-size: 25px; }.welcome h2 { margin: 17px 0 9px; font-size: 25px; }.welcome p { margin: 0 auto; color: #9eacc0; line-height: 1.75; max-width: 600px; }.suggestions { margin: 25px auto; display: flex; justify-content: center; flex-wrap: wrap; gap: 8px; }.suggestions button { border: 1px solid #34465e; color: #b6d9f3; background: #152335; border-radius: 20px; padding: 8px 12px; font-size: 12px; }
.message { display: grid; grid-template-columns: 32px minmax(0, 1fr); gap: 12px; max-width: 820px; margin: 0 auto 28px; }.avatar { width: 30px; height: 30px; border-radius: 9px; display: grid; place-items: center; font-size: 12px; font-weight: 800; background: #33445b; color: #dce9f8; }.message.user .avatar { background: #2d695d; color: #d6fff1; }.message-content { min-width: 0; }.message-meta { display: flex; gap: 10px; align-items: baseline; margin: 2px 0 7px; }.message-meta b { font-size: 13px; }.message-meta time, .operation-log time { color: #718199; font-size: 11px; }.message-content > p { margin: 0; white-space: pre-wrap; line-height: 1.7; color: #e6edf8; }.turn-result { margin-top: 13px; border: 1px solid #304258; border-radius: 10px; background: #111c2b; overflow: hidden; }.turn-head { display: flex; align-items: center; justify-content: space-between; padding: 12px 14px 0; }.turn-head > div { display: flex; align-items: center; gap: 8px; font-size: 13px; }.tool-dot { width: 19px; height: 19px; display: grid; place-items: center; border-radius: 50%; color: #072719; background: #72e7bd; font-weight: 900; }.status-pill { border-radius: 12px; padding: 3px 8px; font-size: 10px; font-weight: 800; color: #9bd8ff; background: #1b3854; }.status-pill.completed { color: #aaf3ce; background: #1a4434; }.status-pill.failed { color: #ffc1cc; background: #552837; }.report-summary { margin: 10px 14px; color: #c8d7e8; font-size: 13px; }.result-stats { display: flex; flex-wrap: wrap; gap: 8px 16px; padding: 0 14px 11px; font-size: 11px; color: #8fa6c0; }.turn-result details { border-top: 1px solid #27384e; }.turn-result summary { padding: 10px 14px; color: #a4c7e5; font-size: 12px; cursor: pointer; }.operation-log { list-style: none; margin: 0; padding: 0 14px 12px; display: grid; gap: 8px; }.operation-log li { display: flex; justify-content: space-between; gap: 12px; color: #b9c8db; font-size: 12px; }.operation-log b { font-weight: 500; }.file-list { padding: 0 14px 14px; display: flex; flex-wrap: wrap; gap: 6px; }.file-list code { color: #b9dcf8; background: #192b40; padding: 4px 7px; border-radius: 4px; font-size: 11px; overflow-wrap: anywhere; }.thinking { color: #98b2ce; max-width: 820px; margin: 0 auto; padding-left: 44px; font-size: 13px; }.thinking i { display: inline-block; width: 5px; height: 5px; background: #73d9ff; border-radius: 50%; margin-right: 3px; animation: pulse 1s infinite alternate; }.thinking i:nth-child(2) { animation-delay: .2s; }.thinking i:nth-child(3) { animation-delay: .4s; } @keyframes pulse { to { opacity: .2; transform: translateY(-3px); } }
.composer { border-top: 1px solid #29394f; padding: 14px 18px 15px; background: #101925; }.composer textarea { width: 100%; min-height: 62px; resize: vertical; color: #edf5ff; background: #141f2e; border: 1px solid #32435a; outline: none; padding: 11px; border-radius: 9px; line-height: 1.5; }.composer textarea:focus { border-color: #60bff1; box-shadow: 0 0 0 3px #25507244; }.composer-footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 9px 1px 0; }.composer-footer small { color: #8091a8; font-size: 11px; }.send-button { border: 0; border-radius: 8px; color: #082235; font-weight: 800; padding: 9px 15px; background: linear-gradient(135deg, #79e5d2, #63b7ff); }.send-button:disabled, .secondary-button:disabled { cursor: not-allowed; opacity: .45; }.send-button span { margin-left: 7px; }
.settings { max-width: 850px; margin: 25px auto; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }.card { border: 1px solid #2d3e54; background: #111b2a; border-radius: 12px; padding: 21px; }.settings-card:first-child { grid-row: span 2; }.section-title p { color: #66c6ff; font-size: 11px; font-weight: 800; letter-spacing: .08em; margin: 0 0 5px; }.section-title h2 { margin: 0; font-size: 19px; }.form { display: grid; gap: 13px; margin-top: 20px; }.form label { display: grid; gap: 6px; color: #bac8da; font-size: 12px; }.form label small { color: #7d91a9; }.hint, .muted { color: #92a2b5; font-size: 12px; line-height: 1.65; }.secondary-button { margin-top: 12px; border-radius: 8px; padding: 9px 13px; color: #c6e9ff; border: 1px solid #3c5875; background: #182a40; }.capabilities { display: grid; gap: 7px; margin-top: 16px; }.capabilities span { display: flex; justify-content: space-between; color: #aebdd0; font-size: 12px; }.capabilities b { color: #87e3c0; }.split { display: flex; justify-content: space-between; gap: 10px; }.text-button { color: #7ed5ff; border: 0; background: none; }.workspace-form { display: grid; gap: 8px; margin: 16px 0; }.workspace-list { display: grid; gap: 9px; margin-top: 16px; }.workspace-list > div { border-top: 1px solid #28384d; padding-top: 10px; }.workspace-list b, .workspace-list small { display: block; }.workspace-list b { font-size: 13px; }.workspace-list small { margin-top: 4px; color: #8090a7; font-size: 11px; overflow-wrap: anywhere; }
@media (max-width: 1060px) { body { min-width: 760px; }.main { padding: 0 20px 22px; }.app-shell { grid-template-columns: 215px minmax(0, 1fr); }.header-selects { max-width: 430px; }.settings { grid-template-columns: 1fr; }.settings-card:first-child { grid-row: auto; } }
</style>
