# Release C And Unmatched Request Status Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the retired internal Runner Group/Repository Policy code and Release A/B migration surfaces without changing Runner admission semantics or old-database rollback safety, and present admission rejections caused by `profile_labels_not_matched` as neutral unmatched requests in Admin Runner Requests.

**Architecture:** Release B's enabled-Spec matcher remains the sole authoritative matcher. Release C deletes legacy catalog types, CRUD, compatibility APIs, shadow comparison/readiness code, and public `default_available`, while preserving the three obsolete database tables and the physical `runner_profiles.default_available` column for rollback. The Admin UI derives a display-only `unmatched` status from the persisted `failed + admission + profile_labels_not_matched` tuple; persisted lifecycle state is unchanged.

**Tech Stack:** Go 1.26.3, GORM, SQLite/PostgreSQL/MySQL, React 19, TypeScript, Bun tests, i18next.

## Global Constraints

- Do not change any workflow `runs-on` label or the `required_labels ⊆ job_labels ⊆ labels` matcher.
- Keep `runner_group` as the GitHub Organization Runner Group field on Specs and Requests.
- Do not mutate production configuration or production data.
- Do not call `DropTable`; old `runner_groups`, `runner_group_specs`, and `repository_policies` tables and rows must remain untouched.
- Keep the physical `runner_profiles.default_available NOT NULL` column write-compatible while removing it from public types, APIs, UI, and matching.
- Edit UI source under `ui/`; never hand-edit `internal/server/ui/`.

---

### Task 1: Lock Release C database and HTTP compatibility contracts

**Files:**
- Modify: `internal/state/store_test.go`
- Modify: `internal/server/server_helpers_test.go`
- Modify: `internal/server/server_test.go`

**Interfaces:**
- Consumes: current Release B schema and authenticated admin router.
- Produces: tests requiring obsolete tables to be absent on fresh databases, preserved on legacy databases, retired APIs to be absent, and Runner Spec JSON to omit `default_available`.

- [ ] **Step 1: Write failing state tests**

Add a fresh-schema test that asserts `runner_groups`, `runner_group_specs`, and `repository_policies` are not created. Add a legacy fixture test that creates those tables with literal rows, opens the Release C store twice, and asserts identical row counts/content plus preserved Runner Profile custom indexes and Runner Request snapshot fields.

- [ ] **Step 2: Verify the state tests fail for the Release B behavior**

Run: `go test ./internal/state -run 'TestReleaseC(FreshSchemaOmitsRetiredCatalogTables|LegacyCatalogTablesRemainUntouched)' -count=1`

Expected: the fresh-schema assertion fails because Release B still auto-migrates the retired tables.

- [ ] **Step 3: Write failing server tests**

Require authenticated requests to `/runner_groups`, `/runner_groups/name`, and `/runner_policies` to return `404`; require old `/admin/runner_groups` and `/admin/runner_policies` URLs to continue redirecting to `/admin/runner_specs`; require Runner Spec list/detail JSON not to contain `default_available`.

- [ ] **Step 4: Verify the server tests fail for the Release B shims**

Run: `go test ./internal/server -run 'TestReleaseC(RemovesRetiredCatalogAPIs|RunnerSpecJSONOmitsDefaultAvailable)' -count=1`

Expected: retired APIs return `200`/`410`, or Runner Spec JSON still contains `default_available`.

---

### Task 2: Remove state models while preserving legacy physical data

**Files:**
- Modify: `internal/state/store.go`
- Modify: `internal/state/records.go`
- Modify: `internal/state/conversions.go`
- Modify: `internal/state/catalog.go`
- Delete: `internal/state/catalog_migration_readiness.go`
- Modify: `internal/state/db.go`
- Modify: `internal/state/store_test.go`
- Modify: `internal/state/store_extra_test.go`
- Modify: `internal/runnercatalog/defaults.go`
- Modify: `internal/runnercatalog/defaults_test.go`

**Interfaces:**
- Consumes: `RunnerCatalogStore` profile CRUD and `MatchProfile`.
- Produces: a profile-only catalog store, an internal write-only legacy column mapping, and fresh schema without obsolete tables.

