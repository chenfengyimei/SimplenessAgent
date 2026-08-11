# ADR-0003：SQLite Event Store 与物化状态并存

- 状态：已采纳并已实现基础版
- 日期：2026-08-11

SQLite 使用 WAL 与外键。Task/Step 物化状态与 Event Append 在同一事务提交；事件序列按 Aggregate 单调递增。Artifact 正文不写入数据库，而以内容哈希保存在文件存储中。
