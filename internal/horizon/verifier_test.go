package horizon

import (
	"context"
	"strings"
	"testing"

	"github.com/xm/simplenessagent/pkg/contracts"
)

func TestStageVerifierAcceptsThinkingNoiseAndCommonAliases(t *testing.T) {
	response := `<think>weighing the evidence…</think>
` + "```json" + `
{"summary":"stage evidence reviewed","gate":true,"refs":["evd_1"],"risks":[],"check":"none","confidence":0.9,"verdict":"looks good"}
` + "```"
	opinion, err := decodeStageOpinion(response)
	if err != nil {
		t.Fatal(err)
	}
	if !opinion.GateAppearsMet || len(opinion.EvidenceRefs) != 1 || opinion.EvidenceRefs[0] != "evd_1" || opinion.RecommendedCheck != "none" {
		t.Fatalf("aliases and extra fields were not tolerated: %#v", opinion)
	}
}

func TestStageVerifierRecoversViaShapeExampleRepair(t *testing.T) {
	provider := &plannerScriptProvider{responses: []string{
		`{"gate_appears_met":true}`,
		`{"summary":"recovered","gate_appears_met":true,"evidence_refs":[],"risks":[],"recommended_check":"none"}`,
	}}
	verifier, err := NewStageVerifier(provider)
	if err != nil {
		t.Fatal(err)
	}
	profile := contracts.ModelRoleProfile{Role: contracts.ModelRoleVerifier, MaxOutputTokens: 1024, Temperature: 0}
	opinion, _, err := verifier.Assess(context.Background(), StageVerificationInput{DeploymentID: "dep", Goal: "goal", Stage: contracts.HorizonStage{ID: contracts.HorizonStageDiscover}, Profile: profile})
	if err != nil || opinion.Summary != "recovered" {
		t.Fatalf("shape-example repair did not recover: opinion=%#v err=%v", opinion, err)
	}
	repair := provider.requests[1].Messages[len(provider.requests[1].Messages)-1].Content
	if !strings.Contains(repair, "summary is required") || !strings.Contains(repair, "Exact shape example") {
		t.Fatalf("repair prompt lost the actionable error or shape example: %s", repair)
	}
}

func TestStageVerifierRejectsEmptyResponseWithDiagnosis(t *testing.T) {
	provider := &plannerScriptProvider{responses: []string{"", ""}}
	verifier, _ := NewStageVerifier(provider)
	profile := contracts.ModelRoleProfile{Role: contracts.ModelRoleVerifier, MaxOutputTokens: 1024}
	_, _, err := verifier.Assess(context.Background(), StageVerificationInput{DeploymentID: "dep", Goal: "goal", Stage: contracts.HorizonStage{ID: contracts.HorizonStageDiscover}, Profile: profile})
	if err == nil || !strings.Contains(err.Error(), "verifier response is empty") {
		t.Fatalf("empty verifier response lost its diagnosis: %v", err)
	}
}
