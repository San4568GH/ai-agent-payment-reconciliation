# Payment Settlement Investigation Workstation: UI & Agent Verification Guide

This guide walks you step-by-step through running and verifying the entire project directly from the **React Workstation UI** (`http://localhost:8080`), powered by your **Google Gemini** configuration.

---

## 0. Quick Note on Your Gemini Configuration (`.env`)

You configured the following in your `.env`:
```env
GEMINI_API_KEY=AQ.Ab8RN6LoeKtEJ7wec2_naJVCMhP1M4yw1zS7ctidq0EI3CtuQw
OPENAI_BASE_URL=https://generativelanguage.googleapis.com/v1beta/openai
LLM_MODEL=gemini-3.6-flash
LLM_MODE=openai
```

> [!TIP]
> **Gemini Model Identifier Tip**:
> Google's OpenAI compatibility endpoint accepts standard model identifiers: **`gemini-1.5-flash`** or **`gemini-2.0-flash`**.
> If Google returns a `404 model not found` error during investigation with `gemini-3.6-flash`, simply change line 14 in your `.env` to:
> ```env
> LLM_MODEL=gemini-1.5-flash
> ```
> *(Or use `LLM_MODE=fake` for instant 100% offline deterministic verification with zero API cost/quota).*

---

## 1. System Startup (Terminal Commands)

Execute these two commands in your terminal to start the database and backend:

### Step 1.1: Ensure PostgreSQL is Running
From the repository directory (`settlement-agent`):
```bash
docker compose up -d
```
**Expected Terminal Output**:
```text
[+] Running 2/2
 ✔ Network settlement-agent_default  Created
 ✔ Container settlement-db           Started
```

---

### Step 1.2: Build and Start the Go Backend Server
```bash
go build -o bin/api ./cmd/api
./bin/api
```
*(Alternatively, you can run directly via: `go run ./cmd/api/main.go`)*

**Expected Terminal Output**:
```text
2026/11/04 09:30:00 Connected to PostgreSQL database: postgres://postgres:postgres@localhost:5432/settlement
2026/11/04 09:30:00 Database migrations applied cleanly (15 tables ready)
2026/11/04 09:30:00 Configured LLM provider: OpenAI-compatible (Base: https://generativelanguage.googleapis.com/v1beta/openai, Model: gemini-3.6-flash)
2026/11/04 09:30:00 Serving React Workstation UI from web/dist
2026/11/04 09:30:00 Server listening on :8080
```

---

## 2. Launching the Workstation UI

Open your browser and navigate to:
👉 **`http://localhost:8080/`**

### What You Should See on First Load:
1. **Header**:
   - Title: `⚖️ Payment Settlement Investigation Workstation`
   - Subtitle: *Autonomous financial reconciliation & human-in-the-loop exception resolution*
   - Health Badge: **`🟢 DB Connected`**
   - Action Buttons: `[Load Seed Data]`, `[Reconcile]`, `[Audit Log]`, and Role Switcher (`[Operator] | [Viewer]`)
2. **DevTools Bar (Gray panel at top)**:
   - `Acting as: operator:Alice` (Green badge)
   - Gateway/Accounting Mock Failure: `[none | error_500 | commit_then_500]`
3. **Empty State Banner**:
   - `No Reconciliation Data Available Yet` with a prompt to click **"Load Seed Data"**.

---

## 3. End-to-End Walkthrough via the UI

### Step 3.1: Load Seed Data via UI
1. In the top navigation bar, click the **"Load Seed Data"** button.
2. The button will show a quick spinner while the 6 CSV files are imported in dependency order.

**Expected UI Output**:
- A green Mantine notification pops up in the top right:
  > **Seed Data Loaded**
  > *All seed files imported successfully (orders, payments, bank deposits, fee schedules).*
- Database now has:
  - 8 Orders
  - 8 Gateway Payments
  - 1 Gateway Refund
  - 7 Bank Deposits (including the decoy deposit)
  - 3 Fee Schedules

---

### Step 3.2: Run Deterministic Reconciliation Engine
1. In the header, confirm the **"As-Of Date"** dropdown is set to **`2026-10-20`**.
2. Click the purple **"Reconcile"** button.

**Expected UI Output**:
- A green notification pops up:
  > **Reconciliation Completed**
  > *6 batches analyzed: 2 matched, 4 with exceptions.*

- **Dashboard KPI Cards Immediately Update**:
  | KPI Card | Expected Value | Explanation |
  | :--- | :--- | :--- |
  | **Payment Volume** | `₹750,000.00` | Total expected gross across all batches |
  | **Expected Net** | `₹734,550.00` | Net payable after fee schedules & refunds |
  | **Bank Received** | `₹631,750.00` | Actual bank credits received |
  | **Batches Matched** | `2 / 6` | Batches A & B matched cleanly ($V = 0$) |
  | **Open Exceptions** | `4` | Batches F, S, R, and M have discrepancies |
  | **Net Variance** | `-₹102,800.00` | Exact algebraic variance $\sum(B - E)$ |

