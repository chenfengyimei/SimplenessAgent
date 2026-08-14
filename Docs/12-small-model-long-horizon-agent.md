# 7B-14B small-model long-horizon agent

The optional `INCREMENTAL_HORIZON` execution strategy keeps the legacy `SINGLE_PLAN` path as the default and advances software-engineering tasks through five deterministic stages:

`DISCOVER -> DESIGN -> IMPLEMENT -> VERIFY_REPAIR -> FINALIZE`

Each service call performs one durable action: generate one 1-4 step segment, execute one step, or close a segment/stage. The desktop and CLI may repeatedly call this entry point; no correctness state is held only in process memory.

## Public entry points

- App service: `CreateLongHorizonTask`, `AdvanceLongHorizonTask`, `ResumeLongHorizonTask`, `CancelLongHorizonTask`, `GetLongHorizonStatus`.
- CLI: `task create-long`, `task advance`, `task horizon`, `task resume`, `task cancel`.
- Desktop: select the long-horizon strategy before starting a conversation. The task card shows stage, completed/planned steps, token use, checkpoint reason, and resume control.

Defaults are a two-hour deadline, 20 planned steps, four local replans, four steps per segment, and an 8K verified usable context window. A verified 4K deployment is rejected before task creation.

## Small-model boundaries

- Planner, Executor, and Verifier use persisted profiles on the same deployment. Their default output limits are 1536, 768, and 512 tokens.
- Planner returns only a `NextSegmentCandidate`. IDs, scopes, budgets, risk, permission, and plan metadata are assigned and validated by the core.
- Executor receives at most three tools per step and may issue at most one tool call per model turn in long-horizon mode.
- Verifier output is advisory and is stored in `STAGE_SUMMARY`; only deterministic acceptance may complete a step, stage, or task.
- Tool results are limited to 20% of the reliable window when returned to the model. Complete results remain in content-addressed task artifacts.
- Optional provider `TokenCounter` support is preferred. The fallback counts CJK conservatively, estimates other text at four characters per token, and adds a 15% margin.
- At 90% of the reliable context window requests fail closed. One provider-side overflow may trigger a traceable smaller request.

## Durable recovery

The core persists `HORIZON_PLAN`, successive `PROGRESS_LEDGER` artifacts, `STAGE_SUMMARY`, and `FAILURE_REPORT`, plus horizon state, usage, events, and role profiles in SQLite. A running tool action is never automatically replayed after interruption. Write proposals and bounded commands retain their existing approval and write-ahead intent controls.

## Validation and release gate

Mock and integration coverage runs with `go test ./...`. Desktop delivery additionally requires `npm run typecheck`, `npm run build`, and `wails build`.

The real-model release definition is in `evals/suites/long-horizon-small-model-v1.json`. The checked-in baseline intentionally remains `pending_real_deployment_runs`: named 7B and 14B deployments must each run every case at least five times before pass-rate claims can be made.
