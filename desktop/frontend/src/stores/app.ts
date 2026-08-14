import { defineStore } from 'pinia'
import { computed, ref, nextTick } from 'vue'
import type {
  Artifact,
  Capability,
  Conversation,
  ConversationView,
  Deployment,
  DiagnosticEntry,
  PermissionMode,
  Workspace,
} from '../types'
import { clientLog, knownMode } from '../utils'

const STORAGE_KEY = 'simplenessagent.selected-deployment-id'

export type AgentStatus = { status: string; message: string }

export const useAppStore = defineStore('app', () => {
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
  const chatBodyEl = ref<HTMLElement | null>(null)
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
  const agentStatus = ref<AgentStatus | null>(null)

  // Artifact viewer state
  const artifactViewerOpen = ref(false)
  const artifactViewerArtifact = ref<Artifact | null>(null)
  const artifactViewerContent = ref('')

  // Plan viewer state
  const planViewerOpen = ref(false)
  const planViewerTaskID = ref('')

  const selectedWorkspace = computed(() => workspaces.value.find((w) => w.id === workspaceID.value) ?? null)
  const selectedDeployment = computed(() => deployments.value.find((d) => d.deployment_id === deploymentID.value) ?? null)
  const groupedConversations = computed(() =>
    workspaces.value.map((ws) => ({ workspace: ws, conversations: conversations.value.filter((c) => c.workspace_id === ws.id) })),
  )
  const isNewConversation = computed(() => !activeConversation.value)
  const canSend = computed(() => Boolean(prompt.value.trim() && workspaceID.value && !busy.value))
  const turnMap = computed(() =>
    new Map((activeConversation.value?.turns ?? []).filter((t) => t?.snapshot?.task?.id).map((t) => [t.snapshot.task.id, t])),
  )

  function fail(cause: unknown) { error.value = String(cause); notice.value = '' }
  function clearFeedback() { error.value = ''; notice.value = '' }
  function chooseDeployment(id: string) { deploymentID.value = id; capability.value = null; if (id) localStorage.setItem(STORAGE_KEY, id) }
  function selectWorkspace(id: string) { workspaceID.value = id; activePanel.value = 'chat'; clearFeedback() }
  function toggleWorkspace(id: string) { collapsed.value[id] = !collapsed.value[id] }
  function newConversation() { activeConversation.value = null; prompt.value = ''; clearFeedback(); activePanel.value = 'chat'; clientLog('info', '用户开始新会话', { workspace_id: workspaceID.value, permission_mode: permissionMode.value }) }
  function openSettings() { activePanel.value = 'settings'; refreshDiagnosticLogs() }

  function turnFor(message: { turn_task_id?: string }) {
    return message.turn_task_id ? turnMap.value.get(message.turn_task_id) : undefined
  }
  function artifactCount(turn?: { snapshot?: { steps?: { artifact_ids?: string[] }[] } }) {
    return turn?.snapshot?.steps?.reduce((t, s) => t + (s.artifact_ids?.length ?? 0), 0) ?? 0
  }
  function evidenceCount(turn?: { snapshot?: { steps?: { evidence_ids?: string[] }[] } }) {
    return turn?.snapshot?.steps?.reduce((t, s) => t + (s.evidence_ids?.length ?? 0), 0) ?? 0
  }
  function reportSummary(turn?: { report?: { summary: string } }) {
    return turn?.report?.summary || '本轮执行信息已保存，但没有可展示的摘要。'
  }
  function reportTool(turn?: { report?: { tool_name: string } }) {
    return turn?.report?.tool_name || '未调用工具'
  }
  function pendingWriteFor(turn?: { report?: { pending_write?: PendingWriteBatch } }) {
    return turn?.report?.pending_write
  }
  function pendingCommandFor(turn?: { report?: { pending_command?: PendingCommand } }) {
    return turn?.report?.pending_command
  }

  async function scrollChat() {
    await nextTick()
    chatBodyEl.value?.scrollTo({ top: chatBodyEl.value.scrollHeight, behavior: 'smooth' })
  }

  async function refresh() {
    try {
      const [ws, deps, convs] = await Promise.all([
        window.go.main.App.ListWorkspaces(),
        window.go.main.App.ListDeployments(),
        window.go.main.App.ListConversations(),
      ])
      workspaces.value = ws as Workspace[]
      deployments.value = deps as Deployment[]
      conversations.value = convs as Conversation[]
      if (!workspaceID.value && workspaces.value.length) workspaceID.value = workspaces.value[0].id
      if (!deployments.value.some((d) => d.deployment_id === deploymentID.value)) {
        const saved = localStorage.getItem(STORAGE_KEY) ?? ''
        chooseDeployment(deployments.value.some((d) => d.deployment_id === saved) ? saved : (deployments.value[0]?.deployment_id ?? ''))
      }
    } catch (cause) { clientLog('error', '刷新工作台数据失败', { error: String(cause) }); fail(cause) }
  }

  async function refreshDiagnosticLogs() {
    try { diagnosticLogs.value = await window.go.main.App.ListDiagnosticLogs(120) as DiagnosticEntry[] }
    catch (cause) { clientLog('error', '读取诊断日志失败', { error: String(cause) }); fail(cause) }
  }

  async function openConversation(conversation: Conversation) {
    try {
      busy.value = true; clearFeedback()
      activeConversation.value = await window.go.main.App.GetConversation(conversation.id) as ConversationView
      workspaceID.value = conversation.workspace_id
      const storedMode = knownMode(activeConversation.value.conversation.spec?.permission_profile_id)
      if (storedMode) permissionMode.value = storedMode
      activePanel.value = 'chat'; await scrollChat()
    } catch (cause) { clientLog('error', '打开会话失败', { conversation_id: conversation.id, error: String(cause) }); fail(cause) }
    finally { busy.value = false }
  }

  async function sendMessage() {
    const text = prompt.value.trim()
    if (!canSend.value) return
    prompt.value = ''; clearFeedback()
    // Optimistically show the user message immediately
    const now = new Date().toISOString()
    const userMsg = { message_id: 'temp_' + Date.now(), conversation_id: activeConversation.value?.conversation.id ?? '', role: 'user' as const, content: text, created_at: now }
    if (activeConversation.value) {
      activeConversation.value = { ...activeConversation.value, messages: [...activeConversation.value.messages, userMsg] }
    }
    await scrollChat()
    try {
      busy.value = true
      agentStatus.value = { status: 'thinking', message: 'Agent 正在理解需求…' }
      const result = activeConversation.value
        ? await window.go.main.App.SendConversationMessage(activeConversation.value.conversation.id, text, deploymentID.value, permissionMode.value)
        : await window.go.main.App.StartConversation(workspaceID.value, text, deploymentID.value, permissionMode.value)
      const next = result as ConversationView
      if (!next?.conversation?.id || !Array.isArray(next.messages)) throw new Error('桌面核心返回了不完整的会话数据；详情已写入诊断日志。')
      activeConversation.value = next; notice.value = '本轮已完成，执行过程和结果已保存到会话。'; await refresh()
    } catch (cause) { clientLog('error', '发送消息或渲染本轮结果失败', { conversation_id: activeConversation.value?.conversation?.id ?? '', permission_mode: permissionMode.value, error: String(cause) }); fail(cause) }
    finally { busy.value = false; agentStatus.value = null; await scrollChat() }
  }

  async function approveWrite(turn?: { report?: { pending_write?: PendingWriteBatch } }) {
    const pending = pendingWriteFor(turn); if (!pending || busy.value) return
    try { busy.value = true; clearFeedback(); activeConversation.value = await window.go.main.App.ApproveConversationWrite(pending.task_id, pending.step_id) as ConversationView; notice.value = '已批准工作区修改，系统已执行并完成验证。'; await refresh() }
    catch (cause) { clientLog('error', '批准工作区修改失败', { error: String(cause) }); fail(cause) }
    finally { busy.value = false; await scrollChat() }
  }

  async function approveCommand(turn?: { report?: { pending_command?: PendingCommand } }) {
    const pending = pendingCommandFor(turn); if (!pending || busy.value) return
    try { busy.value = true; clearFeedback(); activeConversation.value = await window.go.main.App.ApproveConversationCommand(pending.task_id, pending.step_id) as ConversationView; notice.value = '已批准项目命令，系统已执行并保存输出。'; await refresh() }
    catch (cause) { clientLog('error', '批准项目命令失败', { error: String(cause) }); fail(cause) }
    finally { busy.value = false; await scrollChat() }
  }

  function applyProviderTemplate() {
    if (providerTemplate.value === 'deepseek') { deploymentName.value = 'DeepSeek API'; deploymentEndpoint.value = 'https://api.deepseek.com'; deploymentModel.value = 'deepseek-chat' }
    else if (providerTemplate.value === 'local') { deploymentName.value = '本地模型'; deploymentEndpoint.value = 'http://127.0.0.1:8080/v1'; deploymentModel.value = '' }
    else { deploymentName.value = '自定义模型'; deploymentEndpoint.value = ''; deploymentModel.value = '' }
  }

  async function configureModel() {
    try { busy.value = true; const item = await window.go.main.App.ConfigureOpenAICompatibleDeployment(deploymentName.value, deploymentEndpoint.value, deploymentModel.value, apiKey.value) as Deployment; chooseDeployment(item.deployment_id); apiKey.value = ''; notice.value = '模型配置已保存，密钥仅保存在 Windows 凭据管理器。'; await refresh() }
    catch (cause) { clientLog('error', '保存模型失败', { error: String(cause) }); fail(cause) }
    finally { busy.value = false }
  }

  async function probeModel() {
    try { busy.value = true; capability.value = await window.go.main.App.ProbeDeployment(deploymentID.value) as Capability; notice.value = '能力检查完成。' }
    catch (cause) { clientLog('error', '能力检查失败', { error: String(cause) }); fail(cause) }
    finally { busy.value = false }
  }

  async function createWorkspace() {
    try { busy.value = true; const item = await window.go.main.App.CreateWorkspace(workspaceName.value, workspacePath.value) as Workspace; workspaceID.value = item.id; workspaceName.value = ''; workspacePath.value = ''; showWorkspaceForm.value = false; notice.value = '工作目录已添加。'; await refresh() }
    catch (cause) { clientLog('error', '添加工作目录失败', { error: String(cause) }); fail(cause) }
    finally { busy.value = false }
  }

  async function viewArtifact(taskID: string, artifactID: string) {
    try {
      busy.value = true
      const artifacts = await window.go.main.App.ListTaskArtifacts(taskID) as Artifact[]
      const art = artifacts.find((a) => a.artifact_id === artifactID)
      if (!art) return
      artifactViewerArtifact.value = art
      const content = await window.go.main.App.ReadTaskArtifact(taskID, art.kind) as string
      artifactViewerContent.value = typeof content === 'string' ? content : String(content)
      artifactViewerOpen.value = true
    } catch (cause) { clientLog('error', '读取 Artifact 失败', { artifact_id: artifactID, error: String(cause) }) }
    finally { busy.value = false }
  }

  function closeArtifactViewer() { artifactViewerOpen.value = false; artifactViewerArtifact.value = null; artifactViewerContent.value = '' }

  async function viewPlan(taskID: string) {
    planViewerTaskID.value = taskID
    planViewerOpen.value = true
  }

  function closePlanViewer() { planViewerOpen.value = false; planViewerTaskID.value = '' }

  function setupAgentStatusListener() {
    if (typeof window !== 'undefined' && (window as any).runtime) {
      (window as any).runtime.EventsOn('agent:status', (data: AgentStatus) => {
        agentStatus.value = data
        if (data.status === 'completed' || data.status === 'error') {
          setTimeout(() => { agentStatus.value = null }, 3000)
        }
      })
    }
  }

  return {
    workspaces, deployments, conversations, activeConversation, activePanel,
    workspaceID, deploymentID, permissionMode, prompt, busy, error, notice,
    collapsed, chatBodyEl, showWorkspaceForm, workspaceName, workspacePath,
    deploymentName, deploymentEndpoint, deploymentModel, apiKey, providerTemplate,
    capability, diagnosticLogs, agentStatus,
    artifactViewerOpen, artifactViewerArtifact, artifactViewerContent,
    planViewerOpen, planViewerTaskID,
    selectedWorkspace, selectedDeployment, groupedConversations,
    isNewConversation, canSend, turnMap,
    fail, clearFeedback, chooseDeployment, selectWorkspace, toggleWorkspace,
    newConversation, openSettings, turnFor, artifactCount, evidenceCount,
    reportSummary, reportTool, pendingWriteFor, pendingCommandFor,
    scrollChat, refresh, refreshDiagnosticLogs, openConversation, sendMessage,
    approveWrite, approveCommand, applyProviderTemplate, configureModel,
    probeModel, createWorkspace, viewArtifact, closeArtifactViewer,
    viewPlan, closePlanViewer, setupAgentStatusListener,
  }
})
