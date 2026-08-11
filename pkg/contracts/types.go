// Package contracts contains the versioned, transport-safe objects shared by
// the core, CLI, desktop client, and future extensions.
package contracts

import (
	"encoding/json"
	"time"
)

const SchemaVersion = 1

type TaskStatus string

const (
	TaskDraft           TaskStatus = "DRAFT"
	TaskPlanning        TaskStatus = "PLANNING"
	TaskReady           TaskStatus = "READY"
	TaskRunning         TaskStatus = "RUNNING"
	TaskWaitingApproval TaskStatus = "WAITING_APPROVAL"
	TaskWaitingUser     TaskStatus = "WAITING_USER"
	TaskVerifying       TaskStatus = "VERIFYING"
	TaskCompleted       TaskStatus = "COMPLETED"
	TaskFailed          TaskStatus = "FAILED"
	TaskBlocked         TaskStatus = "BLOCKED"
	TaskPaused          TaskStatus = "PAUSED"
	TaskCancelled       TaskStatus = "CANCELLED"
)

func (s TaskStatus) Terminal() bool {
	return s == TaskCompleted || s == TaskFailed || s == TaskBlocked || s == TaskCancelled
}

type StepStatus string

const (
	StepPending         StepStatus = "PENDING"
	StepReady           StepStatus = "READY"
	StepRunning         StepStatus = "RUNNING"
	StepWaitingApproval StepStatus = "WAITING_APPROVAL"
	StepWaitingUser     StepStatus = "WAITING_USER"
	StepVerifying       StepStatus = "VERIFYING"
	StepCompleted       StepStatus = "COMPLETED"
	StepFailed          StepStatus = "FAILED"
	StepBlocked         StepStatus = "BLOCKED"
	StepSkipped         StepStatus = "SKIPPED"
	StepSuperseded      StepStatus = "SUPERSEDED"
	StepCancelled       StepStatus = "CANCELLED"
)

func (s StepStatus) Terminal() bool {
	return s == StepCompleted || s == StepFailed || s == StepBlocked || s == StepSkipped || s == StepSuperseded || s == StepCancelled
}

type RiskClass string

const (
	RiskRead               RiskClass = "READ"
	RiskWrite              RiskClass = "WRITE"
	RiskDangerous          RiskClass = "DANGEROUS"
	RiskExternalSideEffect RiskClass = "EXTERNAL_SIDE_EFFECT"
)

type AcceptanceType string

const (
	AcceptanceEvidenceExists AcceptanceType = "EVIDENCE_EXISTS"
	AcceptanceFileExists     AcceptanceType = "FILE_EXISTS"
	AcceptanceCommand        AcceptanceType = "COMMAND"
)

type Constraint struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Hard bool   `json:"hard"`
}

type AcceptanceCriterion struct {
	ID          string                 `json:"id"`
	Type        AcceptanceType         `json:"type"`
	Description string                 `json:"description"`
	Spec        map[string]interface{} `json:"spec"`
}

type TaskBudget struct {
	MaxDurationMS        int64    `json:"max_duration_ms"`
	MaxSteps             int      `json:"max_steps"`
	MaxReplans           int      `json:"max_replans"`
	MaxModelInputTokens  int      `json:"max_model_input_tokens"`
	MaxModelOutputTokens int      `json:"max_model_output_tokens"`
	MaxCost              *float64 `json:"max_cost"`
}

type TaskSpec struct {
	Version             int                   `json:"version"`
	TaskID              string                `json:"task_id"`
	WorkspaceID         string                `json:"workspace_id"`
	Title               string                `json:"title"`
	Goal                string                `json:"goal"`
	NonGoals            []string              `json:"non_goals"`
	Constraints         []Constraint          `json:"constraints"`
	AcceptanceCriteria  []AcceptanceCriterion `json:"acceptance_criteria"`
	Assumptions         []string              `json:"assumptions"`
	DeploymentID        string                `json:"deployment_id"`
	PermissionProfileID string                `json:"permission_profile_id"`
	Budget              TaskBudget            `json:"budget"`
	AllowSubagents      bool                  `json:"allow_subagents"`
	CreatedAt           time.Time             `json:"created_at"`
}

