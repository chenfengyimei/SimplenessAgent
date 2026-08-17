package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/xm/simplenessagent/internal/app"
	"github.com/xm/simplenessagent/internal/diagnostics"
	"github.com/xm/simplenessagent/internal/provider/openai"
	"github.com/xm/simplenessagent/pkg/contracts"
)

// App struct
type App struct {
	ctx         context.Context
	service     *app.Service
	openErr     error
	apiKeys     map[string]string
	credentials credentialStore
	logger      *diagnostics.Logger
}

type ConversationView struct {
	Conversation contracts.Task                  `json:"conversation"`
	Messages     []contracts.ConversationMessage `json:"messages"`
	Turns        []ConversationTurn              `json:"turns"`
}

type ConversationTurn struct {
	Snapshot app.TaskSnapshot `json:"snapshot"`
	Report   TurnReportView   `json:"report"`
}

type LongConversationCycle struct {
	View   ConversationView                 `json:"view"`
	TaskID string                           `json:"task_id"`
	Cycle  contracts.LongHorizonCycleResult `json:"cycle"`
}

type TurnReportView struct {
	Summary         string                 `json:"summary"`
	ToolName        string                 `json:"tool_name"`
	Files           []string               `json:"files"`
	Truncated       bool                   `json:"truncated"`
	PendingWrite    *app.PendingWriteBatch `json:"pending_write,omitempty"`
	PendingCommand  *app.PendingCommand    `json:"pending_command,omitempty"`
	PendingQuestion *PendingQuestion       `json:"pending_question,omitempty"`
	InputTokens     int                    `json:"input_tokens"`
	OutputTokens    int                    `json:"output_tokens"`
	Iterations      int                    `json:"iterations"`
	DurationSeconds float64                `json:"duration_seconds"`
}

type PendingQuestion struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
	Context  string   `json:"context"`
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{apiKeys: map[string]string{}, credentials: windowsCredentialStore{}}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	dataDir, err := desktopDataDir()
	if err != nil {
		a.openErr = err
		return
	}
	a.logger, _ = diagnostics.Open(filepath.Join(dataDir, "logs"))
	a.logInfo("desktop", "application startup", nil)
	a.service, a.openErr = app.Open(ctx, app.Config{DataDir: dataDir, ResolveProvider: a.resolveProvider})
	if a.openErr != nil {
		a.logError("desktop", "core open failed", a.openErr, nil)
	}
}

func (a *App) resolveProvider(deployment contracts.Deployment) (contracts.ChatProvider, error) {
	if deployment.ProviderType != "openai_compatible" {
		return nil, contracts.NewError(contracts.ErrCapabilityUnsupported, "desktop supports only openai_compatible deployments")
	}
	apiKey := a.apiKeys[deployment.ID]
	if deployment.CredentialRef != "" && strings.TrimSpace(apiKey) == "" {
		var err error
		apiKey, err = a.credentialStore().Load(deployment.ID)
		if err != nil {
			return nil, contracts.NewError(contracts.ErrAuthenticationFailed, "API Key is unavailable; open 模型与设置, enter the API Key, and save the selected model")
		}
		a.apiKeys[deployment.ID] = apiKey
	}
	return openai.New(openai.Config{BaseURL: deployment.Endpoint, APIKey: apiKey, Model: deployment.Model, DeploymentID: deployment.ID})
}

func (a *App) shutdown(_ context.Context) {
	a.logInfo("desktop", "application shutdown", nil)
	if a.service != nil {
		_ = a.service.Close()
	}
}

func (a *App) core() (*app.Service, error) {
	if a.openErr != nil {
		return nil, a.openErr
	}
	return a.service, nil
}

// ListTasks is a read-only query over Core snapshots. Desktop commands below
// call the same App Service validation and state-machine boundaries as the CLI.
func (a *App) ListTasks() ([]app.TaskSnapshot, error) {
	service, err := a.core()
	if err != nil {
		return nil, err
	}
	tasks, err := service.ListTasks(a.ctx, "")
	if err != nil {
		return nil, err
	}
	result := make([]app.TaskSnapshot, 0, len(tasks))
	for _, task := range tasks {
		snapshot, err := service.GetTaskSnapshot(a.ctx, task.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, snapshot)
	}
	return result, nil
}

func (a *App) ListConversations() ([]contracts.Task, error) {
	service, err := a.core()
	if err != nil {
		return nil, err
	}
	items, err := service.ListConversationRoots(a.ctx)
	if err != nil {
		a.logError("conversation", "list conversations failed", err, nil)
	}
	return items, err
}

func (a *App) GetConversation(conversationID string) (ConversationView, error) {
	service, err := a.core()
	if err != nil {
		return ConversationView{}, err
	}
	conversation, err := service.GetTaskSnapshot(a.ctx, conversationID)
	if err != nil {
		a.logError("conversation", "read conversation failed", err, map[string]string{"conversation_id": conversationID})
		return ConversationView{}, err
	}
	messages, err := service.ListConversationMessages(a.ctx, conversationID)
	if err != nil {
		a.logError("conversation", "read messages failed", err, map[string]string{"conversation_id": conversationID})
		return ConversationView{}, err
	}
	turns := make([]ConversationTurn, 0, len(messages))
	seenTurns := map[string]bool{}
	for _, message := range messages {
		if message.TurnTaskID == "" || seenTurns[message.TurnTaskID] {
			continue
		}
		seenTurns[message.TurnTaskID] = true
		snapshot, snapshotErr := service.GetTaskSnapshot(a.ctx, message.TurnTaskID)
		if snapshotErr == nil {
			report, reportErr := buildTurnReport(service, message.TurnTaskID, snapshot)
			if reportErr != nil {
				report = TurnReportView{Summary: "本轮已保存执行记录；报告内容暂不可读取。"}
			}
			turns = append(turns, ConversationTurn{Snapshot: snapshot, Report: report})
		}
	}
	return ConversationView{Conversation: conversation.Task, Messages: messages, Turns: turns}, nil
}

