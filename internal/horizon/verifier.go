package horizon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xm/simplenessagent/pkg/contracts"
)

const (
	stageVerifierContract = `You are the SimplenessAgent Verifier. Review only the supplied persisted evidence for the current stage. Return one JSON object with summary, gate_appears_met, evidence_refs, risks, and recommended_check. Never claim the task is complete, never request a tool, and never invent evidence. Your opinion is advisory; deterministic acceptance owns all state transitions.`
	stageOpinionExample   = `{"summary":"stage evidence reviewed","gate_appears_met":true,"evidence_refs":["evd_1"],"risks":[],"recommended_check":"none"}`
)

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
		messages = append(messages, contracts.Message{Role: "assistant", Content: response.Text}, contracts.Message{Role: "user", Content: "The verifier JSON was rejected: " + decodeErr.Error() + ". Return one corrected JSON object only, starting with the JSON itself. Use exactly these fields: summary (string), gate_appears_met (boolean), evidence_refs (array of strings), risks (array of strings), recommended_check (string). Extra fields are not allowed. Exact shape example: " + stageOpinionExample})
	}
	return contracts.StageVerificationOpinion{}, usage, contracts.NewError(contracts.ErrInvalidResponse, "verifier failed after one format repair: "+lastErr.Error())
}

func decodeStageOpinion(text string) (contracts.StageVerificationOpinion, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return contracts.StageVerificationOpinion{}, contracts.NewError(contracts.ErrInvalidResponse, "verifier response is empty")
	}
	objects, scanErr := extractJSONObjects(text)
	if len(objects) == 0 {
		if scanErr != nil {
			return contracts.StageVerificationOpinion{}, contracts.NewError(contracts.ErrInvalidResponse, "verifier response contains malformed JSON: "+scanErr.Error())
		}
		return contracts.StageVerificationOpinion{}, contracts.NewError(contracts.ErrInvalidResponse, "verifier response contains no JSON object")
	}
	var lastErr error
	for _, object := range objects {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(object, &fields); err != nil {
			lastErr = contracts.NewError(contracts.ErrInvalidResponse, "verifier response object is not valid JSON: "+err.Error())
			continue
		}
		opinion, err := decodeOpinionFields(fields)
		if err == nil {
			return opinion, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = contracts.NewError(contracts.ErrInvalidResponse, "verifier response contains no object with a summary field")
	}
	return contracts.StageVerificationOpinion{}, lastErr
}

func decodeOpinionFields(fields map[string]json.RawMessage) (contracts.StageVerificationOpinion, error) {
	summaryRaw, ok := firstField(fields, "summary")
	if !ok {
		return contracts.StageVerificationOpinion{}, fmt.Errorf("summary is required")
	}
	var summary string
	if err := json.Unmarshal(summaryRaw, &summary); err != nil || strings.TrimSpace(summary) == "" {
		return contracts.StageVerificationOpinion{}, fmt.Errorf("summary must be a non-empty string")
	}
	opinion := contracts.StageVerificationOpinion{Summary: summary}
	if gateRaw, ok := firstField(fields, "gate_appears_met", "gateAppearsMet", "gate_met", "gate", "passed"); ok {
		var gate bool
		if err := json.Unmarshal(gateRaw, &gate); err != nil {
			return contracts.StageVerificationOpinion{}, fmt.Errorf("gate_appears_met must be a boolean")
		}
		opinion.GateAppearsMet = gate
	}
	if refsRaw, ok := firstField(fields, "evidence_refs", "evidenceRefs", "refs"); ok {
		if err := json.Unmarshal(refsRaw, &opinion.EvidenceRefs); err != nil {
			return contracts.StageVerificationOpinion{}, fmt.Errorf("evidence_refs must be an array of strings")
		}
	}
	if risksRaw, ok := firstField(fields, "risks"); ok {
		if err := json.Unmarshal(risksRaw, &opinion.Risks); err != nil {
			return contracts.StageVerificationOpinion{}, fmt.Errorf("risks must be an array of strings")
		}
	}
	if checkRaw, ok := firstField(fields, "recommended_check", "recommendedCheck", "check"); ok {
		if err := json.Unmarshal(checkRaw, &opinion.RecommendedCheck); err != nil {
			return contracts.StageVerificationOpinion{}, fmt.Errorf("recommended_check must be a string")
		}
	}
	return opinion, nil
}