---

### Step 3.3: Verify Settlement Batches in the "Settlements" Tab
1. Click the **"Settlements"** tab (next to "Exceptions & Investigations").
2. You will see all 6 batches listed:

| Batch Ref | Settled On | Gross | Fee | Expected Net | Bank Received | Variance | Status |
| :--- | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| `STL-20261005-A` | 2026-10-05 | ₹100,000 | ₹1,250 | ₹98,750 | ₹98,750 | **₹0.00** | <span style="color:green">**matched**</span> |
| `STL-20261006-B` | 2026-10-06 | ₹100,000 | ₹1,400 | ₹98,600 | ₹98,600 | **₹0.00** | <span style="color:green">**matched**</span> |
| `STL-20261012-F` | 2026-10-12 | ₹250,000 | ₹6,000 | ₹245,000 | ₹244,000 | **-₹1,000.00** | <span style="color:red">**exception**</span> |
| `STL-20261013-S` | 2026-10-13 | ₹100,000 | ₹2,000 | ₹98,000 | ₹60,000 | **-₹38,000.00** | <span style="color:red">**exception**</span> |
| `STL-20261014-R` | 2026-10-14 | ₹80,000 | ₹1,600 | ₹73,400 | ₹73,400 | **-₹5,000.00** | <span style="color:red">**exception**</span> |
| `STL-20261015-M` | 2026-10-15 | ₹60,000 | ₹1,200 | ₹58,800 | ₹0 | **-₹58,800.00** | <span style="color:red">**exception**</span> |

---

### Step 3.4: Autonomous Agent Investigation — Exception 1 (Batch F: Fee Variance)

1. Click back to the **"Exceptions & Investigations"** tab.
2. In the row for **`STL-20261012-F`** (Type: `FEE_VARIANCE`, Variance: `-₹1,000.00`), click the purple **"Investigate"** button.
3. The **Investigation Workspace Drawer** slides open from the right.

**Expected Drawer Output**:
- **Left Column (Variance Decomposition)**:
  - Mathematical breakdown: $(B - R) = ₹0$, $(R - E) = -₹1,000.00$.
  - Reported Fee: `₹6,000.00` vs Expected Fee: `₹5,000.00`.
  - Evidence citations tagged: `ctx:batch`, `ctx:dates`, `ctx:calc`.
- **Right Column (Autonomous Agent Execution Trace)**:
  - Banner shows: `Investigation Run #1 (Completed)`
  - Agent Steps (Timeline):
    - *Step 1 (Tool Call)*: `get_batch_details` or `query_payment_details`
    - *Step 2 (Analysis)*: Agent checks card payments against fee schedule (200 bps vs 240 bps billed by gateway).
    - *Step 3 (Diagnosis)*: *"Gateway billed 2.40% instead of effective 2.00% schedule, overcharging ₹1,000.00."*
    - *Step 4 (Remediation Proposal)*:
      - Proposal Card: **Gateway Fee Inquiry**
      - Target: Gateway Dispute Desk
      - Amount: **₹1,000.00**
      - Status: **`proposal_ready`**
      - Action Buttons: **`[Approve Remediation]`**, **`[Reject]`**, **`[Escalate]`**.

---

### Step 3.5: Human-in-the-Loop Approval & Execution (Batch F)

1. Inside the Proposal Card, click **"Approve Remediation"**.
   - **Expected**: A prompt or confirmation marks status as **`approved`** (recorded in audit trail under `Alice`).
2. Now click the green **"Execute Remediation"** button.
   - **Expected**:
     - System calls the Mock Gateway API (`POST /mock/gateway/fee-inquiry`) with an `Idempotency-Key`.
     - Status updates to **`executed`**.
     - An external ticket ID appears: e.g., `TKT-1001`.
     - The exception status in the main table updates to **`resolved`**.

---

### Step 3.6: Autonomous Agent Investigation — Exception 2 (Batch S: Split Deposit)

1. In the Exceptions table, click **"Investigate"** on **`STL-20261013-S`** (Type: `DELAYED_OR_SPLIT_DEPOSIT`, Variance: `-₹38,000.00`).
2. The Investigation Drawer opens.

**Expected Agent Output**:
- **Diagnosis**: Agent invokes `query_bank_deposits` tool, searches bank ledger, and locates unlinked deposit `TXN-20261019-01` of `₹38,000.00` posted on 2026-10-19 (3 days past the initial 2-day window).
- **Proposed Remediation**:
  - Action: **Adjusting Journal Entry (Deposit Shortfall)**
  - Debit Account: `1010-Cash-In-Transit`
  - Credit Account: `1020-Settlement-Clearing`
  - Amount: **₹38,000.00**
