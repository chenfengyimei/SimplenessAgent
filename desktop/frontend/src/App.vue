<script lang="ts" setup>
import { onMounted, ref } from 'vue'

type Step = { step_id: string; status: string; artifact_ids: string[]; evidence_ids: string[] }
type Event = { event_type: string; timestamp: string }
type TaskSnapshot = { task: { id: string; title: string; goal: string; status: string }, steps: Step[], events: Event[] }
type Workspace = { id: string; name: string; root_path: string }
type Deployment = { id: string; name: string; endpoint: string; model: string; enabled: boolean }
type Capability = { supports_tools: boolean; supports_streaming: boolean; reliable_context_tokens: number }

const tasks = ref<TaskSnapshot[]>([]); const workspaces = ref<Workspace[]>([]); const deployments = ref<Deployment[]>([])
const selected = ref<TaskSnapshot | null>(null); const error = ref(''); const notice = ref(''); const loading = ref(true); const busy = ref(false)
const workspaceName = ref(''); const workspacePath = ref(''); const taskWorkspaceID = ref(''); const taskTitle = ref(''); const taskGoal = ref('')
const deploymentName = ref('Local model'); const deploymentEndpoint = ref('http://127.0.0.1:8080/v1'); const deploymentModel = ref(''); const apiKey = ref(''); const selectedDeploymentID = ref(''); const capability = ref<Capability | null>(null)

function fail(cause: unknown) { error.value = String(cause); notice.value = '' }
async function refresh() {
  loading.value = true; error.value = ''
  try {
    const [taskItems, workspaceItems, deploymentItems] = await Promise.all([window.go.main.App.ListTasks(), window.go.main.App.ListWorkspaces(), window.go.main.App.ListDeployments()])
    tasks.value = taskItems as TaskSnapshot[]; workspaces.value = workspaceItems as Workspace[]; deployments.value = deploymentItems as Deployment[]
    if (!taskWorkspaceID.value && workspaces.value.length) taskWorkspaceID.value = workspaces.value[0].id
    if (!selectedDeploymentID.value && deployments.value.length) selectedDeploymentID.value = deployments.value[0].id
  } catch (cause) { fail(cause) } finally { loading.value = false }
}
async function createWorkspace() { try { busy.value = true; await window.go.main.App.CreateWorkspace(workspaceName.value, workspacePath.value); workspaceName.value = ''; workspacePath.value = ''; notice.value = '工作区已创建'; await refresh() } catch (cause) { fail(cause) } finally { busy.value = false } }
async function createTask() { try { busy.value = true; const item = await window.go.main.App.CreateTask(taskWorkspaceID.value, taskTitle.value, taskGoal.value) as TaskSnapshot; taskTitle.value = ''; taskGoal.value = ''; selected.value = item; notice.value = '任务已创建，可选择模型后运行 Agent'; await refresh() } catch (cause) { fail(cause) } finally { busy.value = false } }
async function configureModel() { try { busy.value = true; const item = await window.go.main.App.ConfigureOpenAICompatibleDeployment(deploymentName.value, deploymentEndpoint.value, deploymentModel.value, apiKey.value) as Deployment; selectedDeploymentID.value = item.id; apiKey.value = ''; notice.value = '模型配置已保存；API Key 仅保留在当前应用会话'; await refresh() } catch (cause) { fail(cause) } finally { busy.value = false } }
async function probeModel() { try { busy.value = true; capability.value = await window.go.main.App.ProbeDeployment(selectedDeploymentID.value) as Capability; notice.value = '连通性与能力探测完成' } catch (cause) { fail(cause) } finally { busy.value = false } }
async function selectTask(id: string) { try { selected.value = await window.go.main.App.GetTaskSnapshot(id) as TaskSnapshot } catch (cause) { fail(cause) } }
async function runAgent() { if (!selected.value || !selectedDeploymentID.value) return; try { busy.value = true; await window.go.main.App.GeneratePlan(selected.value.task.id, selectedDeploymentID.value); selected.value = await window.go.main.App.RunAgent(selected.value.task.id, selectedDeploymentID.value) as TaskSnapshot; notice.value = 'Agent 已运行；请查看步骤、证据与事件'; await refresh() } catch (cause) { fail(cause) } finally { busy.value = false } }
onMounted(refresh)
</script>

