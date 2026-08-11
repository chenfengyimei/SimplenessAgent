# 变更日志

本项目遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/) 的记录方式，并使用[语义化版本](https://semver.org/lang/zh-CN/)管理版本号。

## [Unreleased]

### 2026-08-11 Approved file writer

- 新增显式注册的 `write_file` 执行器：参数、工作区边界和预期内容哈希均通过校验后，才会调用参数绑定的审批回调。
- 写入采用同目录临时文件、同步落盘和原子替换；内容哈希不匹配、审批拒绝或路径越界时均不产生文件修改。
- 该工具默认不注册到 Worker；持久化 Intent/Approval 票据的接线见后续本版本记录。

### 2026-08-11 Persisted write approvals

- 新增 App Service 的 `ApproveWriteFile` 与 `WriteFile`：仅允许当前 Plan 显式授权且处于运行中的 `WRITE` 步骤执行已审批的路径范围。
- 首次 Intent 与审计事件、审批票据与审计事件分别以 SQLite 单事务保存；审批按任务、步骤、工具与参数哈希精确单次消费。
- 成功和失败结果均回写同一 Intent；重试发现目标内容已经一致时安全收敛，不重复消耗票据或产生 Intent 审计事件。

### 2026-08-11 Safe running-task recovery

- 新增版本化 Checkpoint 读取模型和运行边界的自动 Checkpoint；同一事件序列只保存一次快照。
- 新增显式恢复操作：遗留 `RUNNING` 任务先保存观测到的状态，再原子转为 `PAUSED` 并记录 `TASK_RECOVERY_PAUSED`；不会自动重放步骤或副作用。

### 2026-08-11 Local replanning

- 新增 `ReplanTask`：仅允许恢复后处于 `PAUSED` 的任务创建新的模型 Plan Revision，并受 Task 的 `MaxReplans` 预算限制。
- 重规划上下文只包含前一版 Plan、持久化步骤状态和显式原因；模型不得复用旧 Step ID，Plan 以 `PLAN_REPLANNED` 审计后才将任务转回 `READY`。

### 2026-08-11 Context package compiler

- 新增版本化 `ContextPackage`、段落来源与 Token 预算契约，以及确定性 Context 编译器。
- 编译器在预留输出 Token 后按优先级选择段落、限制同源段落数量、规范化来源并返回省略项；没有段落可装入预算时 fail-closed 为 `CONTEXT_OVERFLOW`。

### 2026-08-11 Worker context package boundary

- Worker 现可接收并优先渲染 `ContextPackage` 的已选段及来源；自由字符串上下文仅保留为兼容路径。
- Worker 在调用模型前校验 Package 版本、任务/步骤绑定、段落 Token 总数和预算；无效或超限 Package 不会触发 Provider 请求。

### 2026-08-11 Diff acceptance verifier

- 新增 `DIFF_CONTAINS` 接受条件：在授权工作区内验证指定文件的 `git diff HEAD` 是否包含期望文本。
- diff 调用不经过 shell，禁用外部 diff 与彩色输出，并设置固定超时；非 Git 工作区、路径越界、超时和不匹配均 fail-closed。

### 2026-08-11 Memory FTS foundation

- 新增版本化、来源绑定的 `MemoryRecord`、SQLite 迁移和 FTS5 索引；写入要求 Workspace 作用域及至少一个 Event/Artifact 来源。
- 新增 App Service 的记忆创建和检索入口；检索按 Workspace、ACTIVE/PINNED 状态和有效期过滤，避免归档或其他项目记忆污染当前上下文。

### 2026-08-11 Memory context retrieval

- 新增显式 `CompileMemoryContext`：仅检索当前 Task 所属 Workspace 的可用记忆，再交给 Context Package 编译器统一执行 Token 预算和来源多样性裁剪。
- 编译结果保留 `memory:<id>`、源 Event 和源 Artifact 引用，并以置信度与重要性确定段落优先级。

### 2026-08-11 Local Skill runtime foundation

- 新增版本化本地 Skill manifest 与按需 `SKILL.md` 加载；发现操作只读取元数据，避免未选中 Skill 正文进入上下文。
- Manifest 必须声明可用工具和工作区 Scope，未知工具、路径越界、符号链接、超大或多 JSON 文档均被拒绝；当前 App 仅提供只读工具表。

### 2026-08-11 Worker Skill boundary

- Worker 现可渲染显式选中的 Skill 指令；Skill 必须绑定已有 Context Package，指令 Token 计入其剩余预算。
- Worker 在 Provider 调用前拒绝重复/不完整 Skill、超出 Step 工具白名单或 Scope 的声明，以及超预算的 Skill；Skill 不会改变实际 Tool 调用白名单。

### 2026-08-11 Model step execution closure

- 新增 App Service `RunModelStep`：为唯一就绪的 `READ` Step 创建受预算上下文、调用受控 Worker、持久化 `AGENT_REPORT` Artifact 与 Evidence 后才进入任务验证。
- 该入口复用 Task/Step 状态机、Checkpoint 和 FinalReport；Worker 失败会将运行中的 Step/Task 标记失败，写工具仍不会注册到模型执行环境。

### 2026-08-11 Model CLI workflow

- CLI 新增 `deployment add|list|probe`、`task plan --deployment` 与 `task run-model --deployment`，将持久化 Deployment、模型规划和受控单步骤执行接入同一 JSON CLI。
- API Key 只从运行时 `SIMPLENESS_API_KEY` 读取；持久化 Deployment 只可记录不透明凭据引用。CLI 同时兼容 `go run ... -- --data-dir ...` 调用形式。

### 2026-08-11 Bounded command acceptance verifier

- 新增 `COMMAND` 接受条件的确定性实现：仅允许 `go_test` 与 `go_vet` runner，以及明确声明的工作区相对 Go 包路径；不会执行 shell 或模型给出的任意命令行。
- 每次验证固定在工作区工作目录运行，使用当前 Go runtime、默认 30 秒/最大 60 秒超时和 1 MiB 输出上限；未知 runner、路径逃逸、超时、超输出或非零退出均 fail-closed。

### 计划中

- OpenAI-compatible Provider、流式调用与能力探测。
- Agent Worker、写工具审批、幂等恢复与验证器。

## [0.1.0] - 2026-08-11

### 新增

- 建立 Go Agent Core、CLI 入口及推荐目录结构。
- 定义版本化 Task、Plan、Step、Event、Artifact、Evidence、Tool 与 Provider 契约。
- 实现 SQLite WAL 迁移、追加事件日志、Task/Step 状态机与 Checkpoint。
- 实现 Plan JSON DAG 校验、工作区路径边界、只读文件工具和 Artifact 内容校验。
- 实现 Mock Provider 与安全侦察任务端到端闭环。
- 增加单元、路径安全、计划校验和重启读取集成测试。
- 增加架构 ADR、项目介绍和开发日志规范。

### 安全

- 默认 fail-closed：尚未完成审批、Write-ahead 与恢复审计的写入、危险、网络及外部副作用工具保持禁用。

### 2026-08-11 Provider PoC

- 新增 OpenAI-compatible `/v1` Provider：同步文本与 Tool Call、SSE 流式事件、取消、错误分类和主动能力探测。
- 新增 Provider Tool Call/Stream 事件契约，以及 HTTP 适配器的本地契约测试。

### 2026-08-11 Read-only Worker

- 新增模型驱动的受控单工具循环：固定执行合同、工具白名单、`READ` 风险限制、JSON Schema 校验、重复动作阻断及 Token/时长/迭代预算。
- 新增 Mock Provider Golden Cases，覆盖正常只读闭环、越权工具、无效参数、重复调用、写工具与预算停止。

### 2026-08-11 Model Planner

- 新增只读模型 Planner：Plan JSON 候选解析、本地身份字段覆盖、DAG/预算/工具策略验证及 Golden Cases。

### 2026-08-11 Deployments

- 新增 Deployment、CapabilitySnapshot 迁移、持久化与 App Service 探测入口。

- App Service 现可通过已启用 Deployment 生成并审计持久化模型 Plan Revision。

### 2026-08-11 Verifier

- 新增 Evidence 驱动的确定性验证器和 FinalReport，任务完成前强制验证接收条件。
- FinalReport 以内容寻址 Artifact 持久化。
- `FILE_EXISTS` 接收条件现在经授权工作区边界验证。

### 2026-08-11 Write-ahead intents

- 新增规范化 Tool Intent、幂等恢复与原子审批票据消费基础。

[Unreleased]: https://github.com/chenfengyimei/SimplenessAgent/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/chenfengyimei/SimplenessAgent/releases/tag/v0.1.0
