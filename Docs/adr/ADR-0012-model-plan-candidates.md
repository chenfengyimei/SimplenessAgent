# ADR-0012：模型计划仅作为本地校验后的候选

- 状态：已采纳
- 日期：2026-08-11

模型 Planner 只返回 JSON `PlanVersion` 候选，不能获得工具调用能力，也不能直接写入数据库。Core 覆盖候选中的 Plan ID、Task ID、Revision、父计划、创建者和时间等事实字段，再执行 DAG、任务预算、只读工具 allowlist 和 Step 边界验证。

因此模型输出永远不是任务事实；只有后续 App Service 在同一事务中保存计划与事件后，计划才成为可运行版本。
