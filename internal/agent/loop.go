package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hackathon/settlement-agent/internal/mocks"
	"github.com/hackathon/settlement-agent/internal/remediation"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	MaxTurns     = 8
	MaxToolCalls = 6
	MaxErrors    = 3
	LoopTimeout  = 90 * time.Second
)

type WorkflowRun struct {
	ID          int64           `json:"id"`
	ExceptionID int64           `json:"exception_id"`
	Attempt     int             `json:"attempt"`
	Status      string          `json:"status"`
	ToolCalls   int             `json:"tool_calls"`
	LLMTurns    int             `json:"llm_turns"`
	Conclusion  json.RawMessage `json:"conclusion,omitempty"`
	Error       *string         `json:"error,omitempty"`
	StartedAt   time.Time       `json:"started_at"`
	FinishedAt  *time.Time      `json:"finished_at,omitempty"`
}

type Engine struct {
	pool       *pgxpool.Pool
	llm        LLM
	gwMock     *mocks.GatewayMock
	acctMock   *mocks.AccountingMock
	remService *remediation.Service
}

func NewEngine(
	pool *pgxpool.Pool,
	llm LLM,
	gwMock *mocks.GatewayMock,
	acctMock *mocks.AccountingMock,
	remService *remediation.Service,
) *Engine {
	return &Engine{
		pool:       pool,
		llm:        llm,
		gwMock:     gwMock,
		acctMock:   acctMock,
		remService: remService,
	}
}

// CleanupStaleRuns marks any stale running runs as failed at startup and resets exceptions to open.
func (e *Engine) CleanupStaleRuns(ctx context.Context) error {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Reset exceptions stuck in 'investigating'
	_, err = tx.Exec(ctx, `
		UPDATE exceptions
		SET status = 'open', updated_at = now()
		WHERE status = 'investigating'
	`)
	if err != nil {
		return fmt.Errorf("reset stuck exceptions: %w", err)
	}

	// Mark stale workflow runs as failed
	_, err = tx.Exec(ctx, `
		UPDATE workflow_runs
		SET status = 'failed', error = 'interrupted by server restart', finished_at = now()
		WHERE status = 'running'
	`)
	if err != nil {
		return fmt.Errorf("mark stale workflow_runs: %w", err)
	}

	return tx.Commit(ctx)
}

