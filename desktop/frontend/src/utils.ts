import { marked } from 'marked'
import DOMPurify from 'dompurify'

marked.setOptions({ breaks: true, gfm: true })

export function renderMarkdown(text: string): string {
  if (!text) return ''
  return DOMPurify.sanitize(marked.parse(text, { async: false }) as string)
}

export function statusText(status: string): string {
  return ({ READY: '就绪', RUNNING: '执行中', WAITING_APPROVAL: '等待确认', COMPLETED: '已完成', FAILED: '失败', PAUSED: '已暂停', PENDING: '待执行' } as Record<string, string>)[status] ?? status
}

export function eventText(event: string): string {
  return ({ TASK_CREATED: '创建执行回合', TASK_STATUS_CHANGED: '更新任务状态', PLAN_CREATED: '生成执行计划', STEP_STATUS_CHANGED: '更新步骤状态', TOOL_APPROVED: '确认操作', TOOL_INTENT_RECORDED: '记录执行意图', ARTIFACT_SAVED: '保存执行产物', EVIDENCE_SAVED: '保存验证证据', FINAL_REPORT_CREATED: '生成最终报告' } as Record<string, string>)[event] ?? event.replaceAll('_', ' ')
}

export function timeText(value: string): string {
  return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : ''
}

export function modeLabel(mode: string): string {
  return ({ PLAN: '计划模式', EDIT: '编辑模式', DEVELOPMENT: '开发模式' } as Record<string, string>)[mode] ?? mode
}

export function modeHint(mode: string): string {
  return ({ PLAN: '仅读取文件与结构，不执行命令、不修改文件', EDIT: '修改与项目命令均须先确认', DEVELOPMENT: '允许受限命令和工作区直接修改' } as Record<string, string>)[mode] ?? ''
}

export function knownMode(value?: string): PermissionMode | null {
  return value === 'PLAN' || value === 'EDIT' || value === 'DEVELOPMENT' ? value : null
}

export function commandText(command?: PendingCommand): string {
  if (!command) return ''
  const labels: Record<string, string> = { go_test: 'go test', go_vet: 'go vet', npm_test: 'npm test', npm_build: 'npm run build', npm_init: 'npm init', npm_install: 'npm install', npm_run: 'npm run', npx: 'npx', python: 'python', pip_install: 'pip install' }
  return `${labels[command.command] ?? command.command}${command.arguments?.length ? ` ${command.arguments.join(' ')}` : ''}`
}

export function clientLog(level: 'info' | 'error', message: string, fields: Record<string, string> = {}): void {
  try { window.go?.main?.App?.RecordClientLog(level, message, fields) } catch (_) { /* diagnostics must never break the UI */ }
}
