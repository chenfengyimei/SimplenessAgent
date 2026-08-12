/// <reference types="vite/client" />

interface Window { go: { main: { App: {
  ListTasks(): Promise<unknown[]>
  ListWorkspaces(): Promise<unknown[]>
  CreateWorkspace(name: string, path: string): Promise<unknown>
  CreateTask(workspaceID: string, title: string, goal: string): Promise<unknown>
  GetTaskSnapshot(taskID: string): Promise<unknown>
  ListDeployments(): Promise<unknown[]>
  ConfigureOpenAICompatibleDeployment(name: string, endpoint: string, model: string, apiKey: string): Promise<unknown>
  ProbeDeployment(deploymentID: string): Promise<unknown>
  GeneratePlan(taskID: string, deploymentID: string): Promise<unknown>
  RunAgent(taskID: string, deploymentID: string): Promise<unknown>
  ListConversations(): Promise<unknown[]>
  GetConversation(conversationID: string): Promise<unknown>
  StartConversation(workspaceID: string, message: string, deploymentID: string): Promise<unknown>
  SendConversationMessage(conversationID: string, message: string, deploymentID: string): Promise<unknown>
  ListDiagnosticLogs(limit: number): Promise<unknown[]>
  RecordClientLog(level: string, message: string, fields: Record<string, string>): Promise<void>
} } } }

declare module '*.vue' {
    import type {DefineComponent} from 'vue'
    const component: DefineComponent<{}, {}, any>
    export default component
}
