package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xm/simplenessagent/pkg/contracts"
)

func TestGoalRequiresWorkspaceAction(t *testing.T) {
	tests := []struct {
		goal string
		want bool
	}{
		{goal: "帮我做浏览器网页版的一个我的世界", want: true},
		{goal: "把 note 改成 after", want: true},
		{goal: "build a browser game", want: true},
		{goal: "run the project tests", want: true},
		{goal: "请查看工作目录并告诉我有什么", want: false},
		{goal: "如何创建一个浏览器游戏", want: false},
		{goal: "show me how to build a browser game", want: false},
	}
	for _, test := range tests {
		if got := goalRequiresWorkspaceAction(test.goal); got != test.want {
			t.Errorf("goalRequiresWorkspaceAction(%q) = %v, want %v", test.goal, got, test.want)
		}
	}
}

func TestActionGoalCannotCompleteAfterReadOnlyRecon(t *testing.T) {
	provider := &recordingPermissionProvider{responses: []contracts.ChatResponse{
		{ToolCalls: []contracts.ToolCall{{ID: "call-list", Name: "list_files", ArgumentsJSON: `{"path":".","limit":20}`}}},
		{Text: "Only reconnaissance was completed."},
	}}
	ctx := context.Background()
	service, err := Open(ctx, Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) { return provider, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspace, err := service.CreateWorkspace(ctx, "demo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(ctx, contracts.Deployment{Name: "model", ProviderType: "openai_compatible", Location: "TEST", Endpoint: "http://127.0.0.1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := service.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, Title: "browser game", Goal: "帮我做浏览器网页版的一个我的世界", PermissionMode: contracts.PermissionModeEdit, AllowWriteProposals: true, AcceptanceCriteria: agentAcceptance()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.RunModelStep(ctx, RunModelStepInput{TaskID: created.ID, DeploymentID: deployment.ID})
	var domain *contracts.Error
	if !errors.As(err, &domain) || domain.Code != contracts.ErrPlanInvalid || !strings.Contains(domain.Message, "long-horizon mode") {
		t.Fatalf("read-only action outcome was not rejected: %v", err)
	}
	snapshot, err := service.GetTaskSnapshot(ctx, created.ID)
	if err != nil || snapshot.Task.Status != contracts.TaskFailed || snapshot.Steps[0].Status != contracts.StepFailed {
		t.Fatalf("rejected action goal did not persist failure: snapshot=%#v err=%v", snapshot, err)
	}
}
