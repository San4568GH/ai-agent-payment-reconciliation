# Settlement Agent Backend: Setup & Operating Guide

This guide provides end-to-end instructions for configuring, running, and testing the **Payment Settlement Investigation Agent** backend.

---

## 1. Prerequisites

Ensure you have the following installed on your machine:
- **Go**: Version 1.22+ or 1.23+ (`go version`)
- **Docker & Docker Compose**: For running PostgreSQL 16 (`docker compose version`)
- **curl & jq**: For interacting with and formatting API responses

---

## 2. PostgreSQL Database Setup

The backend uses PostgreSQL 16 with two databases:
- `settlement`: The main runtime database.
- `settlement_test`: An isolated test database for running integration tests.

### Start the Database Container
Run from the `settlement-agent` directory:
```bash
docker compose up -d
```

### Verify Container Health
```bash
docker ps --filter "name=settlement-db"
```
The container runs an automated healthcheck. The script `init-test-db.sh` automatically creates `settlement_test` on first boot.

### Schema Migrations
The API automatically applies Goose migrations from `migrations/001_init.sql` on startup (creating all 15 tables, indexes, and constraints). No manual migration command is required.

---

## 3. Configuration & Environment Variables (`.env`)

Copy `.env.example` to `.env` if not already present:
```bash
cp .env.example .env
```

### Environment Variable Reference

| Variable | Description | Example / Default |
| :--- | :--- | :--- |
| `DATABASE_URL` | Main PostgreSQL connection string | `postgres://postgres:postgres@localhost:5432/settlement?sslmode=disable` |
| `TEST_DATABASE_URL` | Integration test DB connection string | `postgres://postgres:postgres@localhost:5432/settlement_test?sslmode=disable` |
| `HTTP_ADDR` | Listening port for the API server | `:8080` |
| `SEED_DIR` | Directory containing seed CSV files | `seed` |
| `OPERATOR_TOKEN` | Bearer token for Operator (read & write) | `test-operator-token` |
| `VIEWER_TOKEN` | Bearer token for Viewer (read-only) | `test-viewer-token` |
| `OPERATOR_NAME` | Name recorded in audit trail logs | `Alice` |
| `SETTLEMENT_WINDOW_DAYS` | Deposit matching window (T to T+N) | `2` |
| `LLM_MODE` | Provider mode: `fake`, `anthropic`, or `openai` | `fake` |
| `ANTHROPIC_API_KEY` | Anthropic API key (when using Claude) | `sk-ant-api03-...` |
| `OPENAI_API_KEY` | OpenAI API key (when using OpenAI models) | `sk-proj-...` |
| `GEMINI_API_KEY` | Google Gemini API key (when using Gemini) | `AIzaSy...` |
| `OPENAI_BASE_URL` | Custom endpoint for Gemini, Groq, Ollama, etc. | `https://api.openai.com/v1` |
| `LLM_MODEL` | Model identifier string | `claude-3-5-sonnet-20241022` or `gpt-4o` |
| `MOCK_BASE_URL` | Base URL for mock external services | `http://localhost:8080/mock` |
| `MOCK_ADMIN` | Enable mock admin endpoints (`1` or `0`) | `1` |

---

## 4. AI & LLM Provider Configuration

The agent backend uses a pluggable `LLM` interface that supports **Anthropic Claude**, **OpenAI**, **Google Gemini**, **Groq**, **OpenRouter**, **local Ollama models**, and an **offline deterministic fake provider**.

---

### Provider 1: Deterministic Offline (`LLM_MODE=fake`) — Recommended for Dev & Demo
- **No API keys or internet connection required.** Zero LLM API cost.
- Matches and handles all 4 golden reconciliation exception scenarios deterministically:
  1. `FEE_VARIANCE` (Batch F): Discrepancy diagnosed $\to$ `fee_inquiry` remediation proposed.
  2. `DELAYED_OR_SPLIT_DEPOSIT` (Batch S): Split deposit found in bank ledger $\to$ `deposit_shortfall` adjustment proposed.
  3. `REFUND_NOT_IN_LEDGER` (Batch R): Missing gateway refund located $\to$ `unmatched_refund` journal entry proposed.
  4. `MISSING_DEPOSIT` (Batch M): Overdue deposit with no bank match $\to$ human escalation triggered.