- **Operator Action**:
  - Click **"Approve Remediation"** $\to$ Click **"Execute Remediation"**.
  - Mock ERP Accounting Ledger registers the double-entry journal entry (`entry_id: JRN-5001`).

---

### Step 3.7: Autonomous Agent Investigation — Exception 3 (Batch R: Missing Refund)

1. In the Exceptions table, click **"Investigate"** on **`STL-20261014-R`** (Type: `REFUND_NOT_IN_LEDGER`, Variance: `-₹5,000.00`).

**Expected Agent Output**:
- **Diagnosis**: Agent executes `query_gateway_refunds`, finds refund `REFR1` of `₹5,000.00` was executed on 2026-10-13 by gateway but is missing from internal merchant ledger.
- **Proposed Remediation**:
  - Action: **Adjusting Journal Entry (Unrecorded Refund)**
  - Amount: **₹5,000.00**
- **Operator Action**:
  - Click **"Approve Remediation"** $\to$ Click **"Execute Remediation"**.

---

### Step 3.8: Autonomous Agent Investigation — Exception 4 (Batch M: Missing Deposit Escalation)

1. In the Exceptions table, click **"Investigate"** on **`STL-20261015-M`** (Type: `MISSING_DEPOSIT`, Variance: `-₹58,800.00`, Severity: `high`).

**Expected Agent Output**:
- **Diagnosis**: Agent queries bank deposits across all dates; finds **₹0** bank credit received for reported batch `STL-20261015-M` settled 5 days ago.
- **Autonomous Decision**: Because variance is large (₹58,800 $\ge ₹10,000$) and zero bank money was received, the agent **escalates to human Treasury Ops** instead of proposing an automated financial adjustment.
- **Exception Status**: Changes to **`escalated`** with detailed notes explaining that gateway payout failed to reach the merchant's bank.

---

### Step 3.9: Verify the Immutable Audit Trail

1. In the top navigation bar, click the **"Audit Log"** button.
2. The **Audit Trail Drawer** opens from the right.

**Expected Audit Log Entries** (Chronological, tamper-evident):
- `recon_run`: Triggered for `as_of: 2026-10-20`
- `investigation_started`: Autonomous agent started workflow for Batch F
- `investigation_completed`: Bounded loop finished in $\le 6$ tool calls
- `remediation_proposed`: Proposed fee inquiry for `₹1,000.00`
- `remediation_approved`: Approved by operator `Alice`
- `remediation_executed`: External gateway mock ticket registered
- *(Similar entries for Batch S, Batch R, and Batch M escalation)*

---

### Step 3.10: Demonstrating Security & Chaos to an Evaluator

#### 1. Role-Based Access Control (RBAC) Demonstration:
1. In the header role switcher, click **`[Viewer]`**.
2. Notice the green indicator changes to: `Acting as: viewer:Auditor`.
3. Try clicking **"Load Seed Data"** or **"Reconcile"** $\to$ The buttons are automatically disabled.
4. Try approving a remediation $\to$ Backend rejects with **`403 Forbidden`** (Viewer is strictly read-only).
5. Switch back to **`[Operator]`**.

#### 2. Fault Tolerance & Idempotency Demonstration:
1. In the **DevTools Bar**, set "Gateway/Accounting Mock Failure" to **`commit_then_500`**.
2. When executing a remediation, the external service will commit the ledger entry and then simulate a network timeout/500 crash.
3. Click **"Retry / Execute"** again.
4. **Expected**: The backend passes the identical `Idempotency-Key`. The accounting mock recognizes the key and returns the existing entry **without double-posting funds**!

---

## 4. Summary Checklist for Your Manual Run

- [ ] Started PostgreSQL with `docker compose up -d`
- [ ] Started API server with `./bin/api`
- [ ] Opened `http://localhost:8080/` and verified `DB Connected`
- [ ] Clicked **Load Seed Data** $\to$ Success notification
- [ ] Clicked **Reconcile** $\to$ Matched 2, Open Exceptions 4, Net Variance `-₹102,800.00`
- [ ] Clicked **Investigate** on Batch F $\to$ Gemini diagnosed fee overcharge $\to$ Approved & Executed
- [ ] Clicked **Investigate** on Batch S $\to$ Agent found split deposit $\to$ Approved & Executed
- [ ] Clicked **Investigate** on Batch R $\to$ Agent found missing refund $\to$ Approved & Executed
- [ ] Clicked **Investigate** on Batch M $\to$ High severity missing deposit escalated to Treasury
- [ ] Opened **Audit Log** $\to$ Verified complete tamper-evident history
- [ ] Tested **Viewer Role** and **Mock Failure Mode**

