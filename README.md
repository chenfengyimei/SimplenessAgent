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

使用 OpenAI-compatible 模型工作流时，仅在当前进程环境设置 `SIMPLENESS_API_KEY`（本地无认证 Runtime 可不设置），随后执行：

```powershell
go run ./cmd/simpleness -- --data-dir .\data deployment add --name local --endpoint http://127.0.0.1:8080/v1 --model model-id
go run ./cmd/simpleness -- --data-dir .\data task plan --deployment <deployment-id> <task-id>
go run ./cmd/simpleness -- --data-dir .\data task run-model --deployment <deployment-id> <task-id>
```

数据库只保存可选的凭据引用，不保存 API Key。

若要使用受控单层子 Agent，创建任务时必须显式启用它；随后 Coordinator 只会运行唯一就绪的只读 Step：

```powershell
go run ./cmd/simpleness -- --data-dir .\data task create --workspace <workspace-id> --title 'Inspect project' --goal 'Create a verified report' --allow-subagents
go run ./cmd/simpleness -- --data-dir .\data task coordinate --deployment <deployment-id> <task-id>
go run ./cmd/simpleness -- --data-dir .\data task agents <task-id>
```

## 当前已实现

- 版本化领域契约与 SQLite/WAL 迁移。
- `desktop/` Wails + Vue TypeScript 白色会话工作台：按授权工作目录收纳会话，在会话内展示中文执行记录、审批卡片和本地运行诊断；不持有独立业务状态。
- Task/Step 状态机、追加事件、DAG 计划校验和 Checkpoint。
- 授权工作区边界与只读文件工具。
- 内容寻址 Artifact、Evidence 和恢复读取。
- Mock Provider 边界及 CLI 端到端侦察闭环。
- 会话级权限模式：计划模式只读；编辑模式以参数绑定审批执行文件修改和受限项目命令；开发模式可在授权工作区内直接使用受审计、无 Shell 的受限写入/命令面。

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
- Deterministic `FILE_EXISTS`, `DIFF_CONTAINS`, evidence and bounded command acceptance checks. Command checks use a closed `go_test`/`go_vet` runner allowlist, never a shell.
- A Mock Provider boundary and a CLI that drives the same Core an eventual Wails client will use.
- An OpenAI-compatible `/v1` Provider adapter with normalized chat/tool calls, SSE streaming, cancellation and active capability probing. The CLI resolves configured OpenAI-compatible deployments at runtime without persisting API keys.
- A bounded Agent Worker with a fixed executor contract, allowlist/risk/Schema checks, repeated-action blocking and token/duration budgets. Conversation-level modes keep the authority explicit: PLAN exposes inspection only; EDIT produces exact write/command proposals that stop for parameter-bound approval; DEVELOPMENT enables the same workspace-bounded direct write/project-command surface with write-ahead audit records. The command runner has no shell and currently permits only fixed `go test`, `go vet`, `npm test`, and `npm run build` forms.
- Integration, recovery, state-machine, plan and path-boundary tests.

The deterministic reconnaissance runner remains available as a safe baseline. Model-driven planning and bounded execution are available through an explicit Deployment; EDIT mutations remain behind approval, while DEVELOPMENT direct operations retain write-ahead intent and recovery boundaries.

## Desktop workbench

`desktop/` is a Wails v2 + Vue TypeScript workbench. Its Go binding intentionally exposes only task snapshot queries and the data directory; task commands and tool execution remain in the Core boundary. With Wails installed, build it from that directory with `wails build`.

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

For an OpenAI-compatible model workflow, set `SIMPLENESS_API_KEY` only in the current environment (leave it unset for local unauthenticated runtimes), then:

```powershell
& 'C:\Program Files\Go\bin\go.exe' run ./cmd/simpleness -- --data-dir .\data deployment add --name local --endpoint http://127.0.0.1:8080/v1 --model model-id
& 'C:\Program Files\Go\bin\go.exe' run ./cmd/simpleness -- --data-dir .\data task plan --deployment <deployment-id> <task-id>
& 'C:\Program Files\Go\bin\go.exe' run ./cmd/simpleness -- --data-dir .\data task run-model --deployment <deployment-id> <task-id>
```

The database stores only an optional credential reference, never the API key.

To use a bounded single-layer subagent, opt in when creating the task. The Coordinator then runs only one ready read-only Step:

```powershell
& 'C:\Program Files\Go\bin\go.exe' run ./cmd/simpleness -- --data-dir .\data task create --workspace <workspace-id> --title 'Inspect project' --goal 'Create a verified report' --allow-subagents
& 'C:\Program Files\Go\bin\go.exe' run ./cmd/simpleness -- --data-dir .\data task coordinate --deployment <deployment-id> <task-id>
& 'C:\Program Files\Go\bin\go.exe' run ./cmd/simpleness -- --data-dir .\data task agents <task-id>
```

All command output is JSON so a future desktop client, integration tests and automation can consume the same views.