- Ideal for automated CI/CD and offline demonstrations.

---

### Provider 2: Anthropic Claude (`LLM_MODE=anthropic`)
To use Anthropic Claude (e.g., Claude 3.5 Sonnet):
1. Obtain an API key from [Anthropic Console](https://console.anthropic.com/).
2. In `.env`:
   ```bash
   LLM_MODE=anthropic
   ANTHROPIC_API_KEY=sk-ant-api03-YOUR-KEY-HERE
   LLM_MODEL=claude-3-5-sonnet-20241022
   ```

---

### Provider 3: OpenAI (GPT-4o / GPT-4o-mini)
To use OpenAI:
1. Obtain an API key from [OpenAI Platform](https://platform.openai.com/).
2. In `.env`:
   ```bash
   LLM_MODE=openai
   OPENAI_API_KEY=sk-proj-YOUR-OPENAI-KEY-HERE
   LLM_MODEL=gpt-4o
   ```
   *(Or `LLM_MODEL=gpt-4o-mini` for faster, lower-cost investigations).*

---

### Provider 4: Google Gemini (Gemini 1.5 Pro / Flash, Gemini 2.0)
To use Google Gemini via its OpenAI-compatible endpoint:
1. Obtain a Gemini API key from [Google AI Studio](https://aistudio.google.com/).
2. In `.env`:
   ```bash
   LLM_MODE=openai
   GEMINI_API_KEY=AIzaSyYOUR-GEMINI-KEY-HERE
   OPENAI_BASE_URL=https://generativelanguage.googleapis.com/v1beta/openai
   LLM_MODEL=gemini-1.5-pro
   ```
   *(You can also use `gemini-1.5-flash` or `gemini-2.0-flash-exp`).*

---

### Provider 5: Local Ollama / vLLM / LiteLLM (Zero API Key, 100% Local)
If you prefer running open-source models completely locally:
1. Install [Ollama](https://ollama.com/) and pull a tool-calling model:
   ```bash
   ollama run llama3.1
   # or: ollama run mistral-nemo / qwen2.5
   ```
2. In `.env`:
   ```bash
   LLM_MODE=openai
   OPENAI_BASE_URL=http://localhost:11434/v1
   OPENAI_API_KEY=ollama
   LLM_MODEL=llama3.1
   ```

---

### Provider 6: Groq / OpenRouter / DeepSeek / Together AI
Any provider implementing the standard OpenAI Chat Completions API with tool calling works out-of-the-box:

**For Groq (Ultra-fast inference):**
```bash
LLM_MODE=openai
OPENAI_BASE_URL=https://api.groq.com/openai/v1
OPENAI_API_KEY=gsk_YOUR-GROQ-KEY
LLM_MODEL=llama-3.3-70b-versatile
```

**For OpenRouter:**
```bash
LLM_MODE=openai
OPENAI_BASE_URL=https://openrouter.ai/api/v1
OPENAI_API_KEY=sk-or-v1-YOUR-KEY
LLM_MODEL=anthropic/claude-3.5-sonnet
```

---

## 5. Seed Data Generation & Ingestion

### Step 5.1: Generate Deterministic Seed Files
Run the seed generator to generate the 7 required CSV files in `seed/`:
```bash
go run cmd/seedgen/main.go
```
This generates:
- `orders.csv`
- `gateway_payments.csv`
- `gateway_refunds.csv`
- `gateway_batches.csv`
- `gateway_fee_schedules.csv`
- `bank_deposits.csv`
- `ledger_entries.csv`

### Step 5.2: Ingest Seed Data into Database
With the backend server running (see Section 6), trigger seed ingestion:
```bash
curl -s -X POST \
  -H "Authorization: Bearer test-operator-token" \
  http://localhost:8080/api/imports/seed | jq
```
*Note: The ingest pipeline uses SHA-256 hashing. Ingesting again will safely skip existing files.*

---

## 6. Running the API Backend

### Build the Binary
```bash
go build -o bin/api ./cmd/api
```

### Start the Server
```bash
./bin/api
```
On startup, the server:
1. Connects to PostgreSQL.
2. Runs Goose migrations.
3. Cleans up any stale workflow runs or interrupted remediations.
4. Executes an LLM smoke test.
5. Listens on `http://localhost:8080`.

### Health Check
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

## 7. React Operator Workstation UI (Day 4)

The workstation UI is a single-page React 18 + TypeScript application styled with Mantine UI v7 and powered by TanStack Query v5.

### Accessing the Workstation
The application can be accessed in two ways:
1. **Directly via Go Server (Embedded)**:
   - URL: **`http://localhost:8080/`**
   - The Go Chi API server automatically detects and serves the built production assets from `web/dist/`. No separate web server is required.
2. **Via Standalone Preview / Vite Server**:
   - URL: **`http://localhost:3000/`**
   - Runs with automated API proxy forwarding to `:8080`.

### Building the Frontend Assets (Optional)
The pre-built production assets are already bundled in `web/dist/`. If you modify any frontend code in `web/src/`, re-bundle using Docker:
```bash
docker run --rm -v $(pwd)/web:/app -w /app node:20-alpine npm run build
```
*(Or locally if Node is installed: `cd web && npm install && npm run build`).*

### Workstation Feature Tour
- **Role Toggle (Operator vs. Viewer)**:
  - Located in the top header.
  - **Operator (`operator:Alice`)**: Allowed to load seed data, trigger reconciliations, launch agent investigations, and approve/reject/execute remediations.
  - **Viewer (`viewer:Auditor`)**: Read-only mode. All mutation buttons are disabled with tooltips. Any unauthorized backend calls return `403 Forbidden`.
- **KPI Summary Cards**:
  - Displays Total Payment Volume, Expected Net, Actual Bank Received, Net Variance (color-coded red/green), and Batch Match Health percentage.
- **Exceptions Table**:
  - Filter by exception type (`FEE_VARIANCE`, `DELAYED_OR_SPLIT_DEPOSIT`, `REFUND_NOT_IN_LEDGER`, `MISSING_DEPOSIT`) or status.
  - One-click **"Investigate"** button triggers autonomous agent analysis.
  - **"Inspect / Review Draft"** opens the split-view drawer.
- **Split-View Investigation Workspace**:
  - **Left Pane (Variance Decomposition)**: Displays the deterministic formula ($B - E = \text{Variance}$), Reported vs Expected grid, candidate deposit window ($T \dots T+2$), and traceable evidence registry chips.
  - **Right Pane (Agent Execution Trace)**: Accordion displaying turn-by-turn tool calls (`inspect_gateway_config`, `search_ledger`, `draft_remediation_entry`), execution latencies, input arguments, sanitized tool outputs, and the authoritative **"Agent summary"** conclusion card.
- **Human-In-The-Loop Remediation Card**:
  - Displays proposed policy action, Go-calculated financial payload, and SHA-256 idempotency key.
  - **"Approve & Execute"**: Authorizes the Go backend to execute against external mock services and marks the exception as `action_taken`.
  - **"Reject Proposal"**: Rejects proposal with an operator note.
  - **"Retry Execution"**: Safely retries external execution if a downstream service previously failed.
- **Dev & Simulation Controls Bar**:
  - Test mock downstream failure modes (`none`, `error_500`, `commit_then_500`) to demonstrate retry resilience.
  - Switch between Synchronous (waits 3–4s) and Asynchronous background investigations.
- **Audit Log Drawer**:
  - Click **"Audit Log"** in the header to view the immutable append-only regulatory audit ledger.

---

## 8. Step-by-Step Operational Workflow (CLI & Automated API)

Once the server is running and seed data is ingested:

### Step 1: Run Reconciliation Engine
Run reconciliation as of `2026-10-20`:
```bash
curl -s -X POST \
  -H "Authorization: Bearer test-operator-token" \
  "http://localhost:8080/api/settlement-runs?as_of=2026-10-20" | jq
```

### Step 2: View Dashboard Summary & Exceptions
Check summary metrics:
```bash
curl -s -H "Authorization: Bearer test-operator-token" \
  http://localhost:8080/api/summary | jq
```

List all 4 detected exceptions:
```bash
curl -s -H "Authorization: Bearer test-operator-token" \
  http://localhost:8080/api/exceptions | jq
```
Returns:
- Exception #1: `FEE_VARIANCE` on batch `STL-20261012-F` (−₹1,000.00).
- Exception #2: `DELAYED_OR_SPLIT_DEPOSIT` on batch `STL-20261013-S` (−₹38,000.00).
- Exception #3: `REFUND_NOT_IN_LEDGER` on batch `STL-20261014-R` (−₹5,000.00).
- Exception #4: `MISSING_DEPOSIT` on batch `STL-20261015-M` (−₹58,800.00).

### Step 3: Trigger an AI Agent Investigation
Launch an investigation on Exception #1 (synchronous execution):
```bash
curl -s -X POST \
  -H "Authorization: Bearer test-operator-token" \
  "http://localhost:8080/api/exceptions/1/investigate?sync=true" | jq
```
The agent executes bounded tool calls:
- Calls `inspect_gateway_config` to check fee schedules.
- Calls `draft_remediation_entry` to propose a `fee_inquiry` with an idempotency key.
- Calls `submit_conclusion` with cited evidence.

### Step 4: Inspect Workflow Step Logs
Review the agent's turn-by-turn reasoning and tool calls:
```bash
curl -s -H "Authorization: Bearer test-operator-token" \
  http://localhost:8080/api/workflows/1 | jq
```

### Step 5: Operator Review & Remediation Approval
As a human operator (`Alice`), approve and execute the drafted remediation:
```bash
curl -s -X POST \
  -H "Authorization: Bearer test-operator-token" \
  http://localhost:8080/api/remediations/1/approve | jq
```
Response:
```json
{
  "id": 1,
  "status": "executed"
}
```

Check Exception status (now updated to `action_taken`):
```bash
curl -s -H "Authorization: Bearer test-operator-token" \
  http://localhost:8080/api/exceptions/1 | jq
```

### Step 6: Query Immutable Audit Trail
```bash
curl -s -H "Authorization: Bearer test-operator-token" \
  http://localhost:8080/api/audit-events | jq
```

---

## 8. Role-Based Access Control (RBAC) Testing

- **Operator Token (`test-operator-token`)**: Allowed to perform all actions (`investigate`, `approve`, `reject`, `execute`, `escalate`).
- **Viewer Token (`test-viewer-token`)**: Read-only access. Attempting to approve or investigate returns `403 Forbidden`:
```bash
curl -s -X POST \
  -H "Authorization: Bearer test-viewer-token" \
  http://localhost:8080/api/remediations/1/approve | jq
```
Output:
```json
{
  "error": "Forbidden: operator role required"
}
```

---

## 9. Failure Injection Testing (Mock External APIs)

The mock external services allow injecting failure modes to verify retry resilience:

1. **Inject HTTP 500 error**:
   ```bash
   curl -s -X POST http://localhost:8080/mock/admin/failure-mode \
     -H "Content-Type: application/json" \
     -d '{"mode":"error_500"}' | jq
   ```
2. **Inject Partial Commit then 500**:
   ```bash
   curl -s -X POST http://localhost:8080/mock/admin/failure-mode \
     -H "Content-Type: application/json" \
     -d '{"mode":"commit_then_500"}' | jq
   ```
3. **Reset to Normal**:
   ```bash
   curl -s -X POST http://localhost:8080/mock/admin/failure-mode \
     -H "Content-Type: application/json" \
     -d '{"mode":"none"}' | jq
   ```

---

## 10. Running the Test Suite

Run the full suite of unit and integration tests against `settlement_test`:
```bash
TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/settlement_test?sslmode=disable" \
go test -p 1 -v -count=1 ./...
```
*Note: Always use `-p 1` so tests against the shared test database run serially and prevent table truncation conflicts.*