- [ ] **Step 1: Remove public catalog types and methods**

Delete `RunnerGroup`, `RepositoryPolicy`, comparison/readiness structs, Group/Policy CRUD methods, `CompareProfileMatches`, and `CatalogMigrationReadiness` from `RunnerCatalogStore`.

- [ ] **Step 2: Remove legacy catalog records and logic**

Delete Group/Policy record types, conversions, CRUD, wildcard authorization, snapshot comparison, and Group-link cleanup. Keep enabled-Spec `MatchProfile` ordering and no-match reasons unchanged.

- [ ] **Step 3: Stop auto-migrating obsolete tables**

Remove the three obsolete models and the legacy Policy-column migration from startup. Do not add any destructive migration.

- [ ] **Step 4: Make `default_available` internal-only**

Remove `DefaultAvailable` from public `RunnerProfile`. Retain an internal physical column field on `runnerProfileRecord`, write a compatibility value for inserts, and omit it from update assignments so old values remain unchanged.

- [ ] **Step 5: Run the Release C state tests green**

Run: `go test ./internal/state -count=1`

Expected: PASS with old-table preservation, fresh-schema omission, repeat migration, Runner Profile index, and Runner Request snapshot coverage.

---

### Task 3: Remove server/API shadow compatibility and migration diagnostics

**Files:**
- Modify: `internal/server/server.go`
- Modify: `internal/server/server_admin_handlers.go`
- Modify: `internal/server/server_runner_lifecycle.go`
- Modify: `internal/server/server_diagnostics.go`
- Modify: `internal/server/server_helpers_test.go`
- Modify: `internal/server/server_test.go`
- Modify: `internal/server/server_default_templates_test.go`
- Modify: `internal/metrics/metrics.go`
- Modify: `internal/metrics/metrics_test.go`

**Interfaces:**
- Consumes: authoritative `MatchProfile` only.
- Produces: no Group/Policy API routes, no shadow comparison metrics, and no catalog-migration readiness endpoint.

- [ ] **Step 1: Replace admission shadow comparison with direct matching**

Make `matchProfileForAdmission(repository, labels)` delegate to `MatchProfile` without comparison, classification, logging, or comparison metrics.

- [ ] **Step 2: Delete compatibility routes and handlers**

Remove `/runner_groups`, `/runner_policies`, retired mutation handlers, deprecation headers, and their tests. Preserve Admin SPA redirects.

- [ ] **Step 3: Delete migration-only diagnostics and metrics**

Remove `/diagnostics/catalog-migration-readiness`, its response/gate helpers, catalog comparison expvar metrics, and migration-specific tests. Keep general Diagnostics, recent failures, runtime metrics, and pprof.

- [ ] **Step 4: Run focused server and metrics tests**

Run: `go test ./internal/server ./internal/metrics -count=1`

Expected: PASS.

---

### Task 4: Present label no-match admission records as neutral Admin status

**Files:**
- Modify: `ui/src/admin-types.ts`
- Modify: `ui/src/admin-format.ts`
- Modify: `ui/src/admin-format.test.js`
- Modify: `ui/src/App.tsx`
- Modify: `ui/src/components/admin-shared.tsx`
- Modify: `ui/src/components/runner-requests-section.tsx`
- Modify: `ui/src/components/runner-requests-section.test.js`
- Modify: `ui/src/locales/en.ts`
- Modify: `ui/src/locales/zh.ts`

**Interfaces:**
- Consumes: persisted `RunnerState.status`, `failure_stage`, and `failure_reason`.
- Produces: `AdminRunnerDisplayStatus = RunnerStatus | "unmatched"` and `runnerDisplayStatus(runner)`.

- [ ] **Step 1: Write failing display-status unit tests**

Assert the literal tuple `{status:"failed", failure_stage:"admission", failure_reason:"profile_labels_not_matched"}` derives `unmatched`; other admission failures and runtime failures remain `failed`; failed metrics exclude only the unmatched tuple.

- [ ] **Step 2: Write a failing rendered-list test**

Render an unmatched request in Chinese and assert the list/detail use `未匹配`, the badge is non-danger, and no retry button is rendered. Render a genuine failed request and assert it still shows `失败` with retry.

