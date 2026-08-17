# 变更日志

本项目遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/) 的记录方式，并使用[语义化版本](https://semver.org/lang/zh-CN/)管理版本号。

## [Unreleased]

### 2026-08-17 IMPLEMENT write-intent enforcement

- 修复 EDIT 模式下长程任务零产出：规划器契约示例只展示只读工具，思考型模型锚定示例，22 步全部只读、从未请求写入工具，任务最终零文件"完成"。现 IMPLEMENT 阶段确定性门禁：当写入/命令工具可用（EDIT/DEVELOPMENT 权限）而候选段无任何写入意图时，拒绝并要求修复轮至少一个步骤引用写入工具；只读（PLAN）权限无写工具可用时跳过门禁，保持旧行为。
- 规划器契约增加 IMPLEMENT 阶段工具指引；执行器契约明确：步骤目标需要创建/修改文件时必须使用提供的写入工具提交提案，只读证据无法满足写入目标。

### 2026-08-17 Verifier tolerance, read replay, larger horizon budget

- 阶段验证器解码容错重构：复用规划器的 JSON 提取器（容忍思考前缀/代码围栏），移除 DisallowUnknownFields（思考型模型附加 confidence/verdict 等字段不再整体拒绝），接受 gate/gate_met/evidenceRefs/check 等常见别名，缺失 gate 默认 false（保守）；修复提示携带具体字段错误与完整形状示例；空响应错误改为明确诊断。
- 执行器重复只读请求不再整步失败：以协议合法的 tool 响应回放缓存的先前结果（标注重复读），迭代预算仍约束循环；重复的写入/命令类调用依旧硬拒绝。
- 长程预算扩容：默认与上限 20 步/2 小时 → 40 步/4 小时。"做个网页游戏"量级的任务此前会耗尽 20 步预算并进入不可恢复的 BUDGET_BLOCKED。

### 2026-08-17 Executor read-batching alignment

- 修复执行器 profile 每响应工具上限与执行器契约的自相矛盾：契约承诺"可一次请求多个独立只读工具"，而默认/已持久化 profile 却限制 1 个，思考型模型按契约批量请求 2 个只读工具即触发 INVALID_TOOL_CALL，修复轮再次批量后整步失败。默认上限 1→4，旧任务恢复时下限钳制为 ≥4（操作员调高的值不会被压低）。
- Worker 任务指派现在事前写明每响应工具调用上限，模型无需先违规再靠修复轮纠偏。
- 完成总结覆盖 TERMINAL 路径：任务完成后再次推进（状态面板刷新、会话重开）此前回退为模板句且不带总结，现在同样输出完整完成总结（目标、报告、文件、启动方法）。
- 旧任务执行器输出预算下限钳制 ≥1536：修复前仅 planner/verifier 有下限，旧任务执行器仍可能被 768 上限卡回空响应失败。

### 2026-08-17 Long-horizon completion summary

- 长程任务完成时不再发送模板化状态句，改为完整总结消息：目标、Agent 最终报告原文、产出文件清单（工作区根路径）、按入口文件推断的启动方法（浏览器打开 / npm / go run / python / cargo 等确定性启发式）、执行统计。文件扫描限定深度且跳过 node_modules 等目录，不调用模型。
- 暂停/失败消息在"已规划 > 已完成"时补充说明：差值来自失败后被替换的旧计划段，不会重复执行，消除"为何没执行完就完成/暂停"的歧义。

### 2026-08-17 Mutating-step read enrichment

- 长程规划器构建 Plan 时，对含写入/命令工具的步骤确定性附加该阶段全部只读工具：执行器"先读现状再写入"的自然请求不再触发 TOOL_NOT_ALLOWED 失败；只读工具不提升步骤风险等级，不扩大变更面。
- Worker 工具修复提示现在明确列出该步骤唯一允许的工具名与单响应调用上限，模型可在修复轮内改用白名单工具而非重复越界请求后整步失败。

### 2026-08-17 Reasoning-model empty-answer recovery

- OpenAI 兼容 Provider 现解析 `reasoning_content`/`reasoning` 字段：思考型本地模型把输出预算全部耗在隐藏推理、正文为空时，回退用推理文本作为答案（结构化 JSON 提取器本就容忍推理噪音）；流式路径聚合推理增量但不混入实时文本流。
- 长程规划预算提升：Planner 输出 1536→3072、Executor 768→1536、Verifier 512→1024，段内步骤输出预算 768→1536；恢复旧任务时对已持久化 Profile 施加同楼下限钳制，避免同样的空响应失败。
- 规划器空响应错误现在携带 `finish_reason` 与输出 token 诊断（指向"预算被隐藏推理耗尽"），修复轮提示改为明确指令：以 JSON 开头、禁止先思考。

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

### 2026-08-11 Single-layer agent assignments

- 新增持久化 `AgentAssignment`：Coordinator 仅可为显式允许子 Agent 的 `READY` Task 分配依赖已满足的只读 Step；分配固定为深度 1，并快照 Step 的工具白名单与工作区范围。
- Assignment 与 `AGENT_ASSIGNED` 事件在同一事务提交；同一 Step 同时最多一个 `PENDING`/`RUNNING` Assignment。此增量不启动递归 Agent、自由 Agent 通信或写权限。

### 2026-08-11 Synchronous single-agent handoff

- 新增 `RunAssignedAgent`：仅可运行深度 1 的 `PENDING` Assignment，复用既有受控只读模型步骤，分别记录 Agent 运行/完成或失败状态事件。
- 成功执行后持久化仅含 Artifact/Evidence 引用的 `AGENT_HANDOFF`；新增任务 Artifact 查询接口，供后续 Coordinator 与 UI 读取交接事实，而非读取隐藏推理或自由聊天记录。

### 2026-08-11 Bounded coordinator cycle

- 新增 `RunCoordinatorCycle`：只为显式允许子 Agent 的 `READY` Task 调度唯一依赖就绪的 `READ` Step；优先复用其 `PENDING` Assignment，再同步等待执行和 Handoff。
- Coordinator 拒绝多个就绪步骤、运行中 Assignment、非只读 Step 与 Assignment/当前 Plan 权限快照不一致的情况，保持单层、非递归和 fail-closed。

### 2026-08-11 Agent assignment recovery audit

- 新增 `RecoverRunningAgentAssignments`：遗留 `RUNNING` Assignment 先以 `AGENT_RECOVERY_FAILED` 事件持久化为失败，再复用 Task Checkpoint/暂停恢复路径。
- 恢复不会重放模型、工具或 Handoff；没有运行中 Assignment 的 Task 也不会被此入口隐式改变。

### 2026-08-11 Coordinator CLI controls

- `task create` 新增显式 `--allow-subagents`；默认仍关闭单层子 Agent。
- 新增 `task coordinate --deployment` 与 `task agents`，以 JSON 暴露既有受控 Coordinator 调度和 Assignment 查询。

### 2026-08-11 Sequential DAG coordinator cycles

- 模型步骤和受控 Coordinator 现在支持跨多个 cycle 串行执行依赖 DAG：前序只读 Step 完成后 Task 保持 `RUNNING`，直到没有未终态 Step 才进入 FinalReport 验证。
- 保持每个 cycle 至多一个 Ready Step；多 Ready Step 仍 fail-closed，尚未引入并行 Agent 或共享写入。

### 2026-08-11 Wails + Vue desktop workbench

- 新增 `desktop/` Wails v2 + Vue TypeScript 项目，并以本地 App Service 打开同一数据目录；桌面 Binding 只提供任务快照和数据目录查询。
- Vue 工作台展示 Task/Step/Event 事实并提供刷新，不维护独立任务状态，也不暴露 Provider 凭据、工具执行或审批操作。
- 已通过 Vue 生产构建、桌面子模块 `go test`/`go vet` 与 Windows Wails 生产构建验证。

### 2026-08-11 Desktop Core commands

- 桌面 Binding 新增工作区列表/创建和任务创建；所有命令都直接调用 App Service，复用路径边界、任务状态机和事件持久化。
- Vue 工作台增加工作区与任务表单；前端只提交用户输入并读取刷新后的 Task Snapshot。

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
