# Admin Release Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an admin-only Release A observation surface that survives runnerd restarts, replays legacy versus enabled-Spec matching over persisted production requests, and proves per-Spec registration, completion, and cleanup evidence before Release B.

**Architecture:** Add a read-only state report that evaluates one repeatable-read database snapshot without creating new tables or mutating production data. The report groups persisted request inputs, reuses the exact shared matcher, derives lifecycle evidence from durable runner-request state, and checks existing audit events for catalog/Sandbox changes. Expose it through an admin-only Diagnostics endpoint and render it as an independently refreshable Admin panel; serve `/diagnostics/vars` directly from the current process's `expvar` registry instead of choosing a stale pprof address file.

**Tech Stack:** Go 1.25, GORM, SQLite/PostgreSQL/MySQL, `net/http`, `expvar`, React 19, TypeScript, i18next, Bun, happy-dom.

## Global Constraints

- Release A must continue returning the legacy Group/Policy matcher result.
- Do not change existing workflow labels, Runner Specs, Groups, Policies, Sandbox Service configuration, or persisted Runner Request semantics.
- Do not add or delete production database tables or columns; this feature is read-only over existing records.
- Preserve SQLite, PostgreSQL, and MySQL support and the foreign-keyless schema convention.
- Bound replay inputs and catalog-change results; a truncated report must fail the automated gate rather than claim parity.
- A completed request is cleanup evidence only because runnerd writes `completed` after Sandbox stop and GitHub runner removal/absence; show that evidence meaning explicitly in the UI.
- Automated evidence must not claim that backup/restore, uninterrupted process availability, or unchanged external workflow files have been manually signed off.
- Keep all new APIs admin-role gated and do not expose repository or request evidence to ordinary users.
- Edit UI source only under `ui/`; never hand-edit generated `internal/server/ui/` assets.

---

### Task 1: Define and calculate the durable migration readiness report

**Files:**
- Create: `internal/state/catalog_migration_readiness.go`
- Modify: `internal/state/catalog.go`
- Modify: `internal/state/store.go`
- Test: `internal/state/store_test.go`

**Interfaces:**
- Consumes: `runner_requests`, `runner_profiles`, optional legacy catalog tables, `audit_events`, `selectMatchingProfile`, and `legacyAllowedProfiles` within one repeatable-read transaction.
- Produces: `CatalogMigrationReadiness(start, end time.Time) (CatalogMigrationReadiness, error)` on `RunnerCatalogStore`, plus a shared `ProfileMatchComparison.Result()` classification used by both live admission and replay.

- [x] **Step 1: Write the failing state report tests**

Add tests that construct literal profiles and requests and assert the report:

```go
report, err := store.CatalogMigrationReadiness(start, end)
if err != nil { t.Fatal(err) }
if report.Replay.SameRequests != 3 || report.Replay.EnabledOnlyRequests != 1 {
    t.Fatalf("replay summary = %#v", report.Replay)
}
if got := report.Specs[0]; got.RegisteredRequests != 1 || got.CompletedRequests != 1 || got.CleanupFinalizedRequests != 1 {
    t.Fatalf("lifecycle evidence = %#v", got)
}
```

Cover all four matcher classifications, malformed requested-label JSON as an error, enabled profiles with no lifecycle evidence, catalog/Sandbox audit mutations, a clean frozen window, deterministic ordering, an end-exclusive time window, and replay truncation that cannot pass.

- [x] **Step 2: Run the focused state tests and record RED**

Run:

```bash
go test ./internal/state -run 'TestCatalogMigrationReadiness' -count=1
```

Expected: build failure because the report types and method do not exist.

- [x] **Step 3: Add the public report types and store method**

Define the stable data contract in `internal/state/store.go`:

