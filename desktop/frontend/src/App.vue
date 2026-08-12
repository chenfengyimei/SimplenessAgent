<script lang="ts" setup>
import { computed, nextTick, onMounted, ref } from 'vue'
import { marked } from 'marked'
import DOMPurify from 'dompurify'

marked.setOptions({ breaks: true, gfm: true })

function renderMarkdown(text: string): string {
  if (!text) return ''
  return DOMPurify.sanitize(marked.parse(text, { async: false }) as string)
}

type PermissionMode = 'PLAN' | 'EDIT' | 'DEVELOPMENT'
type Workspace = { id: string; name: string; root_path: string }
type Deployment = { deployment_id: string; name: string; endpoint: string; model: string }
type Capability = { supports_tools: boolean; supports_streaming: boolean; reliable_context_tokens: number }
type Conversation = { id: string; workspace_id: string; title: string; goal: string; status: string; updated_at: string; spec?: { permission_profile_id?: string } }
type Message = { message_id: string; conversation_id: string; turn_task_id?: string; role: 'user' | 'assistant'; content: string; created_at: string }
type Event = { event_type: string; timestamp: string; sequence: number }
type Step = { step_id: string; title?: string; status: string; artifact_ids: string[]; evidence_ids: string[] }
type Snapshot = { task: { id: string; title: string; goal: string; status: string }; steps: Step[]; events: Event[] }
type PendingWrite = { task_id: string; step_id: string; path: string; content: string }
type PendingWriteBatch = { task_id: string; step_id: string; writes: PendingWrite[] }
type PendingCommand = { task_id: string; step_id: string; command: string; arguments: string[]; timeout_ms: number }
type Turn = { snapshot: Snapshot; report: { summary: string; tool_name: string; files: string[]; truncated: boolean; pending_write?: PendingWriteBatch; pending_command?: PendingCommand } }
type ConversationView = { conversation: Conversation; messages: Message[]; turns: Turn[] }
type DiagnosticEntry = { timestamp: string; level: string; component: string; message: string; fields?: Record<string, string> }

const workspaces = ref<Workspace[]>([])
const deployments = ref<Deployment[]>([])
const conversations = ref<Conversation[]>([])
const activeConversation = ref<ConversationView | null>(null)
const activePanel = ref<'chat' | 'settings'>('chat')
const workspaceID = ref('')
const deploymentID = ref('')
const permissionMode = ref<PermissionMode>('EDIT')
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
const diagnosticLogs = ref<DiagnosticEntry[]>([])
const storageKey = 'simplenessagent.selected-deployment-id'

const selectedWorkspace = computed(() => workspaces.value.find((item) => item.id === workspaceID.value) ?? null)
const selectedDeployment = computed(() => deployments.value.find((item) => item.deployment_id === deploymentID.value) ?? null)
const groupedConversations = computed(() => workspaces.value.map((workspace) => ({ workspace, conversations: conversations.value.filter((item) => item.workspace_id === workspace.id) })))
const isNewConversation = computed(() => !activeConversation.value)
const canSend = computed(() => Boolean(prompt.value.trim() && workspaceID.value && !busy.value))
const turnMap = computed(() => new Map((activeConversation.value?.turns ?? []).filter((turn) => turn?.snapshot?.task?.id).map((turn) => [turn.snapshot.task.id, turn])))

