<script lang="ts" setup>
import { useAppStore } from '../stores/app'
import { renderMarkdown, statusText, eventText, timeText, modeLabel, modeHint, commandText } from '../utils'
import { ref } from 'vue'
import type { Turn } from '../types'

const store = useAppStore()

function openArtifacts(turn: Turn) {
  const taskID = turn.snapshot.task.id
  if (taskID) store.viewPlan(taskID)
}
</script>

<template>
  <section class="conversation-shell">
    <div ref="store.chatBodyEl" class="chat-body">
      <div v-if="store.isNewConversation" class="welcome">
        <div class="welcome-icon">S</div>
        <h1>从一个问题开始</h1>
        <p>{{ store.selectedWorkspace ? `当前工作目录：${store.selectedWorkspace.name}` : '请先从左侧选择一个工作目录。' }}</p>
        <div class="suggestions" v-if="store.selectedWorkspace">
          <button @click="store.prompt = '分析当前工作目录的项目结构，并列出关键文件'">分析项目结构</button>
          <button @click="store.prompt = '阅读 README 并说明如何运行项目'">阅读 README</button>
          <button @click="store.prompt = '检查当前工作目录的待办事项和下一步建议'">给出开发建议</button>
        </div>
      </div>

      <article v-for="message in store.activeConversation?.messages ?? []" :key="message.message_id" class="message" :class="message.role">
        <div class="avatar">{{ message.role === 'user' ? '你' : 'S' }}</div>
        <div class="message-content">
          <div class="message-meta">
            <b>{{ message.role === 'user' ? '你' : 'Simpleness' }}</b>
            <time>{{ timeText(message.created_at) }}</time>
          </div>
          <div class="markdown-body" v-html="renderMarkdown(message.content)"></div>

          <section v-if="message.role === 'assistant' && store.turnFor(message)" class="turn-result">
            <div class="turn-head">
              <b>本轮执行结果</b>
              <span class="status-pill" :class="store.turnFor(message)?.snapshot?.task?.status?.toLowerCase()">{{ statusText(store.turnFor(message)?.snapshot?.task?.status ?? '') }}</span>
            </div>

            <div class="result-stats">
              <span>已调用：{{ store.reportTool(store.turnFor(message)) }}</span>
              <span>{{ store.artifactCount(store.turnFor(message)) }} 个产物</span>
              <span>{{ store.evidenceCount(store.turnFor(message)) }} 条证据</span>
              <span v-if="store.turnFor(message)?.report?.input_tokens">Token：{{ store.turnFor(message)?.report?.input_tokens }} 入 / {{ store.turnFor(message)?.report?.output_tokens }} 出</span>
              <span v-if="store.turnFor(message)?.report?.iterations">迭代：{{ store.turnFor(message)?.report?.iterations }} 轮</span>
              <span v-if="store.turnFor(message)?.report?.duration_seconds">耗时：{{ store.turnFor(message)?.report?.duration_seconds?.toFixed(1) }}s</span>
              <button v-if="store.artifactCount(store.turnFor(message)) > 0" class="text-button" @click="store.viewPlan(store.turnFor(message)?.snapshot.task.id ?? '')">查看计划</button>
            </div>

            <section v-if="store.turnFor(message)?.snapshot?.horizon" class="horizon-card">
              <div>
                <b>{{ store.turnFor(message)?.snapshot?.horizon?.plan.stages[store.turnFor(message)?.snapshot?.horizon?.current_stage_index ?? 0]?.stage_id ?? 'FINALIZE' }}</b>
                <span>{{ store.turnFor(message)?.snapshot?.horizon?.steps_completed }}/{{ store.turnFor(message)?.snapshot?.horizon?.steps_planned }} 步 · 剩余 {{ 20 - (store.turnFor(message)?.snapshot?.horizon?.steps_planned ?? 0) }}</span>
              </div>
              <p>重规划 {{ store.turnFor(message)?.snapshot?.horizon?.replans_used }}/4 · Token {{ store.turnFor(message)?.snapshot?.horizon?.usage.input_tokens }} 入 / {{ store.turnFor(message)?.snapshot?.horizon?.usage.output_tokens }} 出</p>
              <p v-if="store.turnFor(message)?.snapshot?.steps?.find(step => ['PENDING', 'READY', 'RUNNING'].includes(step.status))">当前步骤：{{ store.turnFor(message)?.snapshot?.steps?.find(step => ['PENDING', 'READY', 'RUNNING'].includes(step.status))?.title }}</p>
              <p v-if="store.turnFor(message)?.snapshot?.horizon?.checkpoint_reason">{{ store.turnFor(message)?.snapshot?.horizon?.checkpoint_reason }}</p>
              <div class="horizon-actions">
                <button v-if="store.turnFor(message)?.snapshot?.horizon?.awaiting_checkpoint || store.turnFor(message)?.snapshot?.task?.status === 'PAUSED'" class="primary-button" :disabled="store.busy" @click="store.resumeLongHorizon(store.turnFor(message)?.snapshot.task.id ?? '')">{{ store.turnFor(message)?.snapshot?.horizon?.awaiting_checkpoint ? '确认并继续下一阶段' : '恢复任务并局部重规划' }}</button>
                <button v-if="!['COMPLETED', 'CANCELLED', 'BLOCKED'].includes(store.turnFor(message)?.snapshot?.horizon?.status ?? '')" class="secondary-button" :disabled="store.busy" @click="store.cancelLongHorizon(store.turnFor(message)?.snapshot.task.id ?? '')">取消任务</button>
              </div>
            </section>

            <details>
              <summary>查看 Agent 操作记录</summary>
              <ol class="operation-log">
                <li v-for="event in (store.turnFor(message)?.snapshot?.events ?? [])" :key="event.sequence">
                  <b>{{ eventText(event.event_type) }}</b>
                  <time>{{ timeText(event.timestamp) }}</time>
                </li>
              </ol>
            </details>

            <section v-if="store.pendingWriteFor(store.turnFor(message))" class="approval-card">
              <div><b>等待确认工作区修改</b><span>本批 {{ store.pendingWriteFor(store.turnFor(message))?.writes.length }} 个文件</span></div>
              <p>确认前不会修改文件；若任一文件已变化，系统会拒绝覆盖整个批次。</p>
              <details><summary>查看拟写入内容</summary>
                <div class="proposal-files">
                  <section v-for="write in store.pendingWriteFor(store.turnFor(message))?.writes" :key="write.path">
                    <code>{{ write.path }}</code>
                    <pre>{{ write.content }}</pre>
                  </section>
                </div>
              </details>
              <button class="primary-button" :disabled="store.busy" @click="store.approveWrite(store.turnFor(message))">确认并写入全部文件</button>
            </section>

            <section v-else-if="store.pendingCommandFor(store.turnFor(message))" class="approval-card command">
              <div><b>等待确认项目命令</b><span>{{ store.pendingCommandFor(store.turnFor(message))?.timeout_ms }} ms 上限</span></div>
              <code>{{ commandText(store.pendingCommandFor(store.turnFor(message))) }}</code>
              <p>该命令只会在当前工作目录中执行一次，输出会被限额保存到执行记录。</p>
              <button class="primary-button" :disabled="store.busy" @click="store.approveCommand(store.turnFor(message))">确认并执行命令</button>
            </section>

            <section v-if="store.turnFor(message)?.report?.pending_question" class="approval-card question">
              <div><b>Agent 提问</b></div>
              <p class="question-text">{{ store.turnFor(message)?.report?.pending_question?.question }}</p>
              <p v-if="store.turnFor(message)?.report?.pending_question?.context" class="question-context">{{ store.turnFor(message)?.report?.pending_question?.context }}</p>
              <div v-if="store.turnFor(message)?.report?.pending_question?.options?.length" class="question-options">
                <button v-for="opt in store.turnFor(message)?.report?.pending_question?.options" :key="opt" class="secondary-button" @click="store.prompt = opt; store.sendMessage()">{{ opt }}</button>
              </div>
              <p class="question-hint">选择上方选项，或在下方输入框回复后发送</p>
            </section>
          </section>
        </div>
      </article>

      <div v-if="store.busy" class="thinking">
        <i></i><i></i><i></i>
        {{ store.agentStatus?.message || 'Agent 正在处理本轮请求…' }}
      </div>
    </div>

    <div class="composer">
      <textarea v-model="store.prompt" :disabled="store.busy" @keydown.ctrl.enter.prevent="store.sendMessage" placeholder="给 Agent 发送任务或问题；Ctrl + Enter 发送"></textarea>
      <div class="composer-footer">
        <div class="composer-tools">
          <button class="round-button" title="新对话" @click="store.newConversation">＋</button>
          <span class="workspace-chip">{{ store.selectedWorkspace?.name || '未选择目录' }}</span>
          <select :value="store.deploymentID" :disabled="store.busy" @change="store.chooseDeployment(($event.target as HTMLSelectElement).value)">
            <option value="">不使用模型（确定性侦察）</option>
            <option v-for="item in store.deployments" :key="item.deployment_id" :value="item.deployment_id">{{ item.name }} · {{ item.model }}</option>
          </select>
          <select v-model="store.permissionMode" :disabled="store.busy" :title="modeHint(store.permissionMode)" @change="store.enforceLongHorizonPermission">
            <option value="PLAN">计划模式（只读）</option>
            <option value="EDIT">编辑模式（需确认）</option>
            <option value="DEVELOPMENT">开发模式（直接执行）</option>
          </select>
          <select :value="store.executionStrategy" :disabled="store.busy || !store.deploymentID" @change="store.chooseExecutionStrategy(($event.target as HTMLSelectElement).value)">
            <option value="SINGLE_PLAN">单次计划</option>
            <option value="INCREMENTAL_HORIZON">长程 Agent</option>
          </select>
        </div>
        <button class="send-button" :disabled="!store.canSend" @click="store.sendMessage">发送 <span>↑</span></button>
      </div>
      <small class="mode-note">{{ store.executionStrategy === 'INCREMENTAL_HORIZON' ? '长程 Agent' : '单次计划' }} · {{ modeLabel(store.permissionMode) }}：{{ modeHint(store.permissionMode) }}</small>
    </div>
  </section>
</template>