```go
type CatalogMatchReplaySummary struct {
    RequestCount             int64 `json:"request_count"`
    DistinctInputCount       int   `json:"distinct_input_count"`
    SameRequests             int64 `json:"same"`
    LegacyOnlyRequests       int64 `json:"legacy_only"`
    EnabledOnlyRequests      int64 `json:"enabled_only"`
    DifferentProfileRequests int64 `json:"different_profile"`
    ErrorRequests            int64 `json:"errors"`
    Truncated                bool  `json:"truncated"`
}

type RunnerSpecLifecycleEvidence struct {
    Name                     string                       `json:"name"`
    WorkflowLabels           []string                     `json:"workflow_labels"`
    RequestCount             int64                        `json:"request_count"`
    RegisteredRequests       int64                        `json:"registered_requests"`
    CompletedRequests        int64                        `json:"completed_requests"`
    CleanupFinalizedRequests int64                        `json:"cleanup_finalized_requests"`
    Latest                   *RunnerSpecLifecycleExample  `json:"latest,omitempty"`
}

type CatalogMigrationReadiness struct {
    WindowStart             time.Time                       `json:"window_start"`
    WindowEnd               time.Time                       `json:"window_end"`
    Replay                  CatalogMatchReplaySummary       `json:"replay"`
    ReplaySamples           []CatalogMatchReplaySample      `json:"replay_samples"`
    Specs                   []RunnerSpecLifecycleEvidence   `json:"specs"`
    CatalogChanges          []AuditEvent                    `json:"catalog_changes"`
    CatalogChangesTruncated bool                            `json:"catalog_changes_truncated"`
}
```

Add `CatalogMigrationReadiness(start, end time.Time) (CatalogMigrationReadiness, error)` to `RunnerCatalogStore`.

Add a single classification method and literal tests for it:

```go
func (comparison ProfileMatchComparison) Result() string
```

It must preserve the current server behavior, including treating equal profile names with different no-match reasons as `different_profile`.

- [x] **Step 4: Implement one-snapshot bounded replay and lifecycle aggregation**

In `internal/state/catalog_migration_readiness.go`:

- Validate `start < end`.
- Start `db.Transaction(..., catalogSnapshotTxOptions)`.
- Extract the current `CompareProfileMatches` catalog loading into one `loadCatalogSnapshot(tx)` helper and use that helper from both live matching and the report; load profiles, legacy policies/groups/spec membership once.
- Query at most 5,001 distinct `(repository_full_name, requested_labels_json)` inputs ordered deterministically; evaluate only 5,000 and set `Truncated` when the sentinel row exists.
- Parse labels, compute legacy/enabled through `profileMatchFromCandidates`, classify with `ProfileMatchComparison.Result()`, and weight each result by its request count.
- Retain at most 20 non-`same` or error samples with request counts and matcher reasons; never include Sandbox credentials or payload JSON.
- Aggregate requests by current enabled Spec. Count registration only when `running_at IS NOT NULL` and the request has an assigned job marker/ID; count completion and cleanup-finalized only for `status = completed`, `completed_at IS NOT NULL`, and an assigned job.
- Select the latest full-lifecycle request per Spec with only request ID, repository, workflow job ID/URL, labels, and lifecycle timestamps.
- Query at most 101 relevant audit events for `profile.*`, `runner_group.*`, `repository_policy.*`, `sandbox_default.*`, and scoped `sandbox.*`; expose 100 and mark truncation on the sentinel.

- [x] **Step 5: Run state tests and SQL-backend coverage GREEN**

Run:

```bash
go test ./internal/state -run 'TestCatalogMigrationReadiness|TestCompareProfileMatches' -count=1
```

Extend `TestCompareProfileMatchesSQLBackends` so the existing PostgreSQL 17/MySQL 8.4 CI job also creates a persisted request and exercises the readiness report against each real backend.

- [x] **Step 6: Commit the state report**

```bash
git add internal/state/catalog_migration_readiness.go internal/state/catalog.go internal/state/store.go internal/state/store_test.go
git commit -m "feat(state): add catalog migration readiness report"
```

---

### Task 2: Expose current-process metrics and the admin readiness API

**Files:**
- Modify: `internal/server/server.go`
- Modify: `internal/server/server_diagnostics.go`
- Modify: `internal/server/server_runner_lifecycle.go`
- Test: `internal/server/server_helpers_test.go`

**Interfaces:**
- Consumes: `state.Store.CatalogMigrationReadiness`, current process `expvar`, authenticated admin session, and `window_hours`.
- Produces: `GET /diagnostics/catalog-migration-readiness?window_hours=72` and a current-process `GET /diagnostics/vars`.

- [x] **Step 1: Write failing endpoint tests**

Add tests proving:

```go
req := adminRequest(http.MethodGet, "/diagnostics/catalog-migration-readiness?window_hours=72", nil)
rec := httptest.NewRecorder()
srv.ServeHTTP(rec, req)
if rec.Code != http.StatusOK { t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String()) }
```

Assert unauthenticated/non-admin rejection, default 72 hours, accepted 1–720 hour bounds, invalid/zero/over-limit rejection, stable gate codes, current process counters, and manual-gate codes. Add a regression test where stale pprof address files exist and `/diagnostics/vars` still returns a known current `expvar` value without making an HTTP proxy call.

