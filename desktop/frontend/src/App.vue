<script lang="ts" setup>
import { onMounted, ref } from 'vue'

type TaskSnapshot = { task: { id: string; title: string; goal: string; status: string }, steps: unknown[], events: unknown[] }
const tasks = ref<TaskSnapshot[]>([])
const error = ref('')
const loading = ref(true)

async function refresh() {
  loading.value = true
  error.value = ''
  try {
    tasks.value = await window.go.main.App.ListTasks() as TaskSnapshot[]
  } catch (cause) {
    error.value = String(cause)
  } finally {
    loading.value = false
  }
}
onMounted(refresh)
</script>

<template>
  <main>
    <header><div><p class="eyebrow">SIMPLENESSAGENT</p><h1>任务工作台</h1></div><button @click="refresh">刷新</button></header>
    <p v-if="loading">正在读取本地任务事实…</p>
    <p v-else-if="error" class="error">{{ error }}</p>
    <section v-else-if="tasks.length" class="tasks">
      <article v-for="item in tasks" :key="item.task.id">
        <div><p class="status">{{ item.task.status }}</p><h2>{{ item.task.title }}</h2><p>{{ item.task.goal }}</p></div>
        <footer>{{ item.steps.length }} Steps · {{ item.events.length }} Events</footer>
      </article>
    </section>
    <section v-else class="empty"><h2>暂无任务</h2><p>请使用 Core CLI 创建工作区与任务；桌面端只读取可恢复的 Core 状态。</p></section>
  </main>
</template>