func (a *App) ListWorkspaces() ([]contracts.Workspace, error) {
	service, err := a.core()
	if err != nil {
		return nil, err
	}
	return service.ListWorkspaces(a.ctx)
}

func (a *App) CreateWorkspace(name, path string) (contracts.Workspace, error) {
	service, err := a.core()
	if err != nil {
		return contracts.Workspace{}, err
	}
	return service.CreateWorkspace(a.ctx, name, path)
}

func (a *App) CreateTask(workspaceID, title, goal string) (app.TaskSnapshot, error) {
	service, err := a.core()
	if err != nil {
		return app.TaskSnapshot{}, err
	}
	created, _, err := service.CreateTask(a.ctx, app.CreateTaskInput{WorkspaceID: workspaceID, Title: title, Goal: goal, AcceptanceCriteria: []contracts.AcceptanceCriterion{{ID: "agent_report", Type: contracts.AcceptanceEvidenceExists, Description: "bounded agent report persisted", Spec: map[string]interface{}{"kind": "AGENT_REPORT"}}}})
	if err != nil {
		return app.TaskSnapshot{}, err
	}
	return service.GetTaskSnapshot(a.ctx, created.ID)
}

func (a *App) ListDeployments() ([]contracts.Deployment, error) {
	service, err := a.core()
	if err != nil {
		return nil, err
	}
	return service.ListDeployments(a.ctx)
}

// ConfigureOpenAICompatibleDeployment persists only model metadata in SQLite.
// API keys are encrypted by Windows Credential Manager and referenced from the
// deployment, never serialized into the task database.
func (a *App) ConfigureOpenAICompatibleDeployment(name, endpoint, model, apiKey string) (contracts.Deployment, error) {
	service, err := a.core()
	if err != nil {
		return contracts.Deployment{}, err
	}
	name = strings.TrimSpace(name)
	items, err := service.ListDeployments(a.ctx)
	if err != nil {
		return contracts.Deployment{}, err
	}
	for _, item := range items {
		if item.Name != name {
			continue
		}
		if item.ProviderType != "openai_compatible" {
			return contracts.Deployment{}, contracts.NewError(contracts.ErrInvalidInput, "an existing deployment with this name uses another provider type")
		}
		if strings.TrimSpace(apiKey) != "" {
			if err := a.credentialStore().Save(item.ID, strings.TrimSpace(apiKey)); err != nil {
				return contracts.Deployment{}, fmt.Errorf("store API Key securely: %w", err)
			}
			a.apiKeys[item.ID] = strings.TrimSpace(apiKey)
			item.CredentialRef = "windows-credential-manager"
		}
		item.Endpoint = endpoint
		item.Model = model
		item.Location = "DESKTOP"
		item.Enabled = true
		return service.UpdateDeployment(a.ctx, item)
	}
	credentialRef := ""
	if strings.TrimSpace(apiKey) != "" {
		credentialRef = "windows-credential-manager"
	}
	created, err := service.CreateDeployment(a.ctx, contracts.Deployment{Name: name, ProviderType: "openai_compatible", Location: "DESKTOP", Endpoint: endpoint, Model: model, CredentialRef: credentialRef, Enabled: true})
	if err != nil {
		return contracts.Deployment{}, err
	}
	if strings.TrimSpace(apiKey) != "" {
		if err := a.credentialStore().Save(created.ID, strings.TrimSpace(apiKey)); err != nil {
			return contracts.Deployment{}, fmt.Errorf("store API Key securely: %w", err)
		}
		a.apiKeys[created.ID] = strings.TrimSpace(apiKey)
	}
	return created, nil
}

func (a *App) credentialStore() credentialStore {
	if a.credentials == nil {
		a.credentials = windowsCredentialStore{}
	}
	return a.credentials
}

func (a *App) ProbeDeployment(deploymentID string) (contracts.CapabilitySnapshot, error) {
	service, err := a.core()
	if err != nil {
		return contracts.CapabilitySnapshot{}, err
	}
	health, snapshot, err := service.ProbeDeployment(a.ctx, deploymentID)
	if err != nil {
		return contracts.CapabilitySnapshot{}, err
	}
	if !health.Healthy {
		return contracts.CapabilitySnapshot{}, fmt.Errorf("model health check failed: %s", health.Message)
	}
	return snapshot, err
}

func (a *App) GeneratePlan(taskID, deploymentID string) (contracts.PlanVersion, error) {
	service, err := a.core()
	if err != nil {
		return contracts.PlanVersion{}, err
	}
	return service.GeneratePlan(a.ctx, taskID, deploymentID)
}

func (a *App) RunAgent(taskID, deploymentID string) (app.TaskSnapshot, error) {
	service, err := a.core()
	if err != nil {
		return app.TaskSnapshot{}, err
	}
	return service.RunModelPlan(a.ctx, app.RunModelStepInput{TaskID: taskID, DeploymentID: deploymentID})
}