- [x] **Step 2: Run the focused server tests and record RED**

Run:

```bash
go test ./internal/server -run 'TestDiagnostics(VariablesUseCurrentProcess|CatalogMigrationReadiness)' -count=1
```

Expected: failures because the route and direct expvar behavior do not exist.

- [x] **Step 3: Serve expvar directly**

Replace stale-artifact proxying with:

```go
func (s *Server) handleDiagnosticsVars(w http.ResponseWriter, r *http.Request) {
    if !s.requireAdminAuth(w, r) { return }
    expvar.Handler().ServeHTTP(w, r)
}
```

Remove the now-unused `Server.diagnostics` HTTP client.

- [x] **Step 4: Implement the readiness handler and automated gates**

Parse `window_hours` with a default of 72 and range 1–720. Call the state report with `end = time.Now().UTC()` and `start = end.Add(-hours)`. Return stable gate objects for:

```text
window_at_least_72_hours
catalog_unchanged
matcher_parity
all_enabled_specs_full_lifecycle
```

`matcher_parity` passes only when replay is complete and `legacy_only`, `enabled_only`, `different_profile`, and errors are all zero. `all_enabled_specs_full_lifecycle` passes only when every enabled Spec has at least one cleanup-finalized request. Return `automated_gates_passed`; separately return manual requirements `backup_restore_verified`, `continuous_service_observation`, and `workflow_labels_unchanged` so the API never overclaims Release B authorization.

Update `matchProfileForAdmission` to call `comparison.Result()` instead of maintaining a second classification switch. Preserve returning `comparison.Legacy` exactly.

- [x] **Step 5: Run focused and package tests GREEN**

```bash
go test ./internal/server -run 'TestDiagnostics' -count=1
go test ./internal/state ./internal/server -count=1
```

- [x] **Step 6: Commit the admin API**

```bash
git add internal/server/server.go internal/server/server_diagnostics.go internal/server/server_runner_lifecycle.go internal/server/server_helpers_test.go
git commit -m "feat(admin): expose release readiness diagnostics"
```

---

### Task 3: Build the Admin Release A observation panel

**Files:**
- Create: `ui/src/components/release-readiness-section.tsx`
- Create: `ui/src/components/release-readiness-section.test.js`
- Modify: `ui/src/admin-types.ts`
- Modify: `ui/src/App.tsx`
- Modify: `ui/src/components/admin-sections.tsx`
- Modify: `ui/src/components/runner-job-detail.tsx`
- Modify: `ui/src/components/runner-requests-section.tsx`
- Modify: `ui/src/locales/en.ts`
- Modify: `ui/src/locales/zh.ts`
- Modify: `ui/e2e/production-smoke.pw.ts`

**Interfaces:**
- Consumes: `GET /diagnostics/catalog-migration-readiness?window_hours={72|168|720}` through the existing authenticated `request` function.
- Produces: a separate, independently refreshable readiness section on `/admin/diagnostics`.

- [x] **Step 1: Write mounted UI tests first**

Using happy-dom and a real React root, assert:

- Initial load requests only the 72-hour readiness endpoint.
- Summary shows automated pass/block status without claiming manual signoff.
- Parity displays historical weighted counts separately from current-process counters.
- Every enabled Spec row shows workflow labels, registration/completion/cleanup-finalized counts, missing evidence status, and a safe GitHub Job link when available.
- Catalog audit changes and replay truncation visibly block the result.
- Switching to 7 or 30 days issues the correct request.
- Refresh errors remain visible with retry recovery.
- A stale earlier response cannot overwrite a newer window/refresh result.

- [x] **Step 2: Run the mounted test and record RED**

```bash
cd ui && bun test src/components/release-readiness-section.test.js
```

Expected: module-not-found failure because the component has not been created.

- [x] **Step 3: Add TypeScript response types and implement the component**

Create types matching the Go JSON response exactly. Implement `ReleaseReadinessSection` with independent `loading`, `error`, `windowHours`, and request-generation state. Render:

- Automated gate summary and explicit manual-signoff notice.
- Historical matcher replay cards for `same`, `legacy_only`, `enabled_only`, `different_profile`, and errors.
- Current-process counters as supplemental evidence only.
- Catalog-change list with timestamps and actions.
- Per-Spec evidence table with labels, counts, latest evidence, and pass/missing status.
- A concise explanation that cleanup-finalized means terminal state was written only after Sandbox and GitHub runner cleanup completed or absence was confirmed.

