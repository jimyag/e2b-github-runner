# Admin Readiness Drill-down Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Admin Diagnostics explain missing Runner Spec lifecycle evidence with bounded recent request attempts, statuses, failure stages/reasons, timestamps, and safe GitHub Job links.

**Architecture:** Extend the existing read-only `CatalogMigrationReadiness` report instead of adding an endpoint or database schema. While the report holds its repeatable-read snapshot, select at most five newest persisted requests for each enabled Spec and expose a redacted attempt contract. Render those attempts as an accessible expandable detail row inside the existing lifecycle table.

**Tech Stack:** Go 1.25, GORM, SQLite/PostgreSQL/MySQL, React 19, TypeScript, i18next, Bun, happy-dom.

## Global Constraints

- Do not modify production configuration or production data.
- Do not add, remove, or alter database tables, columns, indexes, or migrations.
- Keep Release A admission authoritative on the legacy Group/Policy matcher.
- Keep the readiness query read-only, bounded, deterministic, and compatible with SQLite, PostgreSQL, and MySQL.
- Never expose Sandbox credentials, webhook payload JSON, runner logs, or raw `error`/`last_error_message` fields in readiness evidence.
- Return runtime status, failure stage, and failure reason as stable identifiers; translate only fixed UI labels.
- Keep all readiness evidence admin-only and edit UI source only under `ui/`.

---

### Task 1: Add bounded per-Spec attempt evidence to the state report

**Files:**
- Modify: `internal/state/store.go`
- Modify: `internal/state/catalog_migration_readiness.go`
- Test: `internal/state/store_test.go`

**Interfaces:**
- Consumes: existing `runner_requests` rows already selected by `queued_at >= start AND queued_at < end` and `profile_name = ?`.
- Produces: `RunnerSpecLifecycleAttempt` and `RunnerSpecLifecycleEvidence.RecentAttempts`, with at most five newest attempts per enabled Spec.

- [x] **Step 1: Write a failing state test for diagnostic attempts**

Add `TestCatalogMigrationReadinessIncludesBoundedRecentAttempts` with literal requests that proves newest-first ordering, an end-exclusive window, the five-row limit, failed-stage/reason preservation, and omission of old requests. The expected attempt contract is:

```go
type RunnerSpecLifecycleAttempt struct {
    RequestID          string     `json:"request_id"`
    RepositoryFullName string     `json:"repository_full_name"`
    Status             string     `json:"status"`
    WorkflowJobID      int64      `json:"workflow_job_id,omitempty"`
    GitHubJobURL       string     `json:"github_job_url,omitempty"`
    RequestedLabels    []string   `json:"requested_labels"`
    FailureStage       string     `json:"failure_stage,omitempty"`
    FailureReason      string     `json:"failure_reason,omitempty"`
    QueuedAt           time.Time  `json:"queued_at"`
    RegisteredAt       *time.Time `json:"registered_at,omitempty"`
    CompletedAt        *time.Time `json:"completed_at,omitempty"`
}
```

The production mutation caught by this test is an unbounded, oldest-first, cross-window, or diagnostically empty attempt query.

- [x] **Step 2: Run the focused state test and verify RED**

Run:

```bash
go test ./internal/state -run 'TestCatalogMigrationReadiness(IncludesBoundedRecentAttempts|ReturnsStableEmptyCollections)' -count=1
```

Expected: compile failure because `RecentAttempts` and `RunnerSpecLifecycleAttempt` do not exist.

- [x] **Step 3: Implement the minimal bounded query**

Add `maxCatalogMigrationLifecycleAttempts = 5`. For each enabled profile, initialize `RecentAttempts` to an empty slice, then query:

```go
err := tx.Select(runnerRequestListSelectColumns).
    Where("queued_at >= ? AND queued_at < ?", start, end).
    Where("profile_name = ?", profileName).
    Order("queued_at DESC, id ASC").
    Limit(maxCatalogMigrationLifecycleAttempts).
    Find(&records).Error
```

Convert only the public fields listed above. Use pointers for optional lifecycle timestamps and do not copy `Error`, `LastErrorMessage`, payload JSON, or Sandbox fields.

- [x] **Step 4: Verify GREEN and cross-dialect coverage**

Run:

```bash
go test ./internal/state -run 'TestCatalogMigrationReadiness|TestCompareProfileMatches' -count=1
```

Extend `TestCompareProfileMatchesSQLBackends` to assert the persisted request is present in `RecentAttempts`, so the existing PostgreSQL/MySQL CI job covers the new query.

---

### Task 2: Preserve the attempt contract through the admin Diagnostics API

**Files:**
- Test: `internal/server/server_helpers_test.go`