// ApproveConversationWrite executes the exact pending file proposal shown in
// the chat after the user has explicitly approved it.
func (a *App) ApproveConversationWrite(taskID, stepID string) (ConversationView, error) {
	service, err := a.core()
	if err != nil {
		return ConversationView{}, err
	}
	snapshot, err := service.ApprovePendingWrite(a.ctx, taskID, stepID, time.Now().Add(10*time.Minute))
	if err != nil {
		a.logError("agent", "approve conversation write failed", err, map[string]string{"turn_task_id": taskID, "step_id": stepID})
		return ConversationView{}, err
	}
	conversationID := snapshot.Task.ConversationID
	if conversationID == "" {
		conversationID = snapshot.Task.ID
	}
	a.logInfo("agent", "approved conversation write completed", map[string]string{"conversation_id": conversationID, "turn_task_id": taskID, "step_id": stepID})
	return a.GetConversation(conversationID)
}

// ApproveConversationCommand executes the exact EDIT-mode project command
// shown in the conversation after explicit user confirmation.
func (a *App) ApproveConversationCommand(taskID, stepID string) (ConversationView, error) {
	service, err := a.core()
	if err != nil {
		return ConversationView{}, err
	}
	snapshot, err := service.ApprovePendingCommand(a.ctx, taskID, stepID, time.Now().Add(10*time.Minute))
	if err != nil {
		a.logError("agent", "approve conversation command failed", err, map[string]string{"turn_task_id": taskID, "step_id": stepID})
		return ConversationView{}, err
	}
	conversationID := snapshot.Task.ConversationID
	if conversationID == "" {
		conversationID = snapshot.Task.ID
	}
	a.logInfo("agent", "approved conversation command completed", map[string]string{"conversation_id": conversationID, "turn_task_id": taskID, "step_id": stepID})
	return a.GetConversation(conversationID)
}

// SendMessage creates a scoped task from a user message, then lets the Agent
// generate and execute its read-only plan. The task/event store remains the
// durable conversation record behind the desktop chat surface.
func (a *App) StartConversation(workspaceID, message, deploymentID, permissionMode string) (ConversationView, error) {
	service, err := a.core()
	if err != nil {
		return ConversationView{}, err
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return ConversationView{}, contracts.NewError(contracts.ErrInvalidInput, "message is required")
	}
	mode, err := desktopPermissionMode(permissionMode)
	if err != nil {
		return ConversationView{}, err
	}
	created, _, err := service.CreateTask(a.ctx, app.CreateTaskInput{WorkspaceID: workspaceID, Title: compactTitle(message), Goal: message, DeploymentID: deploymentID, AcceptanceCriteria: conversationAcceptance(deploymentID), AllowWriteProposals: mode == contracts.PermissionModeEdit, PermissionMode: mode})
	if err != nil {
		a.logError("conversation", "create conversation failed", err, map[string]string{"workspace_id": workspaceID, "deployment_id": deploymentID})
		return ConversationView{}, err
	}
	a.logInfo("conversation", "conversation started", map[string]string{"conversation_id": created.ID, "workspace_id": workspaceID, "deployment_id": deploymentID})
	if err = service.SaveConversationMessage(a.ctx, contracts.ConversationMessage{ConversationID: created.ID, TurnTaskID: created.ID, Role: "user", Content: message}); err != nil {
		a.logError("conversation", "save initial user message failed", err, map[string]string{"conversation_id": created.ID})
		return ConversationView{}, err
	}
	return a.executeConversationTurn(service, created, created.ID, deploymentID)
}

func (a *App) SendConversationMessage(conversationID, message, deploymentID, permissionMode string) (ConversationView, error) {
	service, err := a.core()
	if err != nil {
		return ConversationView{}, err
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return ConversationView{}, contracts.NewError(contracts.ErrInvalidInput, "message is required")
	}
	conversation, err := service.GetTaskSnapshot(a.ctx, conversationID)
	if err != nil {
		a.logError("conversation", "read conversation before send failed", err, map[string]string{"conversation_id": conversationID})
		return ConversationView{}, err
	}
	mode, err := desktopPermissionMode(permissionMode)
	if err != nil {
		return ConversationView{}, err
	}
	created, _, err := service.CreateTask(a.ctx, app.CreateTaskInput{WorkspaceID: conversation.Task.WorkspaceID, Title: compactTitle(message), Goal: message, DeploymentID: deploymentID, AcceptanceCriteria: conversationAcceptance(deploymentID), AllowWriteProposals: mode == contracts.PermissionModeEdit, PermissionMode: mode})
	if err != nil {
		a.logError("conversation", "create conversation turn failed", err, map[string]string{"conversation_id": conversationID, "deployment_id": deploymentID})
		return ConversationView{}, err
	}
	created.ConversationID = conversationID
	if err = service.SetConversationID(a.ctx, created.ID, conversationID); err != nil {
		a.logError("conversation", "link conversation turn failed", err, map[string]string{"conversation_id": conversationID, "turn_task_id": created.ID})
		return ConversationView{}, err
	}
	if err = service.SaveConversationMessage(a.ctx, contracts.ConversationMessage{ConversationID: conversationID, TurnTaskID: created.ID, Role: "user", Content: message}); err != nil {
		a.logError("conversation", "save user message failed", err, map[string]string{"conversation_id": conversationID, "turn_task_id": created.ID})
		return ConversationView{}, err
	}
	return a.executeConversationTurn(service, created, conversationID, deploymentID)
}