- [x] **Step 4: Integrate it into Diagnostics without coupling refresh state**

Pass the existing `request` function into `DiagnosticsSection` and render the new section below the existing runtime diagnostics cards. Keep `/diagnostics/pprof`, raw `/diagnostics/vars`, and readiness refresh failures independent.

- [x] **Step 5: Add aligned English and Chinese copy**

Add literal translation keys for all headings, gate labels, statuses, empty states, explanations, window options, retry, and accessible labels. Keep stable API gate codes untranslated and map them to literal translation keys.

- [x] **Step 6: Run focused UI tests GREEN**

```bash
cd ui && bun test src/components/release-readiness-section.test.js
task ui-i18n-check
task ui-lint
```

- [x] **Step 7: Commit the Admin UI**

```bash
git add ui/src/components/release-readiness-section.tsx ui/src/components/release-readiness-section.test.js ui/src/admin-types.ts ui/src/App.tsx ui/src/components/admin-sections.tsx ui/src/locales/en.ts ui/src/locales/zh.ts
git commit -m "feat(ui): add release readiness dashboard"
```

---

### Task 4: Document evidence semantics and complete verification

**Files:**
- Modify: `README.md`
- Modify: `README.zh.md`
- Modify: `TODO.md`
- Modify: `AGENTS.md`
- Modify: `.agents/rules/testing-and-verification.md`
- Modify: `docs/testing.md`
- Modify: `docs/zh/testing.md`
- Modify: `docs/deployment-smoke.md`
- Modify: `docs/zh/deployment-smoke.md`
- Modify: `docs/superpowers/plans/2026-08-13-remove-runner-groups-policies.md`
- Modify: `docs/superpowers/plans/2026-08-20-admin-release-readiness.md`

**Interfaces:**
- Consumes: shipped API/UI behavior and the existing Release A/B/C migration gates.
- Produces: an operator workflow that separates automated evidence from manual backup/continuity/workflow signoff.

- [x] **Step 1: Update paired operator documentation**

Document:

- How the Admin 72-hour/7-day/30-day views replay persisted requests across runnerd restarts.
- The exact parity meanings and why any truncation/error blocks signoff.
- The exact registration/completion/cleanup-finalized evidence derivation.
- Why GitHub-hosted jobs with labels such as only `ubuntu-latest` may appear as no-match noise but do not count toward enabled Spec lifecycle coverage.
- Which gates still require external operator proof: restorable backup, uninterrupted observation, and unchanged workflow labels.
- That no Admin catalog or Sandbox configuration may change during the signed observation window.

- [x] **Step 2: Run the complete verification matrix**

```bash
go test ./internal/state -count=1
go test ./internal/server -count=1
task ui-i18n-check
task ui-lint
task test
task build
task ui-production-smoke
git diff --check
```

Run the opt-in PostgreSQL/MySQL command when dedicated DSNs are available; otherwise report it as CI-covered but not locally executed.

- [x] **Step 3: Review the final diff against the goal**

Confirm there are no schema migrations, no Group/Policy authority change, no generated assets staged by hand, no secrets, no unbounded history queries, and no UI claim that automated gates alone authorize Release B.

- [x] **Step 4: Commit documentation and plan completion**

```bash
git add docs/testing.md docs/zh/testing.md docs/deployment-smoke.md docs/zh/deployment-smoke.md docs/superpowers/plans/2026-08-13-remove-runner-groups-policies.md docs/superpowers/plans/2026-08-20-admin-release-readiness.md
git commit -m "docs: add release readiness workflow"
```

Completed verification on 2026-08-20:

- Focused state, server, and mounted UI tests passed, including the 5,001-input and 101-audit-event truncation boundaries.
- `task ui-i18n-check`, `task ui-lint`, `task test`, `task build`, `task lint`, and the five-case production Chromium smoke passed.
- The production SQLite snapshot replay and dedicated PostgreSQL/MySQL tests were not run locally because their opt-in environment variables were unset; the existing CI catalog-backends job remains the real-dialect gate.
- Final review confirmed no schema migration, no production configuration mutation, admin-only API authorization, bounded evidence, and Release A continuing to return the legacy matcher result. It also found and fixed the clean-report `replay_samples: null` contract before completion.

- [ ] **Step 5: Push the branch and verify remote parity**

```bash
git push -u origin feat/admin-release-readiness
git rev-parse HEAD
git ls-remote --heads origin feat/admin-release-readiness
```
