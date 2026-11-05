# Settlement Agent Platform: Setup & Operating Guide

This guide provides end-to-end instructions for configuring, running, and interacting with the **Payment Settlement Investigation Agent**. 

> **Primary Operating Mode:** The system is operated directly from the **React Operator Workstation UI** (`http://localhost:8080/`). A comprehensive [cURL Command Reference](#10-reference-headless-api--curl-cheat-sheet) is provided at the end for automated scripting, CI/CD, and headless verification.

---

## 1. Prerequisites

Ensure you have the following installed on your machine:
* **Go**: Version 1.22+ or 1.23+ (`go version`)
* **Docker & Docker Compose**: For running PostgreSQL 16 (`docker compose version`)
* **curl & jq**: For headless testing and health checks (optional)

---

## 2. PostgreSQL Database Setup

The backend utilizes PostgreSQL 16 with two isolated databases:
* `settlement`: The main runtime database.
* `settlement_test`: An isolated test database for running integration tests.

### Start the Database Container
Run from the repository root:
```bash
docker compose up -d
```

### Verify Container Health
```bash
docker ps --filter "name=settlement-db"
```
The container runs an automated health check. On first boot, `backend/init-test-db.sh` automatically provisions the `settlement_test` database.

### Schema Migrations
The Go API server automatically executes Goose migrations from `backend/migrations/001_init.sql` on startup, creating all 15 relational tables, indexes, and foreign key constraints. No manual migration command is needed.

---

## 3. Configuration & Environment (`.env`)

Copy `.env.example` to `.env` in the repository root (or in `backend/.env`):
```bash
cp .env.example .env
```

### Key Environment Variables

| Variable | Description | Default / Example |
| :--- | :--- | :--- |
| `DATABASE_URL` | Main PostgreSQL connection string | `postgres://postgres:postgres@localhost:5432/settlement?sslmode=disable` |
| `TEST_DATABASE_URL` | Integration test DB connection string | `postgres://postgres:postgres@localhost:5432/settlement_test?sslmode=disable` |
| `HTTP_ADDR` | Listening port for the API server & UI | `:8080` |
| `SEED_DIR` | Directory holding seed CSV files | `backend/seed` |
| `LLM_MODE` | Provider mode: `fake`, `openai`, `gemini`, or `anthropic` | `fake` |
| `LLM_MODEL` | Model identifier string | `gemini-1.5-pro`, `gpt-4o`, `claude-3-5-sonnet-20241022` |
| `SETTLEMENT_WINDOW_DAYS` | Deposit matching candidate window (days) | `2` |
| `OPERATOR_TOKEN` | Bearer token for Operator role | `test-operator-token` |
| `VIEWER_TOKEN` | Bearer token for Viewer role (read-only) | `test-viewer-token` |
| `OPERATOR_NAME` | Actor recorded in regulatory audit trail | `Alice` |

---

## 4. AI & LLM Provider Configuration

The investigation agent features a pluggable model interface:

### Option A: Deterministic Offline (`LLM_MODE=fake`) — Recommended for Evaluations
* **Zero API keys, zero rate-limit quotas, and zero cost.**
* Deterministically exercises all 4 golden reconciliation scenarios and produces realistic multi-turn tool calling traces.
* Ideal for evaluator demonstrations, offline hackathon presentations, and CI tests.

### Option B: Google Gemini (`LLM_MODE=openai` with Google Endpoint)
```bash
LLM_MODE=openai
GEMINI_API_KEY=AIzaSy...YOUR-GEMINI-KEY...
OPENAI_BASE_URL=https://generativelanguage.googleapis.com/v1beta/openai
LLM_MODEL=gemini-1.5-pro
```
*(Or use `gemini-1.5-flash` or `gemini-2.0-flash-exp`).*

### Option C: Anthropic Claude (`LLM_MODE=anthropic`)
```bash
LLM_MODE=anthropic
ANTHROPIC_API_KEY=sk-ant-api03-...YOUR-KEY...
LLM_MODEL=claude-3-5-sonnet-20241022
```

### Option D: OpenAI (`LLM_MODE=openai`)
```bash
LLM_MODE=openai
OPENAI_API_KEY=sk-proj-...YOUR-KEY...
LLM_MODEL=gpt-4o
```

---

## 5. Starting the Platform

The Go API server has the React Workstation built-in; starting the backend automatically serves the full frontend UI.

### Step 5.1: Build and Run the Server
```bash
cd backend
go build -o bin/api ./cmd/api
./bin/api
```
On boot, the server:
1. Connects to PostgreSQL and applies all 15 Goose migrations.
2. Performs cleanups on stale workflow executions.
3. Automatically mounts and serves the static workstation bundle from `frontend/dist/`.
4. Starts the HTTP server on port `:8080`.

### Step 5.2: Verify Health
In a new terminal window:
```bash
curl -s http://localhost:8080/healthz | jq
```
Expected output:
```json
{
  "db": "connected",
  "goose_version": 1,
  "status": "ok",
  "timestamp": "..."
}
```

---

## 6. Primary Interactive UI Walkthrough

Open your browser and navigate to:
👉 **`http://localhost:8080/`**

### Step 1: Ingest Initial Seed Data
1. Look at the top navigation bar.
2. Click the **"Load Seed Data"** button.
3. The backend imports the 7 CSV files from `backend/seed/` (51 orders, 51 captured payments, fee rate tables, bank statements, and settlement manifests).
4. A notification confirms successful ingestion.

### Step 2: Run Reconciliation Engine
1. In the top navigation bar, ensure the **As-Of Date** is set to `2026-10-20`.
2. Click the **"Reconcile"** button.
3. The deterministic recon engine runs the 3-way matching formula ($B - E = \text{Variance}$) across all 6 batches:
   * **Total Payment Volume:** ₹6,90,000.00
   * **Reconciliation Health:** 2/6 Batches Matched (33%) — Batches A and B matched automatically.
   * **Net Variance:** -₹1,02,800.00 across 4 detected exceptions.

### Step 3: Explore the Exceptions Queue
The **Reconciliation Exceptions** table displays the 4 discrepancy cases:

| Row | Batch Reference | Exception Type | Variance | Reason Code | Status |
| :---: | :--- | :--- | :---: | :--- | :---: |
| **#1** | `STL-20261012-F` | `FEE_VARIANCE` | -₹1,000.00 | `GATEWAY_FEE_MISMATCH` | `OPEN` |
| **#2** | `STL-20261013-S` | `DELAYED_OR_SPLIT_DEPOSIT` | -₹38,000.00 | `PARTIAL_DEPOSIT_RECEIVED` | `OPEN` |
| **#3** | `STL-20261014-R` | `REFUND_NOT_IN_LEDGER` | -₹5,000.00 | `REFUND_OMITTED_FROM_LEDGER` | `OPEN` |
| **#4** | `STL-20261015-M` | `MISSING_DEPOSIT` | -₹58,800.00 | `DEPOSIT_NOT_RECEIVED` | `OPEN` |

---

### Step 4: Launch an AI Agent Investigation
1. On **Row #2 (`STL-20261013-S`)**, click the **"Investigate"** button.
2. The button displays a live spinner while the bounded multi-turn agent executes:
   * **Turn 1:** Calls `search_adjacent_ledger_entries(target: "bank_deposits")` to search bank records.
   * **Database:** Finds the split deposit tranche `TXN-S02` for **₹38,000.00** posted on Oct 19 with narration `"PAYOUT PART 2"`.
   * **Turn 2:** Verifies the amount matches the -₹38,000 variance and calls `draft_remediation_entry`.
   * **Turn 3:** Calls `submit_conclusion(proposal_ready)`.
3. The status badge transitions from `OPEN` $\to$ `PROPOSAL READY`.

---

### Step 5: Inspect the Split-View Workspace
Click the **"Review Draft"** (or **"Inspect"**) button on Row #2 to open the slide-out drawer:

* **Left Pane (Deterministic Formula Breakdown):**
  * Displays $B - E = \text{Variance}$ with color-coded discrepancy amounts.
  * Shows Component Table: Gross Payments, Gateway Fees, Deducted Refunds, and Net Settlement.
  * Highlights Candidate Bank Window (`2026-10-14` to `2026-10-16`).
* **Right Pane (Agent Execution Trace):**
  * Displays turn-by-turn tool steps (`search_adjacent_ledger_entries`, `draft_remediation_entry`, `submit_conclusion`).
  * Shows execution latencies (e.g., `4ms`), input parameters, and structured outputs.
  * Displays the authoritative **Agent Summary** card citing verified evidence tokens (`CTX:BATCH`, `TXN-BNK-002`).

---

### Step 6: Human-In-The-Loop (HITL) Review & Approval
Scroll to the bottom of the drawer to view the **Human-In-The-Loop Remediation Card**:
1. Review the proposed policy action: `SETTLEMENT_LINK` for deposit `TXN-BNK-002` (₹38,000.00).
2. Note the generated SHA-256 idempotency key (`rem_link_...`).
3. Click **"Approve & Execute"** as Operator Alice:
   * The backend links the unattached deposit to the batch in PostgreSQL.
   * Status updates to **`RESOLVED`**.
   * A notification confirms the remediation was successfully recorded and executed.

---

### Step 7: Verify Batch Matching & KPI Update
1. Close the drawer and return to the main dashboard.
2. In the top header, click **"Reconcile"** again.
3. Observe the immediate reconciliation results:
   * **Batch S (`STL-20261013-S`)** is now **`MATCHED`** ($V = ₹0.00$).
   * **Reconciliation Health** updates from 2/6 (33%) $\to$ **3/6 Batches Matched (50%)**.
   * **Bank Received** updates from ₹5,74,750.00 $\to$ **₹6,12,750.00** (+₹38,000.00).

---

### Step 8: Investigate Remaining Scenarios

Repeat the same flow for the other scenarios to demonstrate full autonomous coverage:

* **Batch F (`STL-20261012-F` - Fee Variance):**
  * Agent calls `inspect_gateway_config`, detects that gateway applied 2.4% rate instead of contract 2.0% rate.
  * Agent drafts a `FEE_INQUIRY` dispute ticket (`TKT-GW-9907`).
  * Approving moves status to **`ACTION TAKEN`** (cash pending refund from gateway).
* **Batch R (`STL-20261014-R` - Missing Refund):**
  * Agent pinpoints refund `GWREF-R1` omitted from merchant ledger.
  * Agent drafts a `JOURNAL_ENTRY_DRAFT` adjustment (`JE-2026-9907`).
  * Approving moves status to **`ACTION TAKEN`**.
* **Batch M (`STL-20261015-M` - Missing Overdue Deposit):**
  * Agent searches bank records within the settlement window, finds zero cash received.
  * Agent concludes `INSUFFICIENT_EVIDENCE` and escalates to human treasury ops (**`ESCALATED`**).

---

### Step 9: Audit Trail Inspection
1. In the top header, click the **"Audit Log"** button.
2. The drawer displays an append-only timeline containing:
   * Every Reconciliation Run (`recon_run`)
   * Every Agent Investigation (`agent_investigation`)
   * Every Operator Approval & Execution (`remediation_approved`, `remediation_executed`)
   * Exact UTC timestamps, actor identities (`operator:Alice`), and JSON diff payloads.

---

### Step 10: Role-Based Access Control & Chaos Controls
* **Role-Based Access Control (RBAC):**
  * In the top-right header, toggle the role from **Operator (Alice)** to **Viewer (Auditor)**.
  * Notice that all mutative buttons (**"Investigate"**, **"Approve & Execute"**, **"Reconcile"**, **"Load Seed Data"**) are disabled with read-only tooltips.
* **Chaos Engineering & Resilience:**
  * In the **Dev & Simulation Controls** bar, change mock failure mode to **"HTTP 500 Error"**.
  * Attempt executing a remediation $\to$ the downstream mock API fails with HTTP 500.
  * The UI surfaces a red alert and enables the **"Retry Execution"** button.
  * Reset to **"Normal (None)"** and click Retry $\to$ execution succeeds cleanly with idempotency protections.

---

## 7. Rebuilding Frontend Assets (Optional)

The pre-compiled React workstation assets are already bundled in `frontend/dist/`. If you modify any frontend code in `frontend/src/`, re-bundle using Docker:
```bash
docker run --rm -v $(pwd)/frontend:/app -w /app node:20-alpine npm run build
```
*(Or locally if Node.js 20+ is installed: `cd frontend && npm install && npm run build`).*

---

## 8. Running the Automated Test Suite

Run the full suite of unit, integration, and golden scenario tests against `settlement_test`:
```bash
cd backend
TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/settlement_test?sslmode=disable" \
go test -p 1 -v -count=1 ./...
```
*Note: Always pass `-p 1` so tests against the shared test database run serially and prevent table truncation race conditions.*

---

## 9. Resetting the Environment

To perform a clean rerun of the demo at any time:
1. Click **"Load Seed Data"** in the UI header (or run `POST /api/imports/seed`).
2. Click **"Reconcile"**.
3. All exceptions return to their initial `OPEN` state.

---

## 10. Reference: Headless API & cURL Cheat Sheet

For automated pipelines, integration testing, and headless scripting, all UI operations can be executed directly via the REST API:

### 10.1. Ingest Seed CSVs
```bash
curl -s -X POST \
  -H "Authorization: Bearer test-operator-token" \
  http://localhost:8080/api/imports/seed | jq
```

### 10.2. Run Reconciliation Engine
```bash
curl -s -X POST \
  -H "Authorization: Bearer test-operator-token" \
  "http://localhost:8080/api/settlement-runs?as_of=2026-10-20" | jq
```

### 10.3. Query Dashboard Summary & KPI Metrics
```bash
curl -s -H "Authorization: Bearer test-operator-token" \
  http://localhost:8080/api/summary | jq
```

### 10.4. List Reconciliation Exceptions
```bash
curl -s -H "Authorization: Bearer test-operator-token" \
  http://localhost:8080/api/exceptions | jq
```

### 10.5. Trigger Synchronous AI Investigation
```bash
# Launch investigation on Exception #1
curl -s -X POST \
  -H "Authorization: Bearer test-operator-token" \
  "http://localhost:8080/api/exceptions/1/investigate?sync=true" | jq
```

### 10.6. Retrieve Investigation Workflow Execution Trace
```bash
curl -s -H "Authorization: Bearer test-operator-token" \
  http://localhost:8080/api/workflows/1 | jq
```

### 10.7. Operator Remediation Approval & Execution
```bash
# Approve drafted remediation #1
curl -s -X POST \
  -H "Authorization: Bearer test-operator-token" \
  http://localhost:8080/api/remediations/1/approve | jq

# Execute approved remediation #1
curl -s -X POST \
  -H "Authorization: Bearer test-operator-token" \
  http://localhost:8080/api/remediations/1/execute | jq
```

### 10.8. Fetch Append-Only Audit Trail Ledger
```bash
curl -s -H "Authorization: Bearer test-operator-token" \
  http://localhost:8080/api/audit-events | jq
```

### 10.9. Verify RBAC Forbidden Enforcement (Viewer Token)
```bash
# Attempting a mutation with the viewer token returns 403 Forbidden
curl -s -X POST \
  -H "Authorization: Bearer test-viewer-token" \
  http://localhost:8080/api/remediations/1/approve | jq
```
Expected output:
```json
{
  "error": "Forbidden: operator role required"
}
```

### 10.10. Inject Mock Service Failures (Chaos Testing)
```bash
# Inject HTTP 500 error into downstream gateway mock
curl -s -X POST http://localhost:8080/mock/_admin/fail \
  -H "Content-Type: application/json" \
  -d '{"mode":"error_500"}' | jq

# Reset downstream mock to normal
curl -s -X POST http://localhost:8080/mock/_admin/fail \
  -H "Content-Type: application/json" \
  -d '{"mode":"none"}' | jq
```
