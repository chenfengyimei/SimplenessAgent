# 变更日志

本项目遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/) 的记录方式，并使用[语义化版本](https://semver.org/lang/zh-CN/)管理版本号。

## [Unreleased]

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

[Unreleased]: https://github.com/chenfengyimei/SimplenessAgent/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/chenfengyimei/SimplenessAgent/releases/tag/v0.1.0