type Workspace struct {
	ID        string    `json:"id"`
	Version   int       `json:"version"`
	Name      string    `json:"name"`
	RootPath  string    `json:"root_path"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Deployment describes a configured model endpoint. CredentialRef is an opaque
// reference owned by a credential store; the secret itself is never persisted.
type Deployment struct {
	ID                   string    `json:"deployment_id"`
	Version              int       `json:"version"`
	Name                 string    `json:"name"`
	ProviderType         string    `json:"provider_type"`
	Location             string    `json:"location"`
	Endpoint             string    `json:"endpoint"`
	CredentialRef        string    `json:"credential_ref,omitempty"`
	Model                string    `json:"model"`
	Runtime              string    `json:"runtime,omitempty"`
	RuntimeVersion       string    `json:"runtime_version,omitempty"`
	Quantization         string    `json:"quantization,omitempty"`
	ModelProfileID       string    `json:"model_profile_id,omitempty"`
	CapabilitySnapshotID string    `json:"capability_snapshot_id,omitempty"`
	Enabled              bool      `json:"enabled"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type Task struct {
	ID          string     `json:"id"`
	Version     int        `json:"version"`
	WorkspaceID string     `json:"workspace_id"`
	Title       string     `json:"title"`
	Goal        string     `json:"goal"`
	Status      TaskStatus `json:"status"`
	Spec        TaskSpec   `json:"spec"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type PlanVersion struct {
	Version        int        `json:"version"`
	PlanID         string     `json:"plan_id"`
	TaskID         string     `json:"task_id"`
	Revision       int        `json:"revision"`
	ParentPlanID   string     `json:"parent_plan_id,omitempty"`
	Reason         string     `json:"reason"`
	Summary        string     `json:"summary"`
	Steps          []StepSpec `json:"steps"`
	CreatedByAgent string     `json:"created_by_agent_id"`
	CreatedAt      time.Time  `json:"created_at"`
}

type StepBudget struct {
	MaxAttempts     int   `json:"max_attempts"`
	MaxIterations   int   `json:"max_iterations"`
	MaxDurationMS   int64 `json:"max_duration_ms"`
	MaxInputTokens  int   `json:"max_input_tokens"`
	MaxOutputTokens int   `json:"max_output_tokens"`
}

type ExpectedOutput struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

type StepSpec struct {
	Version            int                   `json:"version"`
	StepID             string                `json:"step_id"`
	Title              string                `json:"title"`
	Goal               string                `json:"goal"`
	Dependencies       []string              `json:"dependencies"`
	AllowedTools       []string              `json:"allowed_tools"`
	WorkspaceScopes    []string              `json:"workspace_scopes"`
	ExpectedOutputs    []ExpectedOutput      `json:"expected_outputs"`
	AcceptanceCriteria []AcceptanceCriterion `json:"acceptance_criteria"`
	Risk               RiskClass             `json:"risk"`
	Budget             StepBudget            `json:"budget"`
	ExecutionMode      string                `json:"execution_mode"`
	PreferredRole      string                `json:"preferred_role"`
}

type StepRuntime struct {
	StepID        string     `json:"step_id"`
	PlanID        string     `json:"plan_id"`
	Status        StepStatus `json:"status"`
	EvidenceIDs   []string   `json:"evidence_ids"`
	ArtifactIDs   []string   `json:"artifact_ids"`
	LastErrorCode string     `json:"last_error_code,omitempty"`
}

type EventEnvelope struct {
	EventID       string                 `json:"event_id"`
	EventVersion  int                    `json:"event_version"`
	EventType     string                 `json:"event_type"`
	AggregateType string                 `json:"aggregate_type"`
	AggregateID   string                 `json:"aggregate_id"`
	RunID         string                 `json:"run_id,omitempty"`
	Sequence      int64                  `json:"sequence"`
	Timestamp     time.Time              `json:"timestamp"`
	ActorType     string                 `json:"actor_type"`
	ActorID       string                 `json:"actor_id"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	CausationID   string                 `json:"causation_id,omitempty"`
	Payload       map[string]interface{} `json:"payload"`
}

// Checkpoint is an immutable, point-in-time task snapshot. Snapshot is kept as
// JSON so clients can evolve their read model without changing the SQLite
// checkpoint envelope.
type Checkpoint struct {
	ID        string          `json:"checkpoint_id"`
	Version   int             `json:"version"`
	TaskID    string          `json:"task_id"`
	Sequence  int64           `json:"sequence"`
	Snapshot  json.RawMessage `json:"snapshot"`
	CreatedAt time.Time       `json:"created_at"`
}

type Artifact struct {
	ID                    string    `json:"artifact_id"`
	Version               int       `json:"version"`
	Kind                  string    `json:"kind"`
	MediaType             string    `json:"media_type"`
	StorageURI            string    `json:"storage_uri"`
	ContentHash           string    `json:"content_hash"`
	SizeBytes             int64     `json:"size_bytes"`
	Summary               string    `json:"summary"`
	WorkspaceRelativePath string    `json:"workspace_relative_path,omitempty"`
	TaskID                string    `json:"task_id,omitempty"`
	StepID                string    `json:"step_id,omitempty"`
	Verified              bool      `json:"verified"`
	CreatedAt             time.Time `json:"created_at"`
}

type Evidence struct {
	ID                 string    `json:"evidence_id"`
	Kind               string    `json:"kind"`
	Claim              string    `json:"claim"`
	ArtifactID         string    `json:"artifact_id"`
	Location           string    `json:"location"`
	VerificationMethod string    `json:"verification_method"`
	VerifiedAt         time.Time `json:"verified_at"`
	Confidence         float64   `json:"confidence"`
}

type ToolDefinition struct {
	Version              int                    `json:"version"`
	Name                 string                 `json:"name"`
	ToolVersion          string                 `json:"tool_version"`
	Description          string                 `json:"description"`
	ParametersSchema     map[string]interface{} `json:"parameters_schema,omitempty"`
	RiskClass            RiskClass              `json:"risk_class"`
	RequiredCapabilities []string               `json:"required_capabilities"`
	DefaultTimeoutMS     int64                  `json:"default_timeout_ms"`
	MaxOutputBytes       int                    `json:"max_output_bytes"`
	SupportsCancel       bool                   `json:"supports_cancel"`
}

type ToolResult struct {
	Version     int                    `json:"version"`
	ToolCallID  string                 `json:"tool_call_id"`
	Status      string                 `json:"status"`
	Summary     string                 `json:"summary"`
	Data        map[string]interface{} `json:"data,omitempty"`
	ArtifactIDs []string               `json:"artifact_ids,omitempty"`
	EvidenceIDs []string               `json:"evidence_ids,omitempty"`
	Error       *ToolError             `json:"error,omitempty"`
	StartedAt   time.Time              `json:"started_at"`
	CompletedAt time.Time              `json:"completed_at"`
}

type ToolCallRecord struct {
	ID             string    `json:"tool_call_id"`
	Version        int       `json:"version"`
	RunID          string    `json:"run_id,omitempty"`
	StepID         string    `json:"step_id"`
	ToolName       string    `json:"tool_name"`
	ArgumentsHash  string    `json:"normalized_arguments_hash"`
	IdempotencyKey string    `json:"idempotency_key"`
	Risk           RiskClass `json:"risk_class"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ApprovalTicket struct {
	ID            string    `json:"approval_id"`
	TaskID        string    `json:"task_id"`
	StepID        string    `json:"step_id"`
	ToolName      string    `json:"tool_name"`
	ArgumentsHash string    `json:"normalized_arguments_hash"`
	Decision      string    `json:"decision"`
	ExpiresAt     time.Time `json:"expires_at,omitempty"`
	UsesRemaining int       `json:"uses_remaining"`
	CreatedAt     time.Time `json:"created_at"`
}

type ToolError struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Retryable   bool   `json:"retryable"`
	Remediation string `json:"remediation,omitempty"`
}
