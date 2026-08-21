

## Codely Structured Memories

### User

### Feedback
- [2026-08-17 10:41:19] 任何交付物完成后必须主动告知用户产物路径与使用/启动方式：wails build 后写明 `D:\xm\SimplenessAgent\desktop\build\bin\desktop.exe`；Agent 任务/游戏等产出写明工作区路径、关键文件与启动方法。**Why:** 用户多次提醒"打包好的链接记得发一下"，且 2026-08-17 长程任务完成后追问"文件在哪怎么启动"。**How to apply:** 每次交付（打包、生成产物、任务完成）都在回复中明确写出路径与启动步骤，不等用户问。

### Project
- [2026-08-17 10:20:06] [project] SimplenessAgent 的用户本地部署使用思考型模型（reasoning_content 输出隐藏推理）。Why: 2026-08-17 曾因 1536 输出预算全被隐藏推理耗尽、content 为空，导致长程规划 PLAN_INVALID 空响应失败。How to apply: 新增模型调用点时给思考留输出余量（规划类 ≥3072），正文为空时回退解析 reasoning_content，错误信息带 finish_reason/token 诊断。
- [2026-08-17 10:41:27] [project] 用户通过自己产品的桌面端（长程模式 + 本地思考型模型）做真实任务做内测（如 2026-08-17 网页版我的世界），失败截图/状态即 bug 报告。Why: dogfooding 闭环，用户报障方式是粘贴应用状态面板。How to apply: 收到"长程任务已执行到 X 阶段：Y"类反馈时，按错误码直接在仓库定位对应 fail-closed 路径修复，而非询问复现步骤。
- [2026-08-17 10:59:43] [project] 思考型模型对 fail-closed 解码路径的通用兼容原则：模型输出解码一律用容错提取（JSON 对象扫描、字段别名、忽略额外字段、修复提示带完整形状示例），禁止 DisallowUnknownFields 式严格解码；重复只读请求回放缓存结果而非整步硬失败；预算上限要按"构建一个游戏"量级设定（长程已扩到 40 步/4h）。Why: 2026-08-17 dogfooding 中规划器/验证器/执行器三处过严 fail-closed 连环烧光重规划预算（verifier 加 confidence 字段被拒、重复搜索 REPEATED_ACTION、20 步耗尽 BUDGET_BLOCKED 不可恢复）。How to apply: 新增任何解析模型 JSON 的代码点先参考 internal/horizon/horizon.go 的 decodeCandidate 模式；新增硬失败路径前考虑能否确定性降级（回放/默认值/宽容解码）。
- [2026-08-17 11:23:40] [project] 桌面端权限选择器是假完成任务事故源：permissionMode 从 localStorage/会话恢复，用户常停在 PLAN（只读）不自知；长程+PLAN 组合曾让"做游戏"任务空跑 25 步/218K token 后假完成（验收标准仅"Agent 报告存在"，只读即可满足）。Why: 2026-08-17 D:\cs\cs 空目录事故，用户质问"执行了什么"。How to apply: 任何"任务完成"路径必须先确定性校验产出（工作区有无可交付文件），无产出只能警示不能报成功；涉及产出文件的功能在 PLAN 权限下要在启动前与检查点两处前置警告。
- [2026-08-17 14:26:01] [project] 内测假完成事故的第二层成因与防线（2026-08-17 连续三轮零产出）：1) 规划器契约示例只展示只读工具，思考型模型锚定示例 → IMPLEMENT 全程只读（已修：IMPLEMENT 写意图门禁）；2) 规划器给了写步骤、执行器仍可用只读证据"完成"WRITE 步骤（已修：WriteCompletionRequired 写义务守卫，service 仅对长程开启，单计划保留原回合级守卫）；3) 用户不重启应用继续用旧 exe（已修：底栏"客户端构建 <时间>"取 exe 修改时间，报障先核对构建时间）。**Why:** 防线只设一层就会被绕过，且旧二进制问题让一次修复验证白跑 20 分钟。**How to apply:** 给 Agent 加行为约束时同时考虑规划器/执行器两层 + 用户可感知的运行版本标识；修 bug 后提醒用户"完全退出并重启应用"再验证。

### Reference

