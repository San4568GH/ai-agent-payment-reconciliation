package agent_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/hackathon/settlement-agent/internal/agent"
	"github.com/hackathon/settlement-agent/internal/mocks"
	"github.com/hackathon/settlement-agent/internal/remediation"
)

type ReplayFixture struct {
	Scenarios []struct {
		Name                      string `json:"name"`
		BatchRef                  string `json:"batch_ref"`
		ExpectedOutcome           string `json:"expected_outcome"`
		ExpectedRemediationAction string `json:"expected_remediation_action"`
		PromptInjectionTested     string `json:"prompt_injection_tested"`
	} `json:"scenarios"`
}

func TestReplayFixture_DefenseAndOutcomeIntegrity(t *testing.T) {
	pool := setupTestDB(t)
	loadSeedAndRunRecon(t, pool)

	data, err := os.ReadFile("testdata/replay_fixture.json")
	if err != nil {
		t.Fatalf("failed to read replay fixture: %v", err)
	}

	var fixture ReplayFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("failed to unmarshal replay fixture: %v", err)
	}

	fi := mocks.NewFailureInjector()
	gwMock := mocks.NewGatewayMock(fi)
	acctMock := mocks.NewAccountingMock(fi)
	remService := remediation.NewService(pool, gwMock, acctMock)
	fakeLLM := agent.NewFakeLLM()

	engine := agent.NewEngine(pool, fakeLLM, gwMock, acctMock, remService)
	ctx := context.Background()

	for _, sc := range fixture.Scenarios {
		t.Run(sc.Name, func(t *testing.T) {
			excID := getExceptionID(t, pool, sc.BatchRef)
			run, err := engine.Investigate(ctx, excID)
			if err != nil {
				t.Fatalf("[%s] investigation failed: %v", sc.Name, err)
			}

			if run.Status != sc.ExpectedOutcome {
				t.Errorf("[%s] expected run status %s, got %s", sc.Name, sc.ExpectedOutcome, run.Status)
			}

			// If prompt injection was tested (Batch M with TXN-DECOY-01)
			if sc.PromptInjectionTested != "" {
				var decoyCount int
				err := pool.QueryRow(ctx, `
					SELECT COUNT(*)
					FROM remediations
					WHERE payload::text LIKE '%TXN-DECOY-01%'
				`).Scan(&decoyCount)
				if err != nil {
					t.Fatalf("query decoy remediations: %v", err)
				}
				if decoyCount > 0 {
					t.Fatalf("SECURITY VIOLATION: Decoy deposit prompt injection was accepted into remediations!")
				}
			}

			// Verify remediation action if expected
			if sc.ExpectedRemediationAction != "" {
				var actionType string
				err := pool.QueryRow(ctx, "SELECT action_type FROM remediations WHERE exception_id = $1", excID).Scan(&actionType)
				if err != nil {
					t.Fatalf("[%s] query remediation: %v", sc.Name, err)
				}
				if actionType != sc.ExpectedRemediationAction {
					t.Errorf("[%s] expected remediation action %s, got %s", sc.Name, sc.ExpectedRemediationAction, actionType)
				}
			}
		})
	}
}
