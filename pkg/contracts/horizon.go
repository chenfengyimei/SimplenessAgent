package contracts

import "time"

type HorizonStageID string

const (
	HorizonStageDiscover     HorizonStageID = "DISCOVER"
	HorizonStageDesign       HorizonStageID = "DESIGN"
	HorizonStageImplement    HorizonStageID = "IMPLEMENT"
	HorizonStageVerifyRepair HorizonStageID = "VERIFY_REPAIR"
	HorizonStageFinalize     HorizonStageID = "FINALIZE"
)

type HorizonStage struct {
	ID             HorizonStageID `json:"stage_id"`
	Title          string         `json:"title"`
	Goal           string         `json:"goal"`
	CompletionGate string         `json:"completion_gate"`
}

type HorizonPlan struct {
	Version   int            `json:"version"`
	HorizonID string         `json:"horizon_id"`
	TaskID    string         `json:"task_id"`
	Stages    []HorizonStage `json:"stages"`
	CreatedAt time.Time      `json:"created_at"`
}

type SegmentStepCandidate struct {
	Ref              string   `json:"ref"`
	Title            string   `json:"title"`
	Goal             string   `json:"goal"`
	Dependencies     []string `json:"dependencies,omitempty"`
	ToolIntents      []string `json:"tool_intents"`
	AcceptanceIntent string   `json:"acceptance_intent"`
}

type NextSegmentCandidate struct {
	Summary         string                 `json:"summary"`
	TerminalSegment bool                   `json:"terminal_segment"`
	Steps           []SegmentStepCandidate `json:"steps"`
}

type ProgressLedger struct {
	Version           int              `json:"version"`
	HorizonID         string           `json:"horizon_id"`
	TaskID            string           `json:"task_id"`
	CurrentStage      HorizonStageID   `json:"current_stage"`
	CompletedStages   []HorizonStageID `json:"completed_stages"`
	CompletedStepIDs  []string         `json:"completed_step_ids"`
	FailedStepIDs     []string         `json:"failed_step_ids"`
	KeyFacts          []string         `json:"key_facts,omitempty"`
	OpenRisks         []string         `json:"open_risks,omitempty"`
	LatestFailureCode string           `json:"latest_failure_code,omitempty"`
	RemainingSteps    int              `json:"remaining_steps"`
	RemainingReplans  int              `json:"remaining_replans"`
	InputTokensUsed   int              `json:"input_tokens_used"`
	OutputTokensUsed  int              `json:"output_tokens_used"`
	UpdatedAt         time.Time        `json:"updated_at"`
}

type HorizonStatus string

const (
	HorizonActive    HorizonStatus = "ACTIVE"
	HorizonWaiting   HorizonStatus = "WAITING_CHECKPOINT"
	HorizonPaused    HorizonStatus = "PAUSED"
	HorizonBlocked   HorizonStatus = "BLOCKED"
	HorizonCompleted HorizonStatus = "COMPLETED"
	HorizonCancelled HorizonStatus = "CANCELLED"
)

type HorizonState struct {
	Version                 int           `json:"version"`
	HorizonID               string        `json:"horizon_id"`
	TaskID                  string        `json:"task_id"`
	Status                  HorizonStatus `json:"status"`
	Plan                    HorizonPlan   `json:"plan"`
	CurrentStageIndex       int           `json:"current_stage_index"`
	SegmentIndex            int           `json:"segment_index"`
	StepsPlanned            int           `json:"steps_planned"`
	StepsCompleted          int           `json:"steps_completed"`
	ReplansUsed             int           `json:"replans_used"`
	NoProgressCycles        int           `json:"no_progress_cycles"`
	LastEvidenceCount       int           `json:"last_evidence_count"`
	LatestLedgerArtifactID  string        `json:"latest_ledger_artifact_id,omitempty"`
	LatestFailureArtifactID string        `json:"latest_failure_artifact_id,omitempty"`
	LastProcessedPlanID     string        `json:"last_processed_plan_id,omitempty"`
	AwaitingCheckpoint      bool          `json:"awaiting_checkpoint"`
	CheckpointReason        string        `json:"checkpoint_reason,omitempty"`
	Usage                   TokenUsage    `json:"usage"`
	StartedAt               time.Time     `json:"started_at"`
	DeadlineAt              time.Time     `json:"deadline_at"`
	UpdatedAt               time.Time     `json:"updated_at"`
}

type ModelRole string

const (
	ModelRolePlanner  ModelRole = "PLANNER"
	ModelRoleExecutor ModelRole = "EXECUTOR"
	ModelRoleVerifier ModelRole = "VERIFIER"
)

type ModelRoleProfile struct {
	Version         int       `json:"version"`
	ID              string    `json:"profile_id"`
	DeploymentID    string    `json:"deployment_id"`
	Role            ModelRole `json:"role"`
	Temperature     float64   `json:"temperature"`
	MaxOutputTokens int       `json:"max_output_tokens"`
	MaxIterations   int       `json:"max_iterations"`
	MaxToolCalls    int       `json:"max_tool_calls"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// StageVerificationOpinion is advisory model output. It can identify missing
// evidence or risks, but it never authorizes a stage or task transition.
type StageVerificationOpinion struct {
	Summary          string   `json:"summary"`
	GateAppearsMet   bool     `json:"gate_appears_met"`
	EvidenceRefs     []string `json:"evidence_refs,omitempty"`
	Risks            []string `json:"risks,omitempty"`
	RecommendedCheck string   `json:"recommended_check,omitempty"`
}

type LongHorizonCycleResult struct {
	Version            int            `json:"version"`
	TaskID             string         `json:"task_id"`
	HorizonID          string         `json:"horizon_id"`
	Status             HorizonStatus  `json:"status"`
	Stage              HorizonStageID `json:"stage"`
	Action             string         `json:"action"`
	StepsPlanned       int            `json:"steps_planned"`
	StepsCompleted     int            `json:"steps_completed"`
	RemainingSteps     int            `json:"remaining_steps"`
	AwaitingCheckpoint bool           `json:"awaiting_checkpoint"`
	CheckpointReason   string         `json:"checkpoint_reason,omitempty"`
}
