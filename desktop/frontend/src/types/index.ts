export type PermissionMode = 'PLAN' | 'EDIT' | 'DEVELOPMENT'

export type Workspace = { id: string; name: string; root_path: string }

export type Deployment = { deployment_id: string; name: string; endpoint: string; model: string }

export type Capability = { supports_tools: boolean; supports_streaming: boolean; reliable_context_tokens: number }

export type Conversation = { id: string; workspace_id: string; title: string; goal: string; status: string; updated_at: string; spec?: { permission_profile_id?: string } }

export type Message = { message_id: string; conversation_id: string; turn_task_id?: string; role: 'user' | 'assistant'; content: string; created_at: string }

export type EventEntry = { event_type: string; timestamp: string; sequence: number }

export type Step = { step_id: string; title?: string; status: string; artifact_ids: string[]; evidence_ids: string[] }

export type Snapshot = { task: { id: string; title: string; goal: string; status: string }; steps: Step[]; events: EventEntry[] }

export type PendingWrite = { task_id: string; step_id: string; path: string; content: string }

export type PendingWriteBatch = { task_id: string; step_id: string; writes: PendingWrite[] }

export type PendingCommand = { task_id: string; step_id: string; command: string; arguments: string[]; timeout_ms: number }

export type Turn = {
  snapshot: Snapshot
  report: {
    summary: string
    tool_name: string
    files: string[]
    truncated: boolean
    pending_write?: PendingWriteBatch
    pending_command?: PendingCommand
  }
}

export type ConversationView = { conversation: Conversation; messages: Message[]; turns: Turn[] }

export type DiagnosticEntry = { timestamp: string; level: string; component: string; message: string; fields?: Record<string, string> }

export type Artifact = {
  artifact_id: string
  kind: string
  media_type: string
  summary: string
  size_bytes: number
  content_hash: string
  created_at: string
  source?: { run_id?: string; step_id?: string; tool_call_id?: string }
}

export type PlanStep = {
  step_id: string
  title: string
  goal: string
  status: string
  dependencies: string[]
  allowed_tools: string[]
  risk: string
}

export type PlanView = {
  plan_id: string
  task_id: string
  revision: number
  summary: string
  reason: string
  steps: PlanStep[]
  created_at: string
}
