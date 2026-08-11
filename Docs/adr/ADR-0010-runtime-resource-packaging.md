# ADR-0010：Runtime 与模型采用独立资源包

- 状态：已采纳；实现计划中
- 日期：2026-08-11

主程序不强制包含大模型。未来默认管理 llama.cpp Runtime；模型与 Runtime 均带 Manifest、校验和、许可证和可恢复下载状态。
