<script lang="ts" setup>
import { onMounted, ref } from 'vue'

type Step = { step_id: string; status: string; artifact_ids: string[]; evidence_ids: string[] }
type Event = { event_type: string; timestamp: string }
type TaskSnapshot = { task: { id: string; title: string; goal: string; status: string }, steps: Step[], events: Event[] }
type Workspace = { id: string; name: string; root_path: string }
const tasks = ref<TaskSnapshot[]>([])
const workspaces = ref<Workspace[]>([])
const error = ref('')
const loading = ref(true)
const workspaceName = ref('')
const workspacePath = ref('')
const taskWorkspaceID = ref('')
const taskTitle = ref('')
const taskGoal = ref('')
const selected = ref<TaskSnapshot | null>(null)

async function refresh() {
  loading.value = true
  error.value = ''
  try {
    const [taskItems, workspaceItems] = await Promise.all([window.go.main.App.ListTasks(), window.go.main.App.ListWorkspaces()])
    tasks.value = taskItems as TaskSnapshot[]
    workspaces.value = workspaceItems as Workspace[]
    if (!taskWorkspaceID.value && workspaces.value.length) taskWorkspaceID.value = workspaces.value[0].id
  } catch (cause) {
    error.value = String(cause)
  } finally {
    loading.value = false
  }
}

async function createWorkspace() {
  try { await window.go.main.App.CreateWorkspace(workspaceName.value, workspacePath.value); workspaceName.value = ''; workspacePath.value = ''; await refresh() }
  catch (cause) { error.value = String(cause) }
}
async function createTask() {
  try { await window.go.main.App.CreateTask(taskWorkspaceID.value, taskTitle.value, taskGoal.value); taskTitle.value = ''; taskGoal.value = ''; await refresh() }
  catch (cause) { error.value = String(cause) }
}
async function selectTask(id: string) {
  try { selected.value = await window.go.main.App.GetTaskSnapshot(id) as TaskSnapshot }
  catch (cause) { error.value = String(cause) }
}
onMounted(refresh)
</script>

<template>
  <main>
    <header><div><p class="eyebrow">SIMPLENESSAGENT</p><h1>任务工作台</h1></div><button @click="refresh">刷新</button></header>
    <section class="commands">
      <form @submit.prevent="createWorkspace"><h2>添加工作区</h2><input v-model="workspaceName" placeholder="名称（可选）"><input v-model="workspacePath" required placeholder="本地目录绝对路径"><button>添加</button></form>
      <form @submit.prevent="createTask"><h2>创建任务</h2><select v-model="taskWorkspaceID" required><option disabled value="">选择工作区</option><option v-for="workspace in workspaces" :key="workspace.id" :value="workspace.id">{{ workspace.name }}</option></select><input v-model="taskTitle" required placeholder="任务标题"><textarea v-model="taskGoal" required placeholder="可验证目标"></textarea><button>创建</button></form>
    </section>
    <p v-if="loading">正在读取本地任务事实…</p>
    <p v-else-if="error" class="error">{{ error }}</p>
    <section v-else-if="tasks.length" class="tasks">
      <article v-for="item in tasks" :key="item.task.id" @click="selectTask(item.task.id)">
        <div><p class="status">{{ item.task.status }}</p><h2>{{ item.task.title }}</h2><p>{{ item.task.goal }}</p></div>
        <footer>{{ item.steps.length }} Steps · {{ item.events.length }} Events</footer>
      </article>
    </section>
    <section v-if="selected" class="detail"><h2>{{ selected.task.title }} · 执行详情</h2><div v-for="step in selected.steps" :key="step.step_id" class="step"><b>{{ step.step_id }}</b><span>{{ step.status }}</span><small>{{ step.artifact_ids.length }} Artifacts · {{ step.evidence_ids.length }} Evidence</small></div><h3>事件</h3><ol><li v-for="event in selected.events.slice().reverse()" :key="event.timestamp + event.event_type"><b>{{ event.event_type }}</b> <small>{{ event.timestamp }}</small></li></ol></section>
    <section v-else class="empty"><h2>暂无任务</h2><p>请使用 Core CLI 创建工作区与任务；桌面端只读取可恢复的 Core 状态。</p></section>
  </main>
</template>