**Interfaces:**
- Consumes: embedded `state.CatalogMigrationReadiness` returned by `diagnosticsReadinessStore`.
- Produces: unchanged `GET /diagnostics/catalog-migration-readiness?window_hours={hours}` with `specs[].recent_attempts` serialized as a stable array.

- [x] **Step 1: Write a failing endpoint assertion**

Add a literal failed attempt to the fake report and decode `Specs` in the HTTP response. Assert request ID, status, failure stage/reason, and `recent_attempts: []` stability for a second Spec.

- [x] **Step 2: Run the focused endpoint test and verify RED**

Run:

```bash
go test ./internal/server -run TestDiagnosticsCatalogMigrationReadinessReturnsAutomatedAndManualGates -count=1
```

Expected: compile failure until Task 1 defines the attempt contract.

- [x] **Step 3: Verify the existing embedded response needs no production handler change**

Keep `catalogMigrationReadinessResponse` embedding `state.CatalogMigrationReadiness`. Do not add another route, query parameter, database call, or mutation.

- [x] **Step 4: Run the server Diagnostics tests GREEN**

Run:

```bash
go test ./internal/server -run 'TestDiagnostics' -count=1
```

---

### Task 3: Add an accessible investigation row to the lifecycle table

**Files:**
- Modify: `ui/src/admin-types.ts`
- Modify: `ui/src/components/release-readiness-section.tsx`
- Modify: `ui/src/components/release-readiness-section.test.js`
- Modify: `ui/e2e/production-smoke.pw.ts`
- Modify: `ui/src/locales/en.ts`
- Modify: `ui/src/locales/zh.ts`

**Interfaces:**
- Consumes: `RunnerSpecLifecycleEvidence.recent_attempts` from the existing readiness request.
- Produces: one `aria-expanded` investigation button per Spec and an adjacent detail row showing at most five attempts.

- [x] **Step 1: Write mounted UI tests first**

Extend the fixture with one failed attempt and add tests that click `Inspect attempts`, then assert the real rendered component contains the localized failed status plus literal repository, request ID, workflow labels, failure stage/reason, queued time, and safe GitHub link. Add a zero-attempt fixture and assert the expanded row says no requests matched the Spec in the selected window.

The production mutations caught are dropping the failure reason, binding the disclosure to the wrong Spec, or hiding the zero-traffic distinction.

- [x] **Step 2: Run the mounted test and verify RED**

Run:

```bash
cd ui && bun test src/components/release-readiness-section.test.js
```

Expected: failure because the attempt disclosure and copy do not exist.

- [x] **Step 3: Implement the minimal UI**

Add `RunnerSpecLifecycleAttempt` typed with `RunnerStatus`. Render each summary row plus an optional adjacent `<TableRow>` keyed by the Spec name. The disclosure button must use `aria-expanded` and `aria-controls`; the detail row must reuse `StatusBadge`, show raw failure identifiers without translation, use `formatTime(..., i18n.resolvedLanguage)`, and open `github_job_url` with `target="_blank" rel="noreferrer"` when present.

Use a restrained operational style: selected evidence receives a subtle muted background and a left status accent; no new dependency, modal, animation system, or decorative illustration.

- [x] **Step 4: Verify focused UI and i18n GREEN**

Run:

```bash
cd ui && bun test src/components/release-readiness-section.test.js
task ui-i18n-check
task ui-lint
```

---

### Task 4: Synchronize operator guidance and complete verification

**Files:**
- Modify: `docs/deployment-smoke.md`
- Modify: `docs/zh/deployment-smoke.md`
- Modify: `.agents/rules/testing-and-verification.md`
- Modify: `AGENTS.md`
- Modify: `TODO.md`

**Interfaces:**
- Consumes: the shipped Admin Diagnostics attempt disclosure.
- Produces: bilingual operator steps that diagnose missing registration without direct production writes.

- [x] **Step 1: Update paired deployment guidance**

Document that operators should expand recent attempts, distinguish no traffic from pre-registration failures, preserve existing workflow labels, and use normal controlled workflow jobs only after the failure reason is understood. State explicitly that changing Catalog/Sandbox data restarts the freeze window.

- [x] **Step 2: Update durable project rules and TODO state**

Record that readiness attempt evidence stays bounded and redacted and that the Admin UI is now the primary diagnosis surface for missing lifecycle evidence. Do not claim backup/restore or Release B authorization.

- [x] **Step 3: Run the full verification matrix**

Run sequentially:

```bash
go test ./internal/state -count=1
go test ./internal/server -count=1
task ui-i18n-check
task lint
task test
task build
task ui-production-smoke
git diff --check
```

- [x] **Step 4: Review the complete diff and commit**

Confirm no generated `internal/server/ui/` assets, sqlite databases, secrets, or local configuration are staged. Commit only the scoped implementation and plan with:

```bash
git commit -m "feat(admin): add readiness evidence drill-down"
```
