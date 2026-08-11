# SimplenessAgent

> 面向本地与 API 模型的可靠任务执行型 Agent 框架。

[English](#english) · [项目介绍](Docs/项目介绍.md) · [开发与提交日志规范](Docs/开发与提交日志规范.md) · [变更日志](CHANGELOG.md) · [架构文档](Docs/README.md)

SimplenessAgent 不是以聊天记录为核心的 Agent 产品，而是以 **Task、Plan、Step、Event、Artifact 与 Evidence** 为事实基础的本地优先任务运行时。它让任务可以被规划、执行、验证、审计与恢复，并为本地 Qwen 和远程 API 模型提供统一的运行边界。

当前项目处于基础框架阶段：已提供可运行的 Go CLI Core；Wails + Vue 桌面端、真实模型 Provider、写工具审批和多 Agent 等能力将严格遵循既定契约逐步接入。

## 快速开始

### 前置条件

- Go 1.26 或更高版本

### 验证与运行

```powershell
go test ./...
go run ./cmd/simpleness -- --data-dir .\data init
go run ./cmd/simpleness -- --data-dir .\data workspace add .
go run ./cmd/simpleness -- --data-dir .\data task create --workspace <workspace-id> --title '检查项目' --goal '生成可验证的工作区侦察报告'
go run ./cmd/simpleness -- --data-dir .\data task run <task-id>
```

命令输出均为 JSON，便于未来桌面端、自动化测试和外部客户端复用同一份事实数据。

## 当前已实现

- 版本化领域契约与 SQLite/WAL 迁移。
- Task/Step 状态机、追加事件、DAG 计划校验和 Checkpoint。
- 授权工作区边界与只读文件工具。
- 内容寻址 Artifact、Evidence 和恢复读取。
- Mock Provider 边界及 CLI 端到端侦察闭环。

详细边界、验证结果与后续计划见 [Docs/11-基础框架实现状态.md](Docs/11-基础框架实现状态.md)。

## English

SimplenessAgent is a local-first, task-centric Agent framework. Its foundation is deliberately a Go CLI Core rather than a chat-only prototype: task state, plans, events, artifacts, evidence and recovery are persisted independently from any future desktop UI.

## Foundation status

The initial framework is implemented and tested as a P0/P1 vertical slice:

- Versioned public contracts for Workspace, Task, Plan, Step, Event, Artifact, Evidence, Tool and Provider.
- SQLite (WAL) migrations, append-only event log, materialized task/step state and checkpoints.
- A deterministic Task state machine and DAG validator.
- Workspace-bound, read-only `list_files`, `read_file` and `search_text` tools.
- Atomic content-addressed Artifact storage and evidence links.
- A Mock Provider boundary and a CLI that drives the same Core an eventual Wails client will use.
- An OpenAI-compatible `/v1` Provider adapter with normalized chat/tool calls, SSE streaming, cancellation and active capability probing. It is available to the future Worker, but intentionally not wired into the safe reconnaissance-only CLI yet.
- A bounded read-only Agent Worker with a fixed executor contract, one-tool-at-a-time loop, allowlist/risk/Schema checks, repeated-action blocking and token/duration budgets. It remains decoupled from task-state persistence until the verifier boundary is complete.
- Integration, recovery, state-machine, plan and path-boundary tests.

The implemented runner intentionally executes a safe reconnaissance step only. Model-driven planning/execution and mutating tools are defined by contracts but remain disabled until their approvals, write-ahead intent records and recovery checks are complete.

See [Docs/11-基础框架实现状态.md](Docs/11-基础框架实现状态.md) for the implementation boundary and next increments.

## Provider adapter

The adapter is a library boundary, configured by the eventual Deployment service rather than command-line flags. It accepts a base URL such as `http://127.0.0.1:8080/v1`, a model ID and an optional API key; callers should always use a bounded `context.Context` for `ProbeCapabilities`, because that operation sends small text, streaming and tool-shape probes.

## Prerequisites

- Go 1.26 or later

## Verify and run

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./...
& 'C:\Program Files\Go\bin\go.exe' run ./cmd/simpleness -- --data-dir .\data init
& 'C:\Program Files\Go\bin\go.exe' run ./cmd/simpleness -- --data-dir .\data workspace add .
& 'C:\Program Files\Go\bin\go.exe' run ./cmd/simpleness -- --data-dir .\data task create --workspace <workspace-id> --title 'Inspect project' --goal 'Create a verifiable workspace reconnaissance report'
& 'C:\Program Files\Go\bin\go.exe' run ./cmd/simpleness -- --data-dir .\data task run <task-id>
```

All command output is JSON so a future desktop client, integration tests and automation can consume the same views.