func (a *App) StartLongConversation(workspaceID, message, deploymentID, permissionMode string) (LongConversationCycle, error) {
	service, err := a.core()
	if err != nil {
		return LongConversationCycle{}, err
	}
	message = strings.TrimSpace(message)
	if message == "" || strings.TrimSpace(deploymentID) == "" {
		return LongConversationCycle{}, contracts.NewError(contracts.ErrInvalidInput, "message and deployment are required for long-horizon mode")
	}
	mode, err := desktopPermissionMode(permissionMode)
	if err != nil {
		return LongConversationCycle{}, err
	}
	created, _, err := service.CreateLongHorizonTask(a.ctx, app.CreateLongHorizonTaskInput{DeploymentID: deploymentID, CreateTaskInput: app.CreateTaskInput{WorkspaceID: workspaceID, Title: compactTitle(message), Goal: message, PermissionMode: mode, AllowWriteProposals: mode == contracts.PermissionModeEdit}})
	if err != nil {
		return LongConversationCycle{}, err
	}
	if err = service.SaveConversationMessage(a.ctx, contracts.ConversationMessage{ConversationID: created.ID, TurnTaskID: created.ID, Role: "user", Content: message}); err != nil {
		return LongConversationCycle{}, err
	}
	return a.advanceLongConversation(service, created.ID, created.ID)
}

func (a *App) SendLongConversationMessage(conversationID, message, deploymentID, permissionMode string) (LongConversationCycle, error) {
	service, err := a.core()
	if err != nil {
		return LongConversationCycle{}, err
	}
	message = strings.TrimSpace(message)
	if message == "" || strings.TrimSpace(deploymentID) == "" {
		return LongConversationCycle{}, contracts.NewError(contracts.ErrInvalidInput, "message and deployment are required for long-horizon mode")
	}
	view, viewErr := a.GetConversation(conversationID)
	if viewErr == nil {
		for index := len(view.Turns) - 1; index >= 0; index-- {
			waiting := view.Turns[index].Snapshot
			if waiting.Horizon == nil || waiting.Task.Status != contracts.TaskWaitingUser {
				continue
			}
			if err = service.SaveConversationMessage(a.ctx, contracts.ConversationMessage{ConversationID: conversationID, TurnTaskID: waiting.Task.ID, Role: "user", Content: message}); err != nil {
				return LongConversationCycle{}, err
			}
			if _, err = service.ResumeLongHorizonTask(a.ctx, waiting.Task.ID); err != nil {
				return LongConversationCycle{}, err
			}
			return a.advanceLongConversation(service, waiting.Task.ID, conversationID)
		}
	}
	conversation, err := service.GetTaskSnapshot(a.ctx, conversationID)
	if err != nil {
		return LongConversationCycle{}, err
	}
	mode, err := desktopPermissionMode(permissionMode)
	if err != nil {
		return LongConversationCycle{}, err
	}
	created, _, err := service.CreateLongHorizonTask(a.ctx, app.CreateLongHorizonTaskInput{DeploymentID: deploymentID, CreateTaskInput: app.CreateTaskInput{WorkspaceID: conversation.Task.WorkspaceID, Title: compactTitle(message), Goal: message, PermissionMode: mode, AllowWriteProposals: mode == contracts.PermissionModeEdit}})
	if err != nil {
		return LongConversationCycle{}, err
	}
	if err = service.SetConversationID(a.ctx, created.ID, conversationID); err != nil {
		return LongConversationCycle{}, err
	}
	if err = service.SaveConversationMessage(a.ctx, contracts.ConversationMessage{ConversationID: conversationID, TurnTaskID: created.ID, Role: "user", Content: message}); err != nil {
		return LongConversationCycle{}, err
	}
	return a.advanceLongConversation(service, created.ID, conversationID)
}

func (a *App) AdvanceLongConversation(taskID string) (LongConversationCycle, error) {
	service, err := a.core()
	if err != nil {
		return LongConversationCycle{}, err
	}
	snapshot, err := service.GetTaskSnapshot(a.ctx, taskID)
	if err != nil {
		return LongConversationCycle{}, err
	}
	conversationID := snapshot.Task.ConversationID
	if conversationID == "" {
		conversationID = snapshot.Task.ID
	}
	return a.advanceLongConversation(service, taskID, conversationID)
}

func (a *App) ResumeLongConversation(taskID string) (LongConversationCycle, error) {
	service, err := a.core()
	if err != nil {
		return LongConversationCycle{}, err
	}
	if _, err = service.ResumeLongHorizonTask(a.ctx, taskID); err != nil {
		return LongConversationCycle{}, err
	}
	return a.AdvanceLongConversation(taskID)
}

func (a *App) CancelLongConversation(taskID string) (contracts.HorizonState, error) {
	service, err := a.core()
	if err != nil {
		return contracts.HorizonState{}, err
	}
	return service.CancelLongHorizonTask(a.ctx, taskID)
}

func (a *App) GetLongHorizonStatus(taskID string) (contracts.HorizonState, error) {
	service, err := a.core()
	if err != nil {
		return contracts.HorizonState{}, err
	}
	return service.GetLongHorizonStatus(a.ctx, taskID)
}

