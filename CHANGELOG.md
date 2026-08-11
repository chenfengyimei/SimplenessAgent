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

[Unreleased]: https://github.com/chenfengyimei/SimplenessAgent/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/chenfengyimei/SimplenessAgent/releases/tag/v0.1.0
