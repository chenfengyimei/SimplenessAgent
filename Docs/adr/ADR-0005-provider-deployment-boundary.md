# ADR-0005：本地与 API 共享 Provider/Deployment 模型

- 状态：已采纳；Mock Provider 已实现
- 日期：2026-08-11

Agent Core 仅依赖 `ChatProvider` 标准对象。具体 OpenAI-compatible、本地 Runtime 和远程 API 的协议差异必须停留在 Provider Adapter 中，不能泄漏至工具和调度器。

## 2026-08-11：Provider PoC

首个真实 Adapter 已落地于 `internal/provider/openai`。它负责 HTTP/SSE、认证 Header、错误分类和能力探测，并将文本、用量及 Tool Call 标准化；它不执行工具、不写入 Artifact，也不提供静默的模型回退。原始 Provider 响应在接入持久化链路前必须脱敏。
