<script lang="ts" setup>
import { ref, watch } from 'vue'
import { useAppStore } from '../stores/app'
import { statusText, timeText } from '../utils'
import type { PlanView as PlanViewType } from '../types'

const store = useAppStore()
const plan = ref<PlanViewType | null>(null)
const loading = ref(false)

watch(() => store.planViewerOpen, async (open) => {
  if (!open || !store.planViewerTaskID) { plan.value = null; return }
  loading.value = true
  try {
    plan.value = await window.go.main.App.GetTaskPlan(store.planViewerTaskID) as PlanViewType
  } catch { plan.value = null }
  finally { loading.value = false }
})

const riskColors: Record<string, string> = { READ: '#5b8c5a', WRITE: '#c4923a', DANGEROUS: '#c44d4d' }
</script>

<template>
  <Teleport to="body">
    <div v-if="store.planViewerOpen" class="modal-overlay" @click.self="store.closePlanViewer">
      <div class="modal-panel plan-modal">
        <div class="modal-header">
          <div>
            <b>执行计划</b>
            <small v-if="plan">Revision {{ plan.revision }}</small>
          </div>
          <button class="modal-close" @click="store.closePlanViewer">✕</button>
        </div>
        <div class="modal-body">
          <div v-if="loading" class="plan-loading">加载中…</div>
          <template v-else-if="plan">
            <p class="plan-summary" v-if="plan.summary">{{ plan.summary }}</p>
            <div class="plan-reason" v-if="plan.reason"><small>原因：{{ plan.reason }}</small></div>
            <div class="plan-steps">
              <div v-for="step in plan.steps" :key="step.step_id" class="plan-step">
                <div class="step-header">
                  <span class="step-status" :class="step.status?.toLowerCase()">{{ statusText(step.status) }}</span>
                  <b>{{ step.title }}</b>
                </div>
                <p class="step-goal">{{ step.goal }}</p>
                <div class="step-meta">
                  <span class="risk-tag" :style="{ color: riskColors[step.risk] ?? '#888' }">{{ step.risk }}</span>
                  <span v-if="step.dependencies?.length">依赖：{{ step.dependencies.join(', ') }}</span>
                  <span v-if="step.allowed_tools?.length">工具：{{ step.allowed_tools.join(', ') }}</span>
                </div>
              </div>
            </div>
          </template>
          <p v-else class="muted">暂无计划数据。</p>
        </div>
      </div>
    </div>
  </Teleport>
</template>