- [ ] **Step 3: Verify UI tests fail**

Run: `cd ui && bun run test admin-format.test.js components/runner-requests-section.test.js`

Expected: FAIL because Release B maps both records to `failed`.

- [ ] **Step 4: Implement the display-only derived status**

Add English `Not matched` and Chinese `未匹配` copy. Use the derived status for list badges, detail status, filtering, failed metrics, and retry visibility. Do not change backend state or ordinary-user Jobs filtering.

- [ ] **Step 5: Run focused UI tests green**

Run: `cd ui && bun run test admin-format.test.js components/runner-requests-section.test.js`

Expected: PASS.

---

### Task 5: Remove the Release A/B Admin readiness surface

**Files:**
- Modify: `ui/src/App.tsx`
- Modify: `ui/src/admin-types.ts`
- Delete: `ui/src/components/release-readiness-section.tsx`
- Delete: `ui/src/components/release-readiness-section.test.js`
- Modify: `ui/src/locales/en.ts`
- Modify: `ui/src/locales/zh.ts`

**Interfaces:**
- Consumes: general Diagnostics response and expvar output.
- Produces: Diagnostics without the completed migration-readiness panel or removed endpoint calls.

- [ ] **Step 1: Remove migration-only types, imports, rendering, and translations**

Keep Diagnostics summary, recent failures, `/debug/vars`, and pprof presentation unchanged.

- [ ] **Step 2: Run all UI tests and i18n validation**

Run: `cd ui && bun run test`

Run: `task ui-i18n-check`

Expected: PASS.

---

### Task 6: Synchronize operator, user, and agent documentation

**Files:**
- Modify: `README.md`
- Modify: `README.zh.md`
- Modify: `TODO.md`
- Modify: `AGENTS.md`
- Modify: `docs/testing.md`
- Modify: `docs/zh/testing.md`
- Modify: `docs/deployment-smoke.md`
- Modify: `docs/zh/deployment-smoke.md`
- Modify: `docs/runner-architecture-comparison.md`
- Modify: `docs/zh/runner-architecture-comparison.md`
- Modify: `ui/src/content/site-docs/en/custom-templates.md`
- Modify: `ui/src/content/site-docs/zh/custom-templates.md`
- Modify: `.agents/rules/project-architecture.md`
- Modify: `.agents/rules/testing-and-verification.md`

**Interfaces:**
- Consumes: shipped Release C behavior.
- Produces: one documented availability control (`enabled`), explicit GitHub Runner Group naming, and a separate post-Release-C destructive cleanup runbook boundary.

- [ ] **Step 1: Remove compatibility-release wording**

State that Group/Policy APIs and code models are gone while obsolete physical tables remain rollback data until separately authorized cleanup.

- [ ] **Step 2: Document unmatched Admin status semantics**

Explain that `profile_labels_not_matched` means runnerd did not claim the GitHub job and is displayed as `Not matched`/`未匹配`, not as an infrastructure failure.

- [ ] **Step 3: Run paired-document and diff checks**

Run: `task ui-i18n-check`

Run: `git diff --check`

Expected: PASS.

---

### Task 7: Full Release C verification and branch review

**Files:**
- Verify all changed files.

**Interfaces:**
- Consumes: complete Release C implementation.
- Produces: a reviewable branch; production deployment remains separately authorized.

- [ ] **Step 1: Run focused schema verification**

Run: `go test ./internal/state -count=1`

- [ ] **Step 2: Run full repository verification**

Run: `task lint`

Run: `task test`

Run: `task build`

Run: `task docker-check`

Run: `task ui-production-smoke`

- [ ] **Step 3: Inspect the complete diff**

Run: `git diff --check`

Run: `git status --short`

Run: `git diff --stat upstream/main...HEAD`

Expected: only Release C, unmatched-status, generated production UI assets, tests, and synchronized docs are changed; no secret or local-state files are present.

- [ ] **Step 4: Stop at a verified worktree until publication is authorized**

Do not commit or push without explicit user authorization. Once authorized, use concise Angular-style commits and push `refactor/remove-runner-groups-policies-release-c` to the fork. Do not deploy or modify production.
