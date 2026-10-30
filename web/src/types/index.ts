export type UserRole = 'operator' | 'viewer';

export interface Summary {
  payment_volume_minor: number;
  expected_net_minor: number;
  bank_received_minor: number;
  net_variance_minor: number;
  batches_total: number;
  batches_matched: number;
  open_exceptions: number;
  as_of: string;
}

export type ExceptionType =
  | 'FEE_VARIANCE'
  | 'DELAYED_OR_SPLIT_DEPOSIT'
  | 'REFUND_NOT_IN_LEDGER'
  | 'MISSING_DEPOSIT'
  | 'UNEXPLAINED_VARIANCE';

export type ExceptionSeverity = 'low' | 'medium' | 'high';

export type ExceptionStatus =
  | 'open'
  | 'investigating'
  | 'proposal_ready'
  | 'action_taken'
  | 'escalated'
  | 'resolved';

export interface ExceptionContext {
  batch: {
    id: number;
    batch_ref: string;
    settled_on: string;
    currency: string;
    evidence_id: string;
  };
  comparison: {
    evidence_id: string;
    gross: { expected_minor: number; reported_minor: number };
    fee: { expected_minor: number; reported_minor: number };
    refund: { expected_minor: number; reported_minor: number };
    net: { expected_minor: number; reported_minor: number; bank_minor: number };
    variance_minor: number;
  };
  dates: {
    evidence_id: string;
    window_start: string;
    window_end: string;
    as_of: string;
  };
  payment_methods?: Array<{
    method: string;
    count: number;
    amount_minor: number;
  }>;
  linked_deposits?: Array<{
    id: number;
    bank_txn_id: string;
    narration: string;
    amount_minor: number;
    posted_on: string;
    evidence_id: string;
  }>;
  unrecorded_refunds?: Array<{
    id: number;
    gateway_refund_id: string;
    amount_minor: number;
    refunded_at: string;
    evidence_id: string;
  }>;
}

export interface Remediation {
  id: number;
  exception_id: number;
  run_id: number;
  action_type: 'fee_inquiry' | 'settlement_link' | 'journal_entry_draft' | 'write_off';
  payload: Record<string, any>;
  rationale: string;
  evidence_ids: string[];
  status: 'proposed' | 'approved' | 'rejected' | 'executing' | 'executed' | 'execution_failed';
  idempotency_key: string;
  decided_by?: string;
  decided_at?: string;
  decision_note?: string;
  external_ref?: string;
  executed_at?: string;
  last_error?: string;
  created_at: string;
}

export interface Exception {
  id: number;
  batch_id: number;
  batch_ref: string;
  type: ExceptionType;
  reason_code: string;
  severity: ExceptionSeverity;
  status: ExceptionStatus;
  variance_minor: number;
  opened_at: string;
  updated_at: string;
  last_as_of: string;
  context?: ExceptionContext;
  latest_workflow_id?: number;
  remediation?: Remediation;
}

export interface SettlementBatch {
  id: number;
  batch_ref: string;
  settled_on: string;
  reported_gross_minor: number;
  reported_fee_minor: number;
  reported_refund_minor: number;
  reported_net_minor: number;
  expected_net_minor: number;
  bank_received_minor: number;
  variance_minor: number;
  status: 'matched' | 'has_exceptions' | 'pending';
}

export interface WorkflowStep {
  id: number;
  seq: number;
  kind: 'tool_call' | 'decision';
  tool_name: string;
  args: Record<string, any>;
  result: Record<string, any>;
  summary: string;
  status: 'ok' | 'error';
  error?: string;
  duration_ms: number;
  evidence_ids: string[];
  created_at: string;
}

export interface WorkflowConclusion {
  conclusion: 'proposal_ready' | 'needs_escalation' | 'insufficient_evidence';
  summary: string;
  cited_evidence_ids: string[];
  terminal: boolean;
}

export interface WorkflowRun {
  id: number;
  exception_id: number;
  attempt: number;
  status: 'running' | 'completed' | 'failed' | 'escalated';
  tool_calls: number;
  llm_turns: number;
  conclusion?: WorkflowConclusion;
  error?: string;
  started_at: string;
  finished_at?: string;
}

export interface WorkflowDetail {
  workflow: WorkflowRun;
  steps: WorkflowStep[];
}

export interface AuditEvent {
  id: number;
  exception_id?: number;
  remediation_id?: number;
  actor: string;
  event_type: string;
  details: Record<string, any>;
  created_at: string;
}

export interface MockFailureMode {
  mode: 'none' | 'error_500' | 'commit_then_500';
}

