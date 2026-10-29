-- +goose Up
-- 001_init.sql: Payment Settlement Investigation Agent 15-table schema

-- 1. import_batches
CREATE TABLE IF NOT EXISTS import_batches (
    id BIGSERIAL PRIMARY KEY,
    kind TEXT NOT NULL,
    filename TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    rows_total INT NOT NULL DEFAULT 0,
    inserted INT NOT NULL DEFAULT 0,
    skipped INT NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'completed',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_import_batches_kind_sha256 UNIQUE (kind, sha256)
);

-- 2. orders
CREATE TABLE IF NOT EXISTS orders (
    id BIGSERIAL PRIMARY KEY,
    order_ref TEXT NOT NULL,
    order_ref_norm TEXT NOT NULL,
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    currency TEXT NOT NULL DEFAULT 'INR' CHECK (currency = 'INR'),
    status TEXT NOT NULL,
    ordered_at TIMESTAMPTZ NOT NULL,
    raw JSONB NOT NULL,
    CONSTRAINT uq_orders_order_ref_norm UNIQUE (order_ref_norm)
);

-- 3. order_refunds
CREATE TABLE IF NOT EXISTS order_refunds (
    id BIGSERIAL PRIMARY KEY,
    refund_ref TEXT NOT NULL,
    refund_ref_norm TEXT NOT NULL,
    order_id BIGINT REFERENCES orders(id),
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    refunded_at TIMESTAMPTZ NOT NULL,
    raw JSONB NOT NULL,
    CONSTRAINT uq_order_refunds_refund_ref_norm UNIQUE (refund_ref_norm)
);

-- 4. gateway_payments
CREATE TABLE IF NOT EXISTS gateway_payments (
    id BIGSERIAL PRIMARY KEY,
    gateway_payment_id TEXT NOT NULL,
    order_ref_norm TEXT NOT NULL,
    order_id BIGINT REFERENCES orders(id),
    link_status TEXT NOT NULL DEFAULT 'unlinked' CHECK (link_status IN ('linked', 'unlinked')),
    link_reason TEXT,
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    currency TEXT NOT NULL DEFAULT 'INR' CHECK (currency = 'INR'),
    method TEXT NOT NULL CHECK (method IN ('card', 'upi')),
    status TEXT NOT NULL CHECK (status IN ('captured', 'failed', 'authorized')),
    captured_at TIMESTAMPTZ NOT NULL,
    settlement_ref_norm TEXT NOT NULL,
    raw JSONB NOT NULL,
    CONSTRAINT uq_gateway_payments_gateway_payment_id UNIQUE (gateway_payment_id)
);
CREATE INDEX IF NOT EXISTS idx_gw_payments_settlement_ref_norm ON gateway_payments(settlement_ref_norm);

-- 5. gateway_refunds
CREATE TABLE IF NOT EXISTS gateway_refunds (
    id BIGSERIAL PRIMARY KEY,
    gateway_refund_id TEXT NOT NULL,
    gateway_payment_id TEXT,
    refund_ref_norm TEXT NOT NULL,
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    status TEXT NOT NULL CHECK (status IN ('processed', 'pending', 'failed')),
    refunded_at TIMESTAMPTZ NOT NULL,
    settlement_ref_norm TEXT NOT NULL,
    internal_refund_id BIGINT REFERENCES order_refunds(id),
    raw JSONB NOT NULL,
    CONSTRAINT uq_gateway_refunds_gateway_refund_id UNIQUE (gateway_refund_id)
);

-- 6. settlement_batches
CREATE TABLE IF NOT EXISTS settlement_batches (
    id BIGSERIAL PRIMARY KEY,
    batch_ref TEXT NOT NULL,
    batch_ref_norm TEXT NOT NULL,
    settled_on DATE NOT NULL,
    currency TEXT NOT NULL DEFAULT 'INR' CHECK (currency = 'INR'),
    reported_gross_minor BIGINT NOT NULL,
    reported_fee_minor BIGINT NOT NULL,
    reported_refund_minor BIGINT NOT NULL,
    reported_net_minor BIGINT NOT NULL,
    raw JSONB NOT NULL,
    CONSTRAINT uq_settlement_batches_batch_ref_norm UNIQUE (batch_ref_norm)
);

-- 7. bank_deposits
CREATE TABLE IF NOT EXISTS bank_deposits (
    id BIGSERIAL PRIMARY KEY,
    bank_txn_id TEXT NOT NULL,
    reference TEXT,
    narration TEXT NOT NULL,
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    currency TEXT NOT NULL DEFAULT 'INR' CHECK (currency = 'INR'),
    posted_on DATE NOT NULL,
    raw JSONB NOT NULL,
    CONSTRAINT uq_bank_deposits_bank_txn_id UNIQUE (bank_txn_id)
);
CREATE INDEX IF NOT EXISTS idx_bank_deposits_posted_on ON bank_deposits(posted_on);

-- 8. gateway_fee_schedules
CREATE TABLE IF NOT EXISTS gateway_fee_schedules (
    id BIGSERIAL PRIMARY KEY,
    method TEXT NOT NULL CHECK (method IN ('card', 'upi')),
    rate_bps INT NOT NULL CHECK (rate_bps >= 0),
    fixed_minor BIGINT NOT NULL DEFAULT 0 CHECK (fixed_minor >= 0),
    effective_from DATE NOT NULL,
    effective_to DATE,
    CONSTRAINT uq_fee_schedules_method_effective_from UNIQUE (method, effective_from)
);

