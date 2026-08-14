package horizon

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/xm/simplenessagent/pkg/contracts"
)

const stageVerifierContract = `You are the SimplenessAgent Verifier. Review only the supplied persisted evidence for the current stage. Return one JSON object with summary, gate_appears_met, evidence_refs, risks, and recommended_check. Never claim the task is complete, never request a tool, and never invent evidence. Your opinion is advisory; deterministic acceptance owns all state transitions.`

type StageVerifier struct{ provider contracts.ChatProvider }

type StageVerificationInput struct {
	DeploymentID string
	Goal         string
	Stage        contracts.HorizonStage
	Steps        []contracts.StepRuntime
	Evidence     []contracts.Evidence
	Profile      contracts.ModelRoleProfile
}

func NewStageVerifier(provider contracts.ChatProvider) (*StageVerifier, error) {
	if provider == nil {
		return nil, contracts.NewError(contracts.ErrInvalidInput, "stage verifier provider is required")
	}
	return &StageVerifier{provider: provider}, nil
}

func (v *StageVerifier) Assess(ctx context.Context, input StageVerificationInput) (contracts.StageVerificationOpinion, contracts.TokenUsage, error) {
	if input.DeploymentID == "" || input.Stage.ID == "" || input.Profile.Role != contracts.ModelRoleVerifier || input.Profile.MaxOutputTokens <= 0 {
		return contracts.StageVerificationOpinion{}, contracts.TokenUsage{}, contracts.NewError(contracts.ErrInvalidInput, "bounded verifier input and profile are required")
	}
	payload, err := json.Marshal(struct {
		Goal     string                  `json:"goal"`
		Stage    contracts.HorizonStage  `json:"stage"`
		Steps    []contracts.StepRuntime `json:"steps"`
		Evidence []contracts.Evidence    `json:"evidence"`
	}{Goal: input.Goal, Stage: input.Stage, Steps: input.Steps, Evidence: input.Evidence})
	if err != nil {
		return contracts.StageVerificationOpinion{}, contracts.TokenUsage{}, err
	}
	messages := []contracts.Message{{Role: "system", Content: stageVerifierContract}, {Role: "user", Content: string(payload)}}
	usage := contracts.TokenUsage{}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		temperature := input.Profile.Temperature
		response, callErr := v.provider.Chat(ctx, contracts.ChatRequest{DeploymentID: input.DeploymentID, Messages: messages, JSONMode: true, MaxOutputTokens: input.Profile.MaxOutputTokens, Temperature: &temperature})
		if callErr != nil {
			return contracts.StageVerificationOpinion{}, usage, callErr
		}
		usage.InputTokens += response.Usage.InputTokens
		usage.OutputTokens += response.Usage.OutputTokens
		opinion, decodeErr := decodeStageOpinion(response.Text)
		if decodeErr == nil {
			return opinion, usage, nil
		}
		lastErr = decodeErr
		messages = append(messages, contracts.Message{Role: "assistant", Content: response.Text}, contracts.Message{Role: "user", Content: "The verifier JSON was rejected: " + decodeErr.Error() + ". Return one corrected JSON object only."})
	}
	return contracts.StageVerificationOpinion{}, usage, contracts.NewError(contracts.ErrInvalidResponse, "verifier failed after one format repair: "+lastErr.Error())
}

func decodeStageOpinion(text string) (contracts.StageVerificationOpinion, error) {
	text = strings.TrimSpace(text)
	if start, end := strings.IndexByte(text, '{'), strings.LastIndexByte(text, '}'); start >= 0 && end >= start {
		text = text[start : end+1]
	}
	decoder := json.NewDecoder(bytes.NewBufferString(text))
	decoder.DisallowUnknownFields()
	var opinion contracts.StageVerificationOpinion
	if err := decoder.Decode(&opinion); err != nil {
		return opinion, contracts.NewError(contracts.ErrInvalidResponse, "response must match StageVerificationOpinion")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return opinion, contracts.NewError(contracts.ErrInvalidResponse, "verifier response contains trailing values")
	}
	if strings.TrimSpace(opinion.Summary) == "" {
		return opinion, contracts.NewError(contracts.ErrInvalidResponse, "verifier summary is required")
	}
	return opinion, nil
}