// buildLongHorizonCompletionSummary composes the user-facing completion message:
// what was accomplished, which files were produced, where they live and how to
// launch the result. Everything is deterministic — the model's own final step
// report is quoted, never re-generated. A task that completed without leaving
// any product file is reported as a warning, never as a success.
func (a *App) buildLongHorizonCompletionSummary(service *app.Service, taskID string, cycle contracts.LongHorizonCycleResult) (string, error) {
	snapshot, err := service.GetTaskSnapshot(a.ctx, taskID)
	if err != nil {
		return "", err
	}
	goal := strings.TrimSpace(snapshot.Task.Goal)
	permissionNotice := ""
	if mode, modeErr := contracts.ParsePermissionMode(snapshot.Task.Spec.PermissionProfileID); modeErr == nil && mode == contracts.PermissionModePlan {
		permissionNotice = "\n\n【原因】本任务创建时选择了只读（PLAN）权限：执行阶段只能查看和检索文件，无法创建或修改任何文件，因此整个任务只产出了侦察报告。"
	}
	workspaceRoot := ""
	if workspaceItem, workspaceErr := service.GetWorkspaceByID(a.ctx, snapshot.Task.WorkspaceID); workspaceErr == nil {
		workspaceRoot = workspaceItem.RootPath
	}
	files, launch := scanWorkspaceForLaunch(workspaceRoot)
	var builder strings.Builder
	if len(files) == 0 {
		builder.WriteString("⚠️ 任务已标记完成，但工作区没有产出任何文件。\n\n【目标】" + goal + "\n")
		builder.WriteString("\n【结果】最终验收只确认了 Agent 报告的存在，没有可交付的文件。" + permissionNotice)
		builder.WriteString("\n\n【如何重做】新建任务时把权限设为 EDIT（可写入）：Agent 提出的每个文件修改仍会先弹出审批，你确认后才会写入磁盘。")
		builder.WriteString(fmt.Sprintf("\n\n【执行统计】完成 %d 步，Token 用量见任务详情。", cycle.StepsCompleted))
		return builder.String(), nil
	}
	builder.WriteString("✅ 长程任务已完成，最终验收通过。\n\n")
	builder.WriteString("【目标】" + goal + "\n")
	if agentSummary := latestAgentReportSummary(service, taskID); agentSummary != "" {
		builder.WriteString("\n【Agent 最终报告】\n" + agentSummary + "\n")
	}
	builder.WriteString("\n【产出文件】（工作区：" + workspaceRoot + "）\n")
	for _, file := range files {
		builder.WriteString("- " + file + "\n")
	}
	if launch != "" {
		builder.WriteString("\n【如何启动】\n" + launch)
	}
	builder.WriteString(fmt.Sprintf("\n\n【执行统计】完成 %d 步（另有 %d 步来自失败后被替换的旧计划段，未重复执行），Token 用量见任务详情。", cycle.StepsCompleted, cycle.StepsPlanned-cycle.StepsCompleted))
	return builder.String(), nil
}

func latestAgentReportSummary(service *app.Service, taskID string) string {
	content, err := service.ReadTaskArtifact(context.Background(), taskID, "AGENT_REPORT")
	if err != nil {
		return ""
	}
	var report contracts.AgentReport
	if json.Unmarshal(content, &report) != nil {
		return ""
	}
	return strings.TrimSpace(report.Summary)
}