-- 9. recon_results
CREATE TABLE IF NOT EXISTS recon_results (
    id BIGSERIAL PRIMARY KEY,
    batch_id BIGINT NOT NULL REFERENCES settlement_batches(id) ON DELETE CASCADE,
    as_of DATE NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('matched', 'pending', 'exception')),
    expected_gross_minor BIGINT NOT NULL,
    expected_fee_minor BIGINT NOT NULL,
    expected_refund_minor BIGINT NOT NULL,
    expected_net_minor BIGINT NOT NULL,
    reported_net_minor BIGINT NOT NULL,
    bank_total_minor BIGINT NOT NULL,
    variance_minor BIGINT NOT NULL,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_recon_results_batch_id UNIQUE (batch_id)
);

-- 10. matches
CREATE TABLE IF NOT EXISTS matches (
    id BIGSERIAL PRIMARY KEY,
    batch_id BIGINT NOT NULL REFERENCES settlement_batches(id) ON DELETE CASCADE,
    deposit_id BIGINT NOT NULL REFERENCES bank_deposits(id),
    applied_amount_minor BIGINT NOT NULL CHECK (applied_amount_minor > 0),
    method TEXT NOT NULL CHECK (method IN ('reference', 'amount_date', 'remediation')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_matches_deposit_id UNIQUE (deposit_id)
);

-- 11. exceptions
CREATE TABLE IF NOT EXISTS exceptions (
    id BIGSERIAL PRIMARY KEY,
    batch_id BIGINT NOT NULL REFERENCES settlement_batches(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('FEE_VARIANCE', 'DELAYED_OR_SPLIT_DEPOSIT', 'REFUND_NOT_IN_LEDGER', 'MISSING_DEPOSIT', 'UNEXPLAINED_VARIANCE', 'UNLINKED_PAYMENT', 'DUPLICATE_RECORD')),
    reason_code TEXT NOT NULL,
    severity TEXT NOT NULL CHECK (severity IN ('low', 'medium', 'high')),
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'investigating', 'proposal_ready', 'escalated', 'action_taken', 'resolved')),
    variance_minor BIGINT NOT NULL,
    context JSONB NOT NULL DEFAULT '{}'::jsonb,
    opened_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_as_of DATE NOT NULL,
    CONSTRAINT uq_exceptions_batch_id_type UNIQUE (batch_id, type)
);

-- 12. workflow_runs
CREATE TABLE IF NOT EXISTS workflow_runs (
    id BIGSERIAL PRIMARY KEY,
    exception_id BIGINT NOT NULL REFERENCES exceptions(id) ON DELETE CASCADE,
    attempt INT NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'completed', 'escalated', 'failed')),
    tool_calls INT NOT NULL DEFAULT 0,
    llm_turns INT NOT NULL DEFAULT 0,
    conclusion JSONB,
    error TEXT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_runs_running ON workflow_runs(exception_id) WHERE status = 'running';

-- 13. workflow_steps
CREATE TABLE IF NOT EXISTS workflow_steps (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES workflow_runs(id) ON DELETE CASCADE,
    seq INT NOT NULL,
    kind TEXT NOT NULL DEFAULT 'tool_call',
    tool_name TEXT NOT NULL,
    args JSONB NOT NULL,
    result JSONB,
    summary TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ok' CHECK (status IN ('ok', 'error')),
    error TEXT,
    duration_ms INT NOT NULL DEFAULT 0,
    evidence_ids TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_workflow_steps_run_id_seq UNIQUE (run_id, seq)
);

-- 14. remediations
CREATE TABLE IF NOT EXISTS remediations (
    id BIGSERIAL PRIMARY KEY,
    exception_id BIGINT NOT NULL REFERENCES exceptions(id) ON DELETE CASCADE,
    run_id BIGINT NOT NULL REFERENCES workflow_runs(id) ON DELETE CASCADE,
    action_type TEXT NOT NULL CHECK (action_type IN ('settlement_link', 'fee_inquiry', 'journal_entry_draft')),
    payload JSONB NOT NULL,
    rationale TEXT NOT NULL,
    evidence_ids TEXT[] NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'proposed' CHECK (status IN ('proposed', 'approved', 'executing', 'executed', 'execution_failed', 'rejected')),
    idempotency_key TEXT NOT NULL,
    decided_by TEXT,
    decided_at TIMESTAMPTZ,
    decision_note TEXT,
    external_ref TEXT,
    executed_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_remediations_idempotency_active ON remediations(idempotency_key) WHERE status <> 'rejected';

-- 15. audit_events
CREATE TABLE IF NOT EXISTS audit_events (
    id BIGSERIAL PRIMARY KEY,
    exception_id BIGINT REFERENCES exceptions(id) ON DELETE SET NULL,
    remediation_id BIGINT REFERENCES remediations(id) ON DELETE SET NULL,
    actor TEXT NOT NULL,
    event_type TEXT NOT NULL,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS audit_events CASCADE;
DROP TABLE IF EXISTS remediations CASCADE;
DROP TABLE IF EXISTS workflow_steps CASCADE;
DROP TABLE IF EXISTS workflow_runs CASCADE;
DROP TABLE IF EXISTS exceptions CASCADE;
DROP TABLE IF EXISTS matches CASCADE;
DROP TABLE IF EXISTS recon_results CASCADE;
DROP TABLE IF EXISTS gateway_fee_schedules CASCADE;
DROP TABLE IF EXISTS bank_deposits CASCADE;
DROP TABLE IF EXISTS settlement_batches CASCADE;
DROP TABLE IF EXISTS gateway_refunds CASCADE;
DROP TABLE IF EXISTS gateway_payments CASCADE;
DROP TABLE IF EXISTS order_refunds CASCADE;
DROP TABLE IF EXISTS orders CASCADE;
DROP TABLE IF EXISTS import_batches CASCADE;
