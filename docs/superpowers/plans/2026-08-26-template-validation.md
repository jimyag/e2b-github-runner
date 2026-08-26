# Runner Spec template validation implementation plan

> **For agentic workers:** Execute task by task with superpowers:executing-plans, test-driven-development, and verification-before-completion. Do not commit or push without a user request.

**Goal:** Reject invalid custom template changes in the admin save flow using only the configured admin Sandbox service, with actionable errors and atomic persistence.

**Architecture:** Validate before the existing audited transaction. GetTemplate proves existence/access; the owner catalog or public default catalog supplies the effective default build ID, since detail builds are paginated, hidden from non-owners, and may belong to other tags. Keep managed specs and unchanged-template edits independent of Sandbox availability.

**Tech Stack:** Go, existing Qiniu Go SDK, httptest, SQLite test stores, React, Bun.

## Global constraints

- No schema changes, new dependencies, user-side Spec API, Sandbox creation, or runtime credential-resolution changes.
- Admin validation uses its saved endpoint/key even when runtime fallback is disabled or restricted to selected audiences. It never uses the signed-in user's or an organization's credentials.
- Create validates the trimmed template ID. PATCH validates only when the trimmed template ID changes; existing specs are not disabled or rewritten automatically.
- Missing admin configuration rejects custom template changes. Managed defaults remain available through their existing catalog and runtime resolution.
- A concrete nonzero default BuildID proves a runnable uploaded build in the current Sandbox catalog contract; latest BuildStatus alone is not sufficient. Public templates outside the default catalog cannot have readiness verified by this API and receive an explicit unavailable-state error.
- Validation has a five-second total deadline. Missing/not-ready templates return 400; missing admin config returns 409; provider auth/unavailability returns 502; timeouts return 504. Upstream response bodies and credentials are not exposed.
- Rejected saves leave profile and audit state unchanged. Conditional writes use the initial updated_at or insert-only semantics to reject concurrent edits/deletes with 409. Successful saves retain the existing transaction.
- Conditional updates advance the stored timestamp by at least one millisecond beyond the expected revision. Insert-only writes use ordinary INSERT and database duplicate-error translation, independent of MySQL affected-row connection flags.

## Task 1: Provider validation

**Files:** internal/sandboxrunner/runner.go, internal/sandboxrunner/runner_api_test.go.

- [x] Replace history-based tests with GET detail plus catalog fixtures covering missing templates, empty/no default build, valid old build during rebuild, unrelated successful tagged builds, owned and public defaults, public non-default unknown state, upstream errors, and cancellation.
- [x] Run `go test ./internal/sandboxrunner -run TestValidateTemplate -count=1` and observe failures.
- [x] Keep `ValidateTemplate(context.Context, string) error`; use `GetTemplate`, then `ListTemplates` for owned templates or `ListDefaultTemplates` for public non-owned templates. Return `ErrTemplateNotReady` for zero default build ID and `ErrTemplateStateUnavailable` when the detail exists but the catalog cannot confirm it.
- [x] Rerun provider tests to green.

## Task 2: Admin save integration

**Files:** internal/server/server_admin_handlers.go, internal/server/server_profile_validation.go, internal/server/server_test.go, internal/server/server_helpers_test.go; internal/state/catalog.go, internal/state/store.go, internal/state/store_test.go.

- [x] Add POST/PATCH HTTP-backed cases for missing configuration, configured but disabled/selected defaults, explicit default key use, valid/missing/not-ready templates, provider 401/403/429/5xx, cancellation, and rejected mutation/audit preservation.
- [x] Prove managed edits and unchanged-template custom edits make no provider calls, while create always validates even when replacing an existing custom name.
- [x] Run `go test ./internal/server -run 'Test.*Profile' -count=1` and observe failures.
- [x] Add a helper `validateAdminProfileTemplate(http.ResponseWriter, *http.Request, string) bool`; resolve only `GetSandboxServiceDefault`, decrypt through `sandboxServiceForConfig`, validate under a bounded context, and map errors to stable JSON codes/messages.
- [x] Call the helper before `applyMutationWithAudit`; normalize template IDs before both comparison and persistence. Keep managed mutation guards ahead of validation.
- [x] Adapt existing save fixtures to the new contract and rerun server tests.
- [x] Add a server-side total-deadline regression and paused-provider concurrent edit/delete regressions. Verify insert-only/stale/deleted conflicts and audit rollback on SQLite, PostgreSQL, and MySQL, including `clientFoundRows=true` and revision precision.

## Task 3: UI, docs, verification

**Files:** ui/src/hooks/use-runner-catalog.ts and test, ui/src/components/runner-specs-section.tsx and test, ui/src/App.tsx, ui/src/locales/{en,zh}.ts; README.md/README.zh.md; docs/testing.md/docs/zh/testing.md; docs/deployment-smoke.md/docs/zh/deployment-smoke.md; docs/README.md/docs/zh/README.md; TODO.md; AGENTS.md; .agents/rules/.

- [x] Add a regression for pending save state and duplicate-submit suppression; preserve dialog/form on failed validation and allow retry.
- [x] Add `savingRunnerSpec` to the hook and form; disable repeated submission and show translated saving text and validation scope guidance.
- [x] Sync the new save contract and future user-owned Spec boundary in paired docs; link this plan from documentation indexes.
- [x] Run focused Bun tests, `task ui-i18n-check`, `task ui-lint`, `task test`, and `task ui-production-smoke`.
- [x] Review the diff for secrets, unrelated changes, and generated files; run `git diff --check`. Record verification outcomes here. Leave changes uncommitted until publication is explicitly requested.

## Verification results

Verified on 2026-08-26 in branch `fix/issue-44-template-validation`, isolated under `.worktrees/issue-44-template-validation`.

| Check | Result |
| --- | --- |
| Tests-first regressions | Observed expected failures for provider readiness, admin rejection, pending UI state, stale concurrent writes, MySQL duplicate insertion with `clientFoundRows=true`, and timestamp revision advancement; all subsequently passed. |
| `go test ./internal/state -count=1` | Passed, including legacy SQLite upgrade coverage and conditional save behavior. |
| Focused server/provider and Bun tests | Passed. |
| `task test` | Passed: production UI build, 176 Bun tests, and all Go packages with race detection and coverage. |
| `task ui-lint` | Passed: ESLint, TypeScript, and production build. |
| `task ui-i18n-check` | Passed, including all 9 checker regressions. |
| `task ui-production-smoke` | Passed all 4 Chromium checks: landing page, responsive hosted/custom guides, and fixture-backed Jobs scrolling. |
| `Test(ApplyMutationWithAudit\|FreshSchema)SQLBackends` | Passed on dedicated PostgreSQL 16 and MySQL 8.3 databases, both normal MySQL affected-row behavior and `clientFoundRows=true`. Temporary containers removed afterward. |
| Final read-only review | No remaining actionable findings after fixing the concurrency and MySQL cases. |
| `git diff --check` | Passed. No generated assets, local secrets, or databases added to the diff. Original checkout remains clean. |

Builds retain existing chunk-size and Node deprecation warnings. No live Sandbox service, real GitHub workflow, deployed environment, or production SQLite snapshot was exercised; the snapshot test remains skipped without `RUNNERD_SQLITE_SNAPSHOT`. SDK/API behavior was checked against local HTTP fixtures and current Sandbox source. Admin validation does not prove user/organization access or runner image contents.