// Investigate executes the bounded investigation loop for an exception.
func (e *Engine) Investigate(ctx context.Context, exceptionID int64) (*WorkflowRun, error) {
	ctx, cancel := context.WithTimeout(ctx, LoopTimeout)
	defer cancel()

	// 1. Fetch Exception Details
	var excType string
	var batchID int64
	var batchRef string
	var varianceMinor int64
	var contextBytes []byte
	var currentStatus string

	err := e.pool.QueryRow(ctx, `
		SELECT e.type, e.batch_id, sb.batch_ref, e.variance_minor, e.context, e.status
		FROM exceptions e
		JOIN settlement_batches sb ON e.batch_id = sb.id
		WHERE e.id = $1
	`, exceptionID).Scan(&excType, &batchID, &batchRef, &varianceMinor, &contextBytes, &currentStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("exception %d not found", exceptionID)
		}
		return nil, fmt.Errorf("query exception: %w", err)
	}

	// Exception must be open or escalated to investigate
	if currentStatus != "open" && currentStatus != "escalated" {
		return nil, fmt.Errorf("exception %d is not in 'open' or 'escalated' state (current: %s)", exceptionID, currentStatus)
	}

	// 2. Determine Attempt Number & Create Workflow Run
	var nextAttempt int
	err = e.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(attempt), 0) + 1
		FROM workflow_runs
		WHERE exception_id = $1
	`, exceptionID).Scan(&nextAttempt)
	if err != nil {
		return nil, fmt.Errorf("determine next attempt: %w", err)
	}

	var run WorkflowRun
	err = e.pool.QueryRow(ctx, `
		INSERT INTO workflow_runs (exception_id, attempt, status, started_at)
		VALUES ($1, $2, 'running', now())
		RETURNING id, exception_id, attempt, status, tool_calls, llm_turns, started_at
	`, exceptionID, nextAttempt).Scan(
		&run.ID, &run.ExceptionID, &run.Attempt, &run.Status, &run.ToolCalls, &run.LLMTurns, &run.StartedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create workflow run: %w", err)
	}

	// Update exception status to 'investigating'
	_, _ = e.pool.Exec(ctx, "UPDATE exceptions SET status = 'investigating', updated_at = now() WHERE id = $1", exceptionID)

	// 3. Initialize Evidence Registry
	registry := NewEvidenceRegistry()
	registry.Register(batchRef, fmt.Sprintf("EXC-%d", exceptionID))

	// Pre-populate evidence registry with entities present in exception context
	var ctxMap map[string]any
	if err := json.Unmarshal(contextBytes, &ctxMap); err == nil {
		if payments, ok := ctxMap["payments"].([]any); ok {
			for _, p := range payments {
				if pm, ok := p.(map[string]any); ok {
					if payID, ok := pm["gateway_payment_id"].(string); ok {
						registry.Register(payID)
					}
				}
			}
		}
		if refunds, ok := ctxMap["refunds"].([]any); ok {
			for _, r := range refunds {
				if rm, ok := r.(map[string]any); ok {
					if rfdID, ok := rm["gateway_refund_id"].(string); ok {
						registry.Register(rfdID)
					}
				}
			}
		}
		if deposits, ok := ctxMap["candidate_deposits"].([]any); ok {
			for _, d := range deposits {
				if dm, ok := d.(map[string]any); ok {
					if txnID, ok := dm["bank_txn_id"].(string); ok {
						registry.Register(txnID)
					}
				}
			}
		}
	}

	toolRunner := NewToolRunner(e.pool, e.gwMock, e.acctMock, e.remService, registry, exceptionID, run.ID)

	// 4. Initialize Conversation Messages
	userPrompt := BuildInvestigationPrompt(excType, batchRef, varianceMinor, contextBytes)
	messages := []Message{
		{
			Role:    RoleUser,
			Content: userPrompt,
		},
	}
	tools := GetAgentTools()

	// 5. Bounded Loop
	var finalConclusion json.RawMessage
	var runOutcome = "completed"
	var totalToolCalls int
	var totalTurns int
	var consecutiveErrors int
	var terminalReached bool
	stepSeq := 0

	for turn := 1; turn <= MaxTurns && !terminalReached; turn++ {
		totalTurns = turn

		turnReq := &TurnRequest{
			SystemPrompt: SystemPrompt,
			Messages:     messages,
			Tools:        tools,
		}

		turnRes, err := e.llm.GenerateTurn(ctx, turnReq)
		if err != nil {
			consecutiveErrors++
			if consecutiveErrors >= MaxErrors {
				runOutcome = "escalated"
				break
			}
			continue
		}

		if len(turnRes.ToolCalls) == 0 {
			// No tool calls made; if draft succeeded, force conclusion
			if toolRunner.HasDraftSucceeded() {
				terminalReached = true
				break
			}
			break
		}

		// Assistant message added to history (free text discarded from DB trace)
		messages = append(messages, Message{
			Role:      RoleAssistant,
			ToolCalls: turnRes.ToolCalls,
		})

		var toolResults []ToolResult

		for _, tc := range turnRes.ToolCalls {
			totalToolCalls++
			stepSeq++
			startTime := time.Now()

			var stepStatus = "ok"
			var stepError *string

			toolOutput, execErr := toolRunner.ExecuteTool(ctx, tc.Name, tc.InputJSON)
			durationMs := int(time.Since(startTime).Milliseconds())

			var outputJSON []byte
			if execErr != nil {
				consecutiveErrors++
				stepStatus = "error"
				errStr := execErr.Error()
				stepError = &errStr
				outputJSON, _ = json.Marshal(map[string]any{"error": errStr})
				toolResults = append(toolResults, ToolResult{
					ToolUseID: tc.ID,
					Content:   string(outputJSON),
					IsError:   true,
				})
			} else {
				consecutiveErrors = 0
				outputJSON, _ = json.Marshal(toolOutput)
				toolResults = append(toolResults, ToolResult{
					ToolUseID: tc.ID,
					Content:   string(outputJSON),
				})
			}

			// Discard assistant free text and persist ONLY structured step to workflow_steps
			summary := fmt.Sprintf("Executed %s with status %s", tc.Name, stepStatus)
			evidenceIDs := registry.All()
			var argsMap map[string]any
			_ = json.Unmarshal([]byte(tc.InputJSON), &argsMap)

			_, _ = e.pool.Exec(ctx, `
				INSERT INTO workflow_steps (
					run_id, seq, kind, tool_name, args, result, summary, status, error, duration_ms, evidence_ids, created_at
				) VALUES ($1, $2, 'tool_call', $3, $4, $5, $6, $7, $8, $9, $10, now())
			`, run.ID, stepSeq, tc.Name, tc.InputJSON, string(outputJSON), summary, stepStatus, stepError, durationMs, evidenceIDs)

			// Check if submit_conclusion was executed
			if tc.Name == "submit_conclusion" && execErr == nil {
				terminalReached = true
				finalConclusion = outputJSON

				if m, ok := toolOutput.(map[string]any); ok {
					conc := fmt.Sprintf("%v", m["conclusion"])
					if conc == "proposal_ready" {
						runOutcome = "completed"
					} else {
						runOutcome = "escalated"
					}
				}
				break
			}

			// Bound enforcement: Max 6 tool calls reached -> break
			if totalToolCalls >= MaxToolCalls {
				terminalReached = true
				if !toolRunner.HasDraftSucceeded() {
					runOutcome = "escalated"
				}
				break
			}

			// If 3 consecutive errors occurred -> escalate
			if consecutiveErrors >= MaxErrors {
				terminalReached = true
				runOutcome = "escalated"
				break
			}
		}

		messages = append(messages, Message{
			Role:        RoleUser,
			ToolResults: toolResults,
		})
	}

	// 6. Finalize Run Status and Exception State
	if !terminalReached && !toolRunner.HasDraftSucceeded() {
		runOutcome = "escalated"
	}

	now := time.Now().UTC()
	run.Status = runOutcome
	run.ToolCalls = totalToolCalls
	run.LLMTurns = totalTurns
	run.Conclusion = finalConclusion
	run.FinishedAt = &now

	_, err = e.pool.Exec(ctx, `
		UPDATE workflow_runs
		SET status = $1, tool_calls = $2, llm_turns = $3, conclusion = $4, finished_at = $5
		WHERE id = $6
	`, run.Status, run.ToolCalls, run.LLMTurns, run.Conclusion, run.FinishedAt, run.ID)
	if err != nil {
		return nil, fmt.Errorf("finalize workflow run: %w", err)
	}

	// Update Exception status
	var finalExceptionStatus string
	switch runOutcome {
	case "completed":
		finalExceptionStatus = "proposal_ready"
	case "escalated":
		finalExceptionStatus = "escalated"
	default:
		finalExceptionStatus = "open"
	}

	_, _ = e.pool.Exec(ctx, `
		UPDATE exceptions
		SET status = $1, updated_at = now()
		WHERE id = $2
	`, finalExceptionStatus, exceptionID)

	return &run, nil
}

