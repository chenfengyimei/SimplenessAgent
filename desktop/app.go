package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xm/simplenessagent/internal/app"
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

type TurnReportView struct {
	Summary   string   `json:"summary"`
	ToolName  string   `json:"tool_name"`
	Files     []string `json:"files"`
	Truncated bool     `json:"truncated"`
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
	a.service, a.openErr = app.Open(ctx, app.Config{DataDir: dataDir, ResolveProvider: a.resolveProvider})
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
	return service.ListConversationRoots(a.ctx)
}

func (a *App) GetConversation(conversationID string) (ConversationView, error) {
	service, err := a.core()
	if err != nil {
		return ConversationView{}, err
	}
	conversation, err := service.GetTaskSnapshot(a.ctx, conversationID)
	if err != nil {
		return ConversationView{}, err
	}
	messages, err := service.ListConversationMessages(a.ctx, conversationID)
	if err != nil {
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

// SendMessage creates a scoped task from a user message, then lets the Agent
// generate and execute its read-only plan. The task/event store remains the
// durable conversation record behind the desktop chat surface.
func (a *App) StartConversation(workspaceID, message, deploymentID string) (ConversationView, error) {
	service, err := a.core()
	if err != nil {
		return ConversationView{}, err
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return ConversationView{}, contracts.NewError(contracts.ErrInvalidInput, "message is required")
	}
	created, _, err := service.CreateTask(a.ctx, app.CreateTaskInput{WorkspaceID: workspaceID, Title: compactTitle(message), Goal: message, AcceptanceCriteria: []contracts.AcceptanceCriterion{{ID: "recon_report", Type: contracts.AcceptanceEvidenceExists, Description: "deterministic workspace reconnaissance persisted", Spec: map[string]interface{}{"kind": "RECON_REPORT"}}}})
	if err != nil {
		return ConversationView{}, err
	}
	if err = service.SaveConversationMessage(a.ctx, contracts.ConversationMessage{ConversationID: created.ID, TurnTaskID: created.ID, Role: "user", Content: message}); err != nil {
		return ConversationView{}, err
	}
	return a.executeConversationTurn(service, created, created.ID, deploymentID)
}

func (a *App) SendConversationMessage(conversationID, message, deploymentID string) (ConversationView, error) {
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
		return ConversationView{}, err
	}
	created, _, err := service.CreateTask(a.ctx, app.CreateTaskInput{WorkspaceID: conversation.Task.WorkspaceID, Title: compactTitle(message), Goal: message, AcceptanceCriteria: []contracts.AcceptanceCriterion{{ID: "recon_report", Type: contracts.AcceptanceEvidenceExists, Description: "deterministic workspace reconnaissance persisted", Spec: map[string]interface{}{"kind": "RECON_REPORT"}}}})
	if err != nil {
		return ConversationView{}, err
	}
	created.ConversationID = conversationID
	if err = service.SetConversationID(a.ctx, created.ID, conversationID); err != nil {
		return ConversationView{}, err
	}
	if err = service.SaveConversationMessage(a.ctx, contracts.ConversationMessage{ConversationID: conversationID, TurnTaskID: created.ID, Role: "user", Content: message}); err != nil {
		return ConversationView{}, err
	}
	return a.executeConversationTurn(service, created, conversationID, deploymentID)
}

func (a *App) executeConversationTurn(service *app.Service, created contracts.Task, conversationID, deploymentID string) (ConversationView, error) {
	snapshot, err := service.RunTask(a.ctx, created.ID)
	if err != nil {
		return ConversationView{}, err
	}
	response := conversationResponse(snapshot)
	if err = service.SaveConversationMessage(a.ctx, contracts.ConversationMessage{ConversationID: conversationID, TurnTaskID: created.ID, Role: "assistant", Content: response}); err != nil {
		return ConversationView{}, err
	}
	return a.GetConversation(conversationID)
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
	content, err := service.ReadTaskArtifact(context.Background(), taskID, "RECON_REPORT")
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

func desktopDataDir() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "SimplenessAgent"), nil
}
