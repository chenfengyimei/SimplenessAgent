# ADR-0005：本地与 API 共享 Provider/Deployment 模型

- 状态：已采纳；Mock Provider 已实现
- 日期：2026-08-11

Agent Core 仅依赖 `ChatProvider` 标准对象。具体 OpenAI-compatible、本地 Runtime 和远程 API 的协议差异必须停留在 Provider Adapter 中，不能泄漏至工具和调度器。