// scanWorkspaceForLaunch lists bounded, depth-limited product files and infers
// a deterministic launch instruction from well-known entry files.
func scanWorkspaceForLaunch(root string) ([]string, string) {
	if strings.TrimSpace(root) == "" {
		return nil, ""
	}
	ignored := map[string]bool{"node_modules": true, ".git": true, "dist": true, "build": true, "bin": true, "vendor": true, "__pycache__": true, ".cache": true}
	files := []string{}
	hasIndexHTML, hasPackageJSON, hasGoMod, hasRequirements, hasMainPy, hasCargo := false, false, false, false, false, false
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		depth := strings.Count(rel, string(os.PathSeparator))
		if entry.IsDir() {
			if depth >= 2 || ignored[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if depth > 2 || len(files) >= 25 {
			return nil
		}
		switch strings.ToLower(entry.Name()) {
		case "index.html":
			hasIndexHTML = true
		case "package.json":
			hasPackageJSON = true
		case "go.mod":
			hasGoMod = true
		case "requirements.txt":
			hasRequirements = true
		case "main.py", "app.py":
			hasMainPy = true
		case "cargo.toml":
			hasCargo = true
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	launch := ""
	switch {
	case hasPackageJSON:
		launch = "进入工作区目录后执行：npm install（首次）→ npm run dev 或 npm start；如需打包再执行 npm run build。"
	case hasIndexHTML:
		launch = "直接用浏览器打开工作区里的 index.html 即可（双击或拖入浏览器窗口）。"
	case hasGoMod:
		launch = "进入工作区目录后执行：go run .（需本机安装 Go）。"
	case hasMainPy && hasRequirements:
		launch = "进入工作区目录后执行：pip install -r requirements.txt（首次）→ python main.py。"
	case hasMainPy:
		launch = "进入工作区目录后执行：python main.py。"
	case hasCargo:
		launch = "进入工作区目录后执行：cargo run（需本机安装 Rust）。"
	}
	return files, launch
}

func (a *App) advanceLongConversation(service *app.Service, taskID, conversationID string) (LongConversationCycle, error) {	cycle, err := service.AdvanceLongHorizonTask(a.ctx, taskID)
	if err != nil {
		a.logError("long-horizon", "cycle failed", err, map[string]string{"conversation_id": conversationID, "turn_task_id": taskID})
		return LongConversationCycle{}, err
	}
	a.emitAgentStatus(map[string]interface{}{"conversation_id": conversationID, "status": strings.ToLower(string(cycle.Status)), "message": fmt.Sprintf("长程 Agent：%s · %s", cycle.Stage, cycle.Action)})
	if cycle.Status != contracts.HorizonActive || cycle.AwaitingCheckpoint {
		message := fmt.Sprintf("长程任务已执行到 %s 阶段：%s。已规划 %d 步，完成 %d 步。", cycle.Stage, cycle.Action, cycle.StepsPlanned, cycle.StepsCompleted)
		if cycle.StepsPlanned > cycle.StepsCompleted && cycle.Action != "COMPLETED" {
			message += fmt.Sprintf("（其中 %d 步来自失败后被替换的旧计划段，不会重复执行）", cycle.StepsPlanned-cycle.StepsCompleted)
		}
		if cycle.Action == "COMPLETED" || cycle.Action == "TERMINAL" && cycle.Status == contracts.HorizonCompleted {
			if summary, summaryErr := a.buildLongHorizonCompletionSummary(service, taskID, cycle); summaryErr == nil {
				message = summary
			} else {
				a.logError("long-horizon", "build completion summary failed", summaryErr, map[string]string{"conversation_id": conversationID, "turn_task_id": taskID})
			}
		}
		if cycle.CheckpointReason != "" {
			message += "\n\n暂停原因：" + cycle.CheckpointReason
		}
		if err = service.SaveConversationMessage(a.ctx, contracts.ConversationMessage{ConversationID: conversationID, TurnTaskID: taskID, Role: "assistant", Content: message}); err != nil {
			return LongConversationCycle{}, err
		}
	}
	view, err := a.GetConversation(conversationID)
	if err != nil {
		return LongConversationCycle{}, err
	}
	return LongConversationCycle{View: view, TaskID: taskID, Cycle: cycle}, nil
}

func desktopPermissionMode(value string) (contracts.PermissionMode, error) {
	if strings.TrimSpace(value) == "" {
		return contracts.PermissionModeEdit, nil
	}
	return contracts.ParsePermissionMode(value)
}

func (a *App) executeConversationTurn(service *app.Service, created contracts.Task, conversationID, deploymentID string) (ConversationView, error) {
	a.logInfo("agent", "turn execution started", map[string]string{"conversation_id": conversationID, "turn_task_id": created.ID, "deployment_id": deploymentID})
	a.emitAgentStatus(map[string]interface{}{"conversation_id": conversationID, "status": "thinking", "message": "Agent 正在理解需求并分析上下文…"})
	var snapshot app.TaskSnapshot
	var err error
	if strings.TrimSpace(deploymentID) == "" {
		a.emitAgentStatus(map[string]interface{}{"conversation_id": conversationID, "status": "recon", "message": "正在侦察工作区…"})
		snapshot, err = service.RunTask(a.ctx, created.ID)
	} else {
		sections, contextErr := service.ConversationContextSections(a.ctx, created.ID, conversationID, created.Goal)
		if contextErr != nil {
			a.logError("agent", "assemble conversation context failed", contextErr, map[string]string{"conversation_id": conversationID, "turn_task_id": created.ID})
			return ConversationView{}, contextErr
		}
		a.logInfo("agent", "conversation context assembled", map[string]string{"conversation_id": conversationID, "turn_task_id": created.ID, "sections": fmt.Sprint(len(sections))})
		a.emitAgentStatus(map[string]interface{}{"conversation_id": conversationID, "status": "model", "message": "正在调用模型生成执行计划…"})
		snapshot, err = service.RunModelPlan(a.ctx, app.RunModelStepInput{TaskID: created.ID, DeploymentID: deploymentID, ContextSections: sections})
	}
	if err != nil {
		a.logError("agent", "turn execution failed", err, map[string]string{"conversation_id": conversationID, "turn_task_id": created.ID, "deployment_id": deploymentID})
		a.emitAgentStatus(map[string]interface{}{"conversation_id": conversationID, "status": "error", "message": "本轮执行失败"})
		response := "本轮 Agent 执行失败：" + userFacingError(err) + "。详细诊断已保存到“模型与设置 → 运行诊断”。"
		if saveErr := service.SaveConversationMessage(a.ctx, contracts.ConversationMessage{ConversationID: conversationID, TurnTaskID: created.ID, Role: "assistant", Content: response}); saveErr != nil {
			return ConversationView{}, saveErr
		}
		return a.GetConversation(conversationID)
	}
	report, reportErr := buildTurnReport(service, created.ID, snapshot)
	if reportErr != nil {
		a.logError("agent", "read turn report failed", reportErr, map[string]string{"conversation_id": conversationID, "turn_task_id": created.ID})
	}
	response := report.Summary
	if strings.TrimSpace(response) == "" {
		response = conversationResponse(snapshot)
	}
	if err = service.SaveConversationMessage(a.ctx, contracts.ConversationMessage{ConversationID: conversationID, TurnTaskID: created.ID, Role: "assistant", Content: response}); err != nil {
		a.logError("conversation", "save assistant response failed", err, map[string]string{"conversation_id": conversationID, "turn_task_id": created.ID})
		return ConversationView{}, err
	}
	a.logInfo("agent", "turn execution completed", map[string]string{"conversation_id": conversationID, "turn_task_id": created.ID, "status": string(snapshot.Task.Status)})
	a.emitAgentStatus(map[string]interface{}{"conversation_id": conversationID, "status": "completed", "message": "本轮执行完成"})
	return a.GetConversation(conversationID)
}

func (a *App) emitAgentStatus(payload map[string]interface{}) {
	// Wails runtime helpers terminate the process when called without the
	// lifecycle context. Core/desktop tests intentionally use a plain context.
	if a.ctx == nil || a.ctx.Value("events") == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "agent:status", payload)
}

func conversationAcceptance(deploymentID string) []contracts.AcceptanceCriterion {
	if strings.TrimSpace(deploymentID) == "" {
		return []contracts.AcceptanceCriterion{{ID: "recon_report", Type: contracts.AcceptanceEvidenceExists, Description: "deterministic workspace reconnaissance persisted", Spec: map[string]interface{}{"kind": "RECON_REPORT"}}}
	}
	return []contracts.AcceptanceCriterion{{ID: "agent_report", Type: contracts.AcceptanceEvidenceExists, Description: "bounded model agent report persisted", Spec: map[string]interface{}{"kind": "AGENT_REPORT"}}}
}

func conversationResponse(snapshot app.TaskSnapshot) string {
	status := "已完成"
	if snapshot.Task.Status != contracts.TaskCompleted {
		status = "执行结束，当前状态为 " + string(snapshot.Task.Status)
	}
	return status + "。我已在授权工作区内完成只读侦察，并保存了可复核的文件清单与证据。"
}

func buildTurnReport(service *app.Service, taskID string, snapshot app.TaskSnapshot) (TurnReportView, error) {
	view := TurnReportView{Summary: "本轮已生成只读执行记录。"}
	if snapshot.Task.Status == contracts.TaskCompleted {
		view.Summary = "已完成本轮只读侦察，并保存可复核的结果。"
	}
	content, err := service.ReadTaskArtifact(context.Background(), taskID, "AGENT_REPORT")
	if err == nil {
		var report contracts.AgentReport
		if err = json.Unmarshal(content, &report); err != nil {
			return view, err
		}
		view.Summary = strings.TrimSpace(report.Summary)
		view.InputTokens = report.Usage.InputTokens
		view.OutputTokens = report.Usage.OutputTokens
		view.Iterations = report.Iterations
		if !snapshot.Task.CreatedAt.IsZero() {
			view.DurationSeconds = time.Since(snapshot.Task.CreatedAt).Seconds()
		}
		toolNames := make([]string, 0, len(report.ToolResults))
		for _, result := range report.ToolResults {
			if result.Data != nil {
				if files, ok := result.Data["files"].([]interface{}); ok {
					for _, item := range files {
						if name, nameOK := item.(string); nameOK {
							view.Files = append(view.Files, name)
						}
					}
				}
			}
			toolNames = append(toolNames, chineseToolName(result))
		}
		view.ToolName = strings.Join(uniqueStrings(toolNames), "、")
		if view.ToolName == "" {
			view.ToolName = "未调用工具"
		}
		if snapshot.Task.Status == contracts.TaskWaitingApproval {
			pending, pendingErr := service.PendingWrite(context.Background(), taskID)
			if pendingErr == nil {
				view.PendingWrite = &pending
				view.Summary = "Agent 已准备好工作区修改，正在等待你的确认；确认后将原子写入并完成验证。"
				view.ToolName = "等待写入审批"
			}
			pendingCommand, commandErr := service.PendingCommand(context.Background(), taskID)
			if commandErr == nil {
				view.PendingCommand = &pendingCommand
				view.Summary = "Agent 已准备好执行项目命令，正在等待你的确认；确认后会在当前工作目录内受限执行并保存输出。"
				view.ToolName = "等待命令审批"
			}
		}
		if snapshot.Task.Status == contracts.TaskWaitingUser {
			questionContent, questionErr := service.ReadTaskArtifact(context.Background(), taskID, "USER_QUESTION")
			if questionErr == nil {
				var uq struct {
					Question string   `json:"Question"`
					Options  []string `json:"Options"`
					Context  string   `json:"Context"`
				}
				if json.Unmarshal(questionContent, &uq) == nil {
					view.PendingQuestion = &PendingQuestion{Question: uq.Question, Options: uq.Options, Context: uq.Context}
					view.Summary = uq.Question
					view.ToolName = "等待用户回答"
				}
			}
		}
		return view, nil
	}
	if snapshot.Task.Status == contracts.TaskWaitingApproval {
		pending, pendingErr := service.PendingWrite(context.Background(), taskID)
		if pendingErr == nil {
			view.PendingWrite = &pending
			view.Summary = "Agent 已准备好工作区修改，正在等待你的确认；确认后将原子写入并完成验证。"
			view.ToolName = "等待写入审批"
			return view, nil
		}
		pendingCommand, commandErr := service.PendingCommand(context.Background(), taskID)
		if commandErr == nil {
			view.PendingCommand = &pendingCommand
			view.Summary = "Agent 已准备好执行项目命令，正在等待你的确认；确认后会在当前工作目录内受限执行并保存输出。"
			view.ToolName = "等待命令审批"
			return view, nil
		}
	}
	content, err = service.ReadTaskArtifact(context.Background(), taskID, "RECON_REPORT")
	if err != nil {
		return view, err
	}
	var report struct {
		Result contracts.ToolResult `json:"result"`
	}
	if err = json.Unmarshal(content, &report); err != nil {
		return view, err
	}
	view.ToolName = "列出文件"
	view.Truncated, _ = report.Result.Data["truncated"].(bool)
	if files, ok := report.Result.Data["files"].([]interface{}); ok {
		for _, item := range files {
			if name, nameOK := item.(string); nameOK {
				view.Files = append(view.Files, name)
			}
		}
	}
	view.Summary = fmt.Sprintf("已调用“列出文件”工具，在授权工作区发现 %d 个文件。", len(view.Files))
	if view.Truncated {
		view.Summary += " 文件清单已按安全上限截断。"
	}
	return view, nil
}

func chineseToolName(result contracts.ToolResult) string {
	if _, found := result.Data["files"]; found {
		return "列出文件"
	}
	if _, found := result.Data["content"]; found {
		return "读取文件"
	}
	if _, found := result.Data["matches"]; found {
		return "搜索文本"
	}
	return "只读工具"
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func userFacingError(err error) string {
	if domain, ok := err.(*contracts.Error); ok {
		switch domain.Code {
		case contracts.ErrContextOverflow:
			return "当前模型可用上下文不足，系统已在调用前停止本轮，避免无效消耗。可在模型设置中探测或提高上下文容量，或缩短对话内容"
		case contracts.ErrBudgetExceeded, contracts.ErrOutputLimitReached:
			return "模型本轮输出超过受控上限，系统已停止继续执行并保留诊断记录。请降低任务粒度或使用支持 max_tokens 的模型服务"
		}
		return domain.Message
	}
	return "本地执行器发生未预期错误"
}

func compactTitle(message string) string {
	value := []rune(strings.Join(strings.Fields(message), " "))
	if len(value) > 32 {
		return string(value[:32]) + "…"
	}
	return string(value)
}

func (a *App) GetTaskSnapshot(taskID string) (app.TaskSnapshot, error) {
	service, err := a.core()
	if err != nil {
		return app.TaskSnapshot{}, err
	}
	return service.GetTaskSnapshot(a.ctx, taskID)
}

func (a *App) DataDir() (string, error) {
	service, err := a.core()
	if err != nil {
		return "", err
	}
	return service.DataDir(), nil
}

// ListDiagnosticLogs returns recent structured, local-only entries. API keys
// and similarly named fields are redacted before a line is ever written.
func (a *App) ListDiagnosticLogs(limit int) ([]diagnostics.Entry, error) {
	if a.logger == nil {
		return []diagnostics.Entry{}, nil
	}
	return a.logger.Query(limit)
}

// RecordClientLog lets the WebView report rendering and bridge failures that
// would otherwise look like a black screen with no actionable server trace.
func (a *App) RecordClientLog(level, message string, fields map[string]string) {
	if strings.EqualFold(level, "error") {
		a.logError("frontend", message, nil, fields)
		return
	}
	a.logInfo("frontend", message, fields)
}

func (a *App) logInfo(component, message string, fields map[string]string) {
	if a.logger != nil {
		a.logger.Info(component, message, fields)
	}
}

func (a *App) logError(component, message string, err error, fields map[string]string) {
	if err != nil {
		if fields == nil {
			fields = map[string]string{}
		}
		fields["error"] = err.Error()
	}
	if a.logger != nil {
		a.logger.Error(component, message, fields)
	}
}

func desktopDataDir() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "SimplenessAgent"), nil
}

func (a *App) ListTaskArtifacts(taskID string) ([]contracts.Artifact, error) {
	service, err := a.core()
	if err != nil {
		return nil, err
	}
	return service.ListTaskArtifacts(a.ctx, taskID)
}

func (a *App) ReadTaskArtifact(taskID, kind string) (string, error) {
	service, err := a.core()
	if err != nil {
		return "", err
	}
	content, err := service.ReadTaskArtifact(a.ctx, taskID, kind)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

type PlanStepView struct {
	StepID       string   `json:"step_id"`
	Title        string   `json:"title"`
	Goal         string   `json:"goal"`
	Status       string   `json:"status"`
	Dependencies []string `json:"dependencies"`
	AllowedTools []string `json:"allowed_tools"`
	Risk         string   `json:"risk"`
}

type PlanViewData struct {
	PlanID          string         `json:"plan_id"`
	TaskID          string         `json:"task_id"`
	Revision        int            `json:"revision"`
	HorizonID       string         `json:"horizon_id,omitempty"`
	StageID         string         `json:"stage_id,omitempty"`
	SegmentIndex    int            `json:"segment_index,omitempty"`
	TerminalSegment bool           `json:"terminal_segment,omitempty"`
	Summary         string         `json:"summary"`
	Reason          string         `json:"reason"`
	Steps           []PlanStepView `json:"steps"`
	CreatedAt       time.Time      `json:"created_at"`
}

func (a *App) GetTaskPlan(taskID string) (PlanViewData, error) {
	service, err := a.core()
	if err != nil {
		return PlanViewData{}, err
	}
	plan, err := service.GetLatestPlan(a.ctx, taskID)
	if err != nil {
		return PlanViewData{}, err
	}
	snapshot, _ := service.GetTaskSnapshot(a.ctx, taskID)
	stepStatus := make(map[string]string)
	for _, step := range snapshot.Steps {
		stepStatus[step.StepID] = string(step.Status)
	}
	steps := make([]PlanStepView, 0, len(plan.Steps))
	for _, spec := range plan.Steps {
		steps = append(steps, PlanStepView{
			StepID:       spec.StepID,
			Title:        spec.Title,
			Goal:         spec.Goal,
			Status:       stepStatus[spec.StepID],
			Dependencies: spec.Dependencies,
			AllowedTools: spec.AllowedTools,
			Risk:         string(spec.Risk),
		})
	}
	return PlanViewData{
		PlanID:          plan.PlanID,
		TaskID:          plan.TaskID,
		Revision:        plan.Revision,
		HorizonID:       plan.HorizonID,
		StageID:         plan.StageID,
		SegmentIndex:    plan.SegmentIndex,
		TerminalSegment: plan.TerminalSegment,
		Summary:         plan.Summary,
		Reason:          plan.Reason,
		Steps:           steps,
		CreatedAt:       plan.CreatedAt,
	}, nil
}
