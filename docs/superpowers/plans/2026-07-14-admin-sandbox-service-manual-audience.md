# Admin Sandbox Service Manual Audience Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let admins preconfigure selected GitHub users and organizations by login before those accounts sign in or synchronize a GitHub App installation.

**Architecture:** Resolve admin-entered logins through GitHub's account API and persist only the canonical login, stable numeric account ID, and normalized user/organization type. During runner fallback resolution, use the locally synchronized installation owner first, then a dedicated installation-owner cache, and finally GitHub App installation lookup; cache successful remote lookups for later requests.

**Tech Stack:** Go, net/http, GORM AutoMigrate, SQLite/Postgres/MySQL-compatible records, React, TypeScript, Bun tests.

## Execution Status

Implemented on `codex/admin-sandbox-service-default` on 2026-07-14. Manual `login`/`@login` resolution, canonical stable identity persistence, unsupported/not-found rejection, the additive installation-owner cache, runtime GitHub App lookup, known-owner suggestions, UI input, tests, and English/Chinese documentation are complete. `task lint`, `task test`, UI tests, and `git diff --check` pass. The authenticated browser smoke could not be repeated after refresh because the existing admin session had expired and the running local instance reported that GitHub OAuth was not configured; no authentication bypass or real configuration mutation was attempted. Commit and push remain deferred because they were not requested.

## Global Constraints

- Scoped account and organization Sandbox credentials keep precedence over the admin default.
- Selected audience membership is matched only by stable GitHub account ID and normalized account type.
- Free-form unresolved or pending login strings are never persisted.
- Ordinary-user APIs do not expose admin audience entries, endpoint, API key, or owner-cache metadata.
- Existing synchronized accounts remain optional UI suggestions, not an admission prerequisite.

---

### Task 1: Resolve Manual GitHub Account Input

**Files:**
- Modify: `internal/github/client.go`
- Modify: `internal/github/client_test.go`
- Modify: `internal/server/server_admin_sandbox_service.go`
- Modify: `internal/server/server_test.go`

**Interfaces:**
- Produces: `github.Client.GetAccount(context.Context, string) (github.Account, error)`.
- Produces: `POST /admin/api/sandbox-service-default/audiences` accepting `account_login` with optional leading `@`.

- [ ] Write a client test for canonical stable identity returned by `GET /users/{login}` and a server test that adds an account with no local installation row.
- [ ] Run focused tests and confirm they fail because remote account resolution does not exist.
- [ ] Implement account lookup, 404 handling, type validation, canonical metadata persistence, and duplicate idempotency.
- [ ] Run focused tests and confirm they pass.

### Task 2: Resolve and Cache Installation Owners at Runtime

**Files:**
- Modify: `internal/state/records.go`
- Modify: `internal/state/db.go`
- Modify: `internal/state/store.go`
- Modify: `internal/state/github_installations.go`
- Modify: `internal/state/store_test.go`
- Modify: `internal/server/server_sandbox_service.go`
- Modify: `internal/server/server_runner_lifecycle.go`
- Modify: `internal/server/server_test.go`

**Interfaces:**
- Produces: `GetGitHubInstallationOwner(int64)` and `UpsertGitHubInstallationOwner(int64, GitHubInstallationAccount)` store methods.
- Produces: selected-audience resolution that can call `github.Client.GetInstallation` on a local cache miss.

- [ ] Write state tests for owner-cache upsert/read/update and a resolver test for an unsynchronized installation owner.
- [ ] Run state and server tests and confirm they fail on the missing cache APIs and resolver fallback.
- [ ] Add the new AutoMigrate-managed cache table and store methods; no legacy-column backfill is required for an additive table.
- [ ] Thread request context into runner config resolution, query local synchronized owner then cache then GitHub App API, and persist successful lookups.
- [ ] Run state and server tests and confirm the second lookup uses the cache.

### Task 3: Replace the Synced-Only Selector

**Files:**
- Modify: `ui/src/components/sandbox-service-default-section.tsx`
- Modify: `ui/src/components/sandbox-service-default-utils.ts`
- Modify: `ui/src/components/sandbox-service-default-utils.test.js`

**Interfaces:**
- Produces: a free-form GitHub login input with known-account suggestions and Enter/add-button submission.

- [ ] Write a UI utility test for trimming whitespace and an optional leading `@`.
- [ ] Run `cd ui && bun test` and confirm the test fails.
- [ ] Replace the synced-only Select with an Input plus datalist suggestions; keep the compact icon add action and selected-account list.
- [ ] Run UI tests and type/lint checks.

### Task 4: Documentation and Verification

**Files:**
- Modify: `README.md`
- Modify: `docs/testing.md`
- Modify: `docs/zh/testing.md`
- Modify: `docs/deployment-smoke.md`
- Modify: `docs/zh/deployment-smoke.md`
- Modify: `.agents/rules/project-architecture.md`
- Modify: `.agents/rules/testing-and-verification.md`
- Modify: `docs/superpowers/plans/2026-07-14-admin-sandbox-service-audience-policy.md`

- [ ] Replace synchronized-only guidance with GitHub-resolved manual provisioning and runtime owner-cache behavior.
- [ ] Run `go test ./internal/github ./internal/state ./internal/server -count=1`.
- [ ] Run `cd ui && bun test`, `task lint`, `task test`, and `git diff --check`.
- [ ] Smoke-test `/admin/sandbox_service`: selected mode accepts typed login without a synchronized account and renders canonical stable identity after add.

## Acceptance Criteria

- Admins can add `login` or `@login` before the account signs in or synchronizes installations.
- A nonexistent GitHub login is rejected and no pending string is stored.
- Canonical login, account type, and stable numeric ID come from GitHub, not the browser payload.
- A selected owner can use the fallback on the first workflow request even without a local `github_installations` row.
- Successful installation-owner lookup is cached and reused.
- Known local/cache owners remain optional input suggestions.
- Existing precedence, selected-empty, audit, snapshot, and secret-redaction behavior remains intact.
