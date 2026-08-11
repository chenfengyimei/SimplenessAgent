# ADR-0008：首期多 Agent 限制为一层

- 状态：已采纳；实现计划中
- 日期：2026-08-11

未来子 Agent 仅允许 Coordinator 创建，最大深度为一层，必须以 AgentReport/Handoff 交付。递归 Agent 和自由群聊不进入基础框架。