<template>
  <main>
    <header><div><p class="eyebrow">SIMPLENESSAGENT · LOCAL-FIRST</p><h1>任务工作台</h1><p class="subtitle">配置模型 → 创建任务 → 运行受控 Agent → 查看证据</p></div><button :disabled="busy" @click="refresh">刷新</button></header>
    <p v-if="notice" class="notice">{{ notice }}</p><p v-if="error" class="error">{{ error }}</p>
    <section class="commands model"><form @submit.prevent="configureModel"><h2>1. 模型设置</h2><input v-model="deploymentName" required placeholder="配置名称"><input v-model="deploymentEndpoint" required placeholder="OpenAI-compatible Base URL"><input v-model="deploymentModel" required placeholder="模型 ID，例如 qwen3"><input v-model="apiKey" type="password" placeholder="API Key（仅本次会话）"><button :disabled="busy">保存模型配置</button></form><div class="model-use"><h2>连通性</h2><select v-model="selectedDeploymentID"><option disabled value="">选择模型配置</option><option v-for="deployment in deployments" :key="deployment.id" :value="deployment.id">{{ deployment.name }} · {{ deployment.model }}</option></select><button :disabled="busy || !selectedDeploymentID" @click="probeModel">探测能力</button><p v-if="capability">Tools: {{ capability.supports_tools ? '支持' : '不支持' }} · Streaming: {{ capability.supports_streaming ? '支持' : '不支持' }} · Context: {{ capability.reliable_context_tokens || '未知' }}</p><p v-else>API Key 不写入数据库；本地无认证服务可留空。</p></div></section>
    <section class="commands"><form @submit.prevent="createWorkspace"><h2>2. 添加工作区</h2><input v-model="workspaceName" placeholder="名称（可选）"><input v-model="workspacePath" required placeholder="本地目录绝对路径"><button :disabled="busy">添加工作区</button></form><form @submit.prevent="createTask"><h2>3. 创建任务</h2><select v-model="taskWorkspaceID" required><option disabled value="">选择工作区</option><option v-for="workspace in workspaces" :key="workspace.id" :value="workspace.id">{{ workspace.name }}</option></select><input v-model="taskTitle" required placeholder="任务标题"><textarea v-model="taskGoal" required placeholder="例如：分析当前项目结构并给出证据报告"></textarea><button :disabled="busy">创建任务</button></form></section>
    <p v-if="loading">正在读取本地任务事实…</p>
    <section v-else-if="tasks.length" class="tasks"><article v-for="item in tasks" :key="item.task.id" :class="{chosen:selected?.task.id === item.task.id}" @click="selectTask(item.task.id)"><div><p class="status">{{ item.task.status }}</p><h2>{{ item.task.title }}</h2><p>{{ item.task.goal }}</p></div><footer>{{ item.steps.length }} Steps · {{ item.events.length }} Events</footer></article></section>
    <section v-else class="empty"><h2>尚无任务</h2><p>从上方依次添加工作区、配置模型、创建任务，然后运行 Agent。</p></section>
    <section v-if="selected" class="detail"><div class="detail-head"><div><p class="eyebrow">SELECTED TASK</p><h2>{{ selected.task.title }}</h2></div><button class="run" :disabled="busy || !selectedDeploymentID || selected.task.status !== 'READY'" @click="runAgent">运行 Agent</button></div><p>{{ selected.task.goal }}</p><div v-for="step in selected.steps" :key="step.step_id" class="step"><b>{{ step.step_id }}</b><span>{{ step.status }}</span><small>{{ step.artifact_ids.length }} Artifacts · {{ step.evidence_ids.length }} Evidence</small></div><h3>事件时间线</h3><ol><li v-for="event in selected.events.slice().reverse()" :key="event.timestamp + event.event_type"><b>{{ event.event_type }}</b> <small>{{ event.timestamp }}</small></li></ol></section>
  </main>
</template>