function fail(cause: unknown) { error.value = String(cause); notice.value = '' }
function clearFeedback() { error.value = ''; notice.value = '' }
function chooseDeployment(id: string) { deploymentID.value = id; capability.value = null; if (id) localStorage.setItem(storageKey, id) }
function selectWorkspace(id: string) { workspaceID.value = id; activePanel.value = 'chat'; clearFeedback() }
function toggleWorkspace(id: string) { collapsed.value[id] = !collapsed.value[id] }
function modeLabel(mode = permissionMode.value) { return ({ PLAN: '计划模式', EDIT: '编辑模式', DEVELOPMENT: '开发模式' } as Record<PermissionMode, string>)[mode] }
function modeHint(mode = permissionMode.value) { return ({ PLAN: '仅读取文件与结构，不执行命令、不修改文件', EDIT: '修改与项目命令均须先确认', DEVELOPMENT: '允许受限命令和工作区直接修改' } as Record<PermissionMode, string>)[mode] }
function knownMode(value?: string): PermissionMode | null { return value === 'PLAN' || value === 'EDIT' || value === 'DEVELOPMENT' ? value : null }
function statusText(status: string) { return ({ READY: '就绪', RUNNING: '执行中', WAITING_APPROVAL: '等待确认', COMPLETED: '已完成', FAILED: '失败' } as Record<string, string>)[status] ?? status }
function eventText(event: string) { return ({ TASK_CREATED: '创建执行回合', TASK_STATUS_CHANGED: '更新任务状态', PLAN_CREATED: '生成执行计划', STEP_STATUS_CHANGED: '更新步骤状态', TOOL_APPROVED: '确认操作', TOOL_INTENT_RECORDED: '记录执行意图', ARTIFACT_SAVED: '保存执行产物', EVIDENCE_SAVED: '保存验证证据', FINAL_REPORT_CREATED: '生成最终报告' } as Record<string, string>)[event] ?? event.replaceAll('_', ' ') }
function timeText(value: string) { return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '' }
function turnFor(message: Message) { return message.turn_task_id ? turnMap.value.get(message.turn_task_id) : undefined }
function artifactCount(turn?: Turn) { return turn?.snapshot?.steps?.reduce((total, step) => total + (step.artifact_ids?.length ?? 0), 0) ?? 0 }
function evidenceCount(turn?: Turn) { return turn?.snapshot?.steps?.reduce((total, step) => total + (step.evidence_ids?.length ?? 0), 0) ?? 0 }
function reportSummary(turn?: Turn) { return turn?.report?.summary || '本轮执行信息已保存，但没有可展示的摘要。' }
function reportTool(turn?: Turn) { return turn?.report?.tool_name || '未调用工具' }
function pendingWriteFor(turn?: Turn) { return turn?.report?.pending_write }
function pendingCommandFor(turn?: Turn) { return turn?.report?.pending_command }
function commandText(command?: PendingCommand) { if (!command) return ''; const labels: Record<string, string> = { go_test: 'go test', go_vet: 'go vet', npm_test: 'npm test', npm_build: 'npm run build' }; return `${labels[command.command] ?? command.command}${command.arguments?.length ? ` ${command.arguments.join(' ')}` : ''}` }
function clientLog(level: 'info' | 'error', message: string, fields: Record<string, string> = {}) { try { window.go?.main?.App?.RecordClientLog(level, message, fields) } catch (_) { /* diagnostics must never break the UI */ } }

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
  } catch (cause) { clientLog('error', '刷新工作台数据失败', { error: String(cause) }); fail(cause) }
}
async function refreshDiagnosticLogs() { try { diagnosticLogs.value = await window.go.main.App.ListDiagnosticLogs(120) as DiagnosticEntry[] } catch (cause) { clientLog('error', '读取诊断日志失败', { error: String(cause) }); fail(cause) } }
async function openConversation(conversation: Conversation) {
  try {
    busy.value = true; clearFeedback()
    activeConversation.value = await window.go.main.App.GetConversation(conversation.id) as ConversationView
    workspaceID.value = conversation.workspace_id
    const storedMode = knownMode(activeConversation.value.conversation.spec?.permission_profile_id)
    if (storedMode) permissionMode.value = storedMode
    activePanel.value = 'chat'; await scrollChat()
  } catch (cause) { clientLog('error', '打开会话失败', { conversation_id: conversation.id, error: String(cause) }); fail(cause) } finally { busy.value = false }
}
function newConversation() { activeConversation.value = null; prompt.value = ''; clearFeedback(); activePanel.value = 'chat'; clientLog('info', '用户开始新会话', { workspace_id: workspaceID.value, permission_mode: permissionMode.value }) }
async function sendMessage() {
  const text = prompt.value.trim()
  if (!canSend.value) return
  prompt.value = ''; clearFeedback()
  try {
    busy.value = true
    const result = activeConversation.value
      ? await window.go.main.App.SendConversationMessage(activeConversation.value.conversation.id, text, deploymentID.value, permissionMode.value)
      : await window.go.main.App.StartConversation(workspaceID.value, text, deploymentID.value, permissionMode.value)
    const next = result as ConversationView
    if (!next?.conversation?.id || !Array.isArray(next.messages)) throw new Error('桌面核心返回了不完整的会话数据；详情已写入诊断日志。')
    activeConversation.value = next; notice.value = '本轮已完成，执行过程和结果已保存到会话。'; await refresh()
  } catch (cause) { clientLog('error', '发送消息或渲染本轮结果失败', { conversation_id: activeConversation.value?.conversation?.id ?? '', permission_mode: permissionMode.value, error: String(cause) }); fail(cause) } finally { busy.value = false; await scrollChat() }
}
async function approveWrite(turn?: Turn) {
  const pending = pendingWriteFor(turn); if (!pending || busy.value) return
  try { busy.value = true; clearFeedback(); activeConversation.value = await window.go.main.App.ApproveConversationWrite(pending.task_id, pending.step_id) as ConversationView; notice.value = '已批准工作区修改，系统已执行并完成验证。'; await refresh() }
  catch (cause) { clientLog('error', '批准工作区修改失败', { error: String(cause) }); fail(cause) } finally { busy.value = false; await scrollChat() }
}
async function approveCommand(turn?: Turn) {
  const pending = pendingCommandFor(turn); if (!pending || busy.value) return
  try { busy.value = true; clearFeedback(); activeConversation.value = await window.go.main.App.ApproveConversationCommand(pending.task_id, pending.step_id) as ConversationView; notice.value = '已批准项目命令，系统已执行并保存输出。'; await refresh() }
  catch (cause) { clientLog('error', '批准项目命令失败', { error: String(cause) }); fail(cause) } finally { busy.value = false; await scrollChat() }
}
function applyProviderTemplate() { if (providerTemplate.value === 'deepseek') { deploymentName.value = 'DeepSeek API'; deploymentEndpoint.value = 'https://api.deepseek.com'; deploymentModel.value = 'deepseek-chat' } else if (providerTemplate.value === 'local') { deploymentName.value = '本地模型'; deploymentEndpoint.value = 'http://127.0.0.1:8080/v1'; deploymentModel.value = '' } else { deploymentName.value = '自定义模型'; deploymentEndpoint.value = ''; deploymentModel.value = '' } }
async function configureModel() { try { busy.value = true; const item = await window.go.main.App.ConfigureOpenAICompatibleDeployment(deploymentName.value, deploymentEndpoint.value, deploymentModel.value, apiKey.value) as Deployment; chooseDeployment(item.deployment_id); apiKey.value = ''; notice.value = '模型配置已保存，密钥仅保存在 Windows 凭据管理器。'; await refresh() } catch (cause) { clientLog('error', '保存模型失败', { error: String(cause) }); fail(cause) } finally { busy.value = false } }
async function probeModel() { try { busy.value = true; capability.value = await window.go.main.App.ProbeDeployment(deploymentID.value) as Capability; notice.value = '能力检查完成。' } catch (cause) { clientLog('error', '能力检查失败', { error: String(cause) }); fail(cause) } finally { busy.value = false } }
async function createWorkspace() { try { busy.value = true; const item = await window.go.main.App.CreateWorkspace(workspaceName.value, workspacePath.value) as Workspace; workspaceID.value = item.id; workspaceName.value = ''; workspacePath.value = ''; showWorkspaceForm.value = false; notice.value = '工作目录已添加。'; await refresh() } catch (cause) { clientLog('error', '添加工作目录失败', { error: String(cause) }); fail(cause) } finally { busy.value = false } }
function openSettings() { activePanel.value = 'settings'; refreshDiagnosticLogs() }
onMounted(() => { window.addEventListener('error', (event) => clientLog('error', '前端未捕获异常', { error: event.message, source: event.filename, line: String(event.lineno) })); window.addEventListener('unhandledrejection', (event) => clientLog('error', '前端未处理的异步异常', { error: String(event.reason) })); refresh() })
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar">
      <div class="brand"><div class="brand-mark">S</div><div><strong>Simpleness</strong><small>智能工作台</small></div></div>
      <button class="new-chat" :class="{ active: isNewConversation && activePanel === 'chat' }" @click="newConversation"><span>✎</span> 新对话</button>
      <nav><button :class="{ active: activePanel === 'settings' }" @click="openSettings"><span>◇</span> 模型与设置</button></nav>
      <section class="projects"><p>项目</p><div v-for="group in groupedConversations" :key="group.workspace.id" class="project-group"><div class="project-head" :class="{ selected: workspaceID === group.workspace.id }"><button @click="selectWorkspace(group.workspace.id)"><span>▱</span><b>{{ group.workspace.name }}</b><em>{{ group.conversations.length }}</em></button><button class="collapse" @click="toggleWorkspace(group.workspace.id)">{{ collapsed[group.workspace.id] ? '›' : '⌄' }}</button></div><div v-show="!collapsed[group.workspace.id]" class="conversation-list"><button v-for="conversation in group.conversations" :key="conversation.id" :class="{ selected: activeConversation?.conversation.id === conversation.id }" @click="openConversation(conversation)">{{ conversation.title || '未命名会话' }}</button><small v-if="!group.conversations.length">该目录尚无会话</small></div></div><p v-if="!workspaces.length" class="empty-project">请先在“模型与设置”授权工作目录。</p></section>
      <div class="core-status"><i></i> 核心服务已连接</div>
    </aside>

    <main class="main">
      <header class="topbar"><span class="crumb">{{ activePanel === 'settings' ? '模型与设置' : (selectedWorkspace?.name || '选择工作目录') }}</span><span v-if="activePanel === 'chat' && activeConversation" class="conversation-name">{{ activeConversation.conversation.title }}</span></header>
      <div v-if="notice" class="banner success">{{ notice }}</div><div v-if="error" class="banner danger">{{ error }}</div>

      <section v-if="activePanel === 'chat'" class="conversation-shell">
        <div ref="chatBody" class="chat-body">
          <div v-if="isNewConversation" class="welcome"><div class="welcome-icon">S</div><h1>从一个问题开始</h1><p>{{ selectedWorkspace ? `当前工作目录：${selectedWorkspace.name}` : '请先从左侧选择一个工作目录。' }}</p><div class="suggestions" v-if="selectedWorkspace"><button @click="prompt = '分析当前工作目录的项目结构，并列出关键文件'">分析项目结构</button><button @click="prompt = '阅读 README 并说明如何运行项目'">阅读 README</button><button @click="prompt = '检查当前工作目录的待办事项和下一步建议'">给出开发建议</button></div></div>
          <article v-for="message in activeConversation?.messages ?? []" :key="message.message_id" class="message" :class="message.role"><div class="avatar">{{ message.role === 'user' ? '你' : 'S' }}</div><div class="message-content"><div class="message-meta"><b>{{ message.role === 'user' ? '你' : 'Simpleness' }}</b><time>{{ timeText(message.created_at) }}</time></div><div class="markdown-body" v-html="renderMarkdown(message.content)"></div><section v-if="message.role === 'assistant' && turnFor(message)" class="turn-result"><div class="turn-head"><b>本轮执行结果</b><span class="status-pill" :class="turnFor(message)?.snapshot?.task?.status?.toLowerCase()">{{ statusText(turnFor(message)?.snapshot?.task?.status ?? '') }}</span></div><div class="result-stats"><span>已调用：{{ reportTool(turnFor(message)) }}</span><span>{{ artifactCount(turnFor(message)) }} 个产物</span><span>{{ evidenceCount(turnFor(message)) }} 条证据</span></div><details><summary>查看 Agent 操作记录</summary><ol class="operation-log"><li v-for="event in (turnFor(message)?.snapshot?.events ?? [])" :key="event.sequence"><b>{{ eventText(event.event_type) }}</b><time>{{ timeText(event.timestamp) }}</time></li></ol></details><section v-if="pendingWriteFor(turnFor(message))" class="approval-card"><div><b>等待确认工作区修改</b><span>本批 {{ pendingWriteFor(turnFor(message))?.writes.length }} 个文件</span></div><p>确认前不会修改文件；若任一文件已变化，系统会拒绝覆盖整个批次。</p><details><summary>查看拟写入内容</summary><div class="proposal-files"><section v-for="write in pendingWriteFor(turnFor(message))?.writes" :key="write.path"><code>{{ write.path }}</code><pre>{{ write.content }}</pre></section></div></details><button class="primary-button" :disabled="busy" @click="approveWrite(turnFor(message))">确认并写入全部文件</button></section><section v-else-if="pendingCommandFor(turnFor(message))" class="approval-card command"><div><b>等待确认项目命令</b><span>{{ pendingCommandFor(turnFor(message))?.timeout_ms }} ms 上限</span></div><code>{{ commandText(pendingCommandFor(turnFor(message))) }}</code><p>该命令只会在当前工作目录中执行一次，输出会被限额保存到执行记录。</p><button class="primary-button" :disabled="busy" @click="approveCommand(turnFor(message))">确认并执行命令</button></section></section></div></article>
          <div v-if="busy" class="thinking"><i></i><i></i><i></i> Agent 正在处理本轮请求…</div>
        </div>
        <div class="composer"><textarea v-model="prompt" :disabled="busy" @keydown.ctrl.enter.prevent="sendMessage" placeholder="给 Agent 发送任务或问题；Ctrl + Enter 发送"></textarea><div class="composer-footer"><div class="composer-tools"><button class="round-button" title="新对话" @click="newConversation">＋</button><span class="workspace-chip">{{ selectedWorkspace?.name || '未选择目录' }}</span><select :value="deploymentID" :disabled="busy" @change="chooseDeployment(($event.target as HTMLSelectElement).value)"><option value="">不使用模型（确定性侦察）</option><option v-for="item in deployments" :key="item.deployment_id" :value="item.deployment_id">{{ item.name }} · {{ item.model }}</option></select><select v-model="permissionMode" :disabled="busy" :title="modeHint()"><option value="PLAN">计划模式（只读）</option><option value="EDIT">编辑模式（需确认）</option><option value="DEVELOPMENT">开发模式（直接执行）</option></select></div><button class="send-button" :disabled="!canSend" @click="sendMessage">发送 <span>↑</span></button></div><small class="mode-note">{{ modeLabel() }}：{{ modeHint() }}</small></div>
      </section>

      <section v-else class="settings">
        <article class="card settings-card"><div class="section-title"><p>模型配置</p><h2>添加或更新模型</h2></div><form class="form" @submit.prevent="configureModel"><label>快速模板<select v-model="providerTemplate" @change="applyProviderTemplate"><option value="local">本地模型（Ollama / LM Studio / vLLM）</option><option value="deepseek">DeepSeek API</option><option value="custom">自定义 OpenAI-compatible</option></select></label><p v-if="providerTemplate === 'deepseek'" class="hint">已填写 DeepSeek 地址；补充 API Key 即可保存。</p><label>名称<input v-model="deploymentName" required></label><label>Base URL<input v-model="deploymentEndpoint" required></label><label>模型 ID<input v-model="deploymentModel" required></label><label>API Key <small>仅保存到 Windows 凭据管理器</small><input v-model="apiKey" type="password" placeholder="sk-…"></label><button class="primary-button" :disabled="busy">保存模型</button></form></article>
        <article class="card settings-card"><div class="section-title"><p>连接检查</p><h2>{{ selectedDeployment?.name ?? '选择一个模型' }}</h2></div><p class="muted">检查会验证服务连通性和模型能力。</p><button class="secondary-button" :disabled="busy || !deploymentID" @click="probeModel">开始能力检查</button><div v-if="capability" class="capabilities"><span>流式输出 <b>{{ capability.supports_streaming ? '支持' : '未支持' }}</b></span><span>工具调用 <b>{{ capability.supports_tools ? '支持' : '未支持' }}</b></span><span>上下文窗口 <b>{{ capability.reliable_context_tokens || '未报告' }}</b></span></div></article>
        <article class="card workspace-card"><div class="section-title split"><div><p>工作目录</p><h2>授权本地目录</h2></div><button class="text-button" @click="showWorkspaceForm = !showWorkspaceForm">添加工作目录</button></div><form v-if="showWorkspaceForm" class="workspace-form" @submit.prevent="createWorkspace"><input v-model="workspaceName" placeholder="显示名称（可选）"><input v-model="workspacePath" required placeholder="目录绝对路径"><button class="secondary-button" :disabled="busy">授权</button></form><div class="workspace-list"><button v-for="item in workspaces" :key="item.id" :class="{ selected: workspaceID === item.id }" @click="selectWorkspace(item.id)"><b>{{ item.name }}</b><small>{{ item.root_path }}</small></button><p v-if="!workspaces.length" class="muted">尚未授权任何目录。</p></div></article>
        <article class="card diagnostics-card"><div class="section-title split"><div><p>运行诊断</p><h2>本地日志</h2></div><button class="text-button" @click="refreshDiagnosticLogs">刷新</button></div><p class="muted">用于定位发送、模型连接与界面异常。日志仅存本机，密钥会自动脱敏。</p><div class="diagnostic-list"><div v-for="(entry, index) in diagnosticLogs" :key="`${entry.timestamp}-${index}`" :class="entry.level.toLowerCase()"><b>{{ entry.level === 'ERROR' ? '错误' : '信息' }} · {{ entry.component }}</b><span>{{ entry.message }}</span><small>{{ timeText(entry.timestamp) }}</small></div><p v-if="!diagnosticLogs.length" class="muted">尚无日志。点击刷新读取最新记录。</p></div></article>
      </section>
    </main>
  </div>
</template>
