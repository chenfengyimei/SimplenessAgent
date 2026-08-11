# ADR-0011：模型 Worker 先限定为受控只读工具循环

- 状态：已采纳
- 日期：2026-08-11

首个 Agent Worker 仅可接收当前 Step 明确列出的、已注册且带参数 Schema 的 `READ` 工具。模型的 Tool Call 必须通过白名单、风险、JSON Schema 与重复动作检查后才会进入 Tool Registry；每轮最多执行一个工具，并受 Step 的迭代、Token 与时长预算约束。

Worker 不推进 Task/Step 状态、不创建 Artifact/Evidence、不运行写工具，也不把模型的完成声明视为完成事实。这些职责继续由后续调度、验证与持久化层承担。此边界使模型驱动循环可独立用 Mock Provider Golden Case 验证，同时保留当前 CLI 的确定性只读侦察闭环。
