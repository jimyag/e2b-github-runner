# Admin Sandbox Service Audience Policy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restrict the admin-managed Sandbox service default to either all GitHub repository owners or an explicit allowlist of GitHub users and organizations.

**Architecture:** Extend synced GitHub installation ownership with the stable GitHub account ID and account type, then store selected audience entries in a dedicated table keyed by that identity. Scoped credentials remain first choice; the admin default resolver checks repository-owner eligibility only at the final fallback. Saved runner credential snapshots bypass later audience changes.

**Tech Stack:** Go 1.24, `net/http`, GORM, sqlite/Postgres/MySQL, React 19, TypeScript, Vite, Tailwind CSS.

## Execution Status

Implemented on `codex/admin-sandbox-service-default` on 2026-07-14, then amended by `2026-07-14-admin-sandbox-service-manual-audience.md`. Stable GitHub owner identity, audience persistence, role-gated admin APIs, audit events, owner eligibility, snapshot stability, admin UI, tests, and English/Chinese documentation are complete. The amendment removes the prior synchronization prerequisite: admins type a login, GitHub supplies the stable identity, and runtime installation-owner lookup is cached. Commit and push remain deferred because they were not requested.

## Global Constraints

- Audience modes are exactly `all` and `selected`; an empty `selected` audience matches nobody.
- Match repository ownership, not the workflow actor or organization membership.
- Match by stable GitHub numeric account ID plus account type; login is display metadata only.
- Existing and empty audience mode values normalize to `all` for backward compatibility.
- Scoped custom and inherited credentials always win before audience evaluation.
- Saved runner snapshots remain usable after audience removal or mode changes.
- Ordinary-user responses expose only the effective source, never the audience list.
- Admin writes and audience additions/removals require admin auth and produce audit events.
- Unknown GitHub owners are rejected; valid users and organizations can be added before sign-in or installation sync.
- Schema changes use GORM models and nullable/default-compatible columns for old sqlite databases.
- Do not hand-edit generated files under `internal/server/ui/`.
- Do not commit or push unless explicitly requested.

---

### Task 1: Persist Stable GitHub Installation Owner Identity

**Files:**
- Modify: `internal/github/client.go`
- Test: `internal/github/client_test.go`
- Modify: `internal/state/store.go`
- Modify: `internal/state/records.go`
- Modify: `internal/state/github_installations.go`
- Test: `internal/state/store_test.go`

**Interfaces:**
- Produces: `github.Installation.AccountID int64` and `AccountType string`.
- Produces: matching fields on `state.GitHubInstallation` and stable owner lookup/list APIs.

- [ ] **Step 1: Write failing GitHub client tests**

Update installation fixtures with `account.id` and `account.type`, then assert both fields are returned by `GetInstallation` and `ListUserInstallations`.

- [ ] **Step 2: Run client tests and confirm RED**

Run: `go test ./internal/github -run 'TestGetInstallation|TestListUserInstallations' -count=1`

Expected: compile failure because `AccountID` and `AccountType` do not exist.

- [ ] **Step 3: Implement GitHub response parsing**

Parse and return account identity from both installation response shapes; normalize types to lowercase `user` or `organization`.

- [ ] **Step 4: Write failing state round-trip and lookup tests**

Assert installation upserts preserve stable owner identity, deduplicate shared installations, list known owner candidates, and find an owner by installation ID or case-insensitive login.

- [ ] **Step 5: Run state tests and confirm RED**

Run: `go test ./internal/state -run 'TestGitHubInstallation.*Account' -count=1`

- [ ] **Step 6: Add model fields and store APIs**

Add nullable-compatible `github_account_id` and `account_type` columns and expose:

```go
type GitHubInstallationAccount struct {
    GitHubAccountID int64
    AccountType     string
    AccountLogin    string
    AccountName     string
    AccountAvatar   string
}

ListGitHubInstallationAccounts() ([]GitHubInstallationAccount, error)
GitHubInstallationAccountForInstallation(installationID int64) (GitHubInstallationAccount, error)
GitHubInstallationAccountForLogin(login string) (GitHubInstallationAccount, error)
```

- [ ] **Step 7: Run client and state tests and confirm GREEN**

Run: `go test ./internal/github ./internal/state -count=1`

### Task 2: Store and Administer Audience Policy

**Files:**
- Modify: `internal/state/store.go`
- Modify: `internal/state/records.go`
- Modify: `internal/state/db.go`
- Modify: `internal/state/sandbox_service_defaults.go`
- Test: `internal/state/store_test.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/server_admin_sandbox_service.go`
- Test: `internal/server/server_test.go`

**Interfaces:**
- Produces: `SandboxServiceDefault.AudienceMode` and `SandboxServiceDefaultAudience` CRUD APIs.
- Produces: admin response fields `audience_mode`, `audiences`, and `available_accounts`.
- Produces: `POST /admin/api/sandbox-service-default/audiences` and `DELETE /admin/api/sandbox-service-default/audiences/{id}`.

- [ ] **Step 1: Write failing state lifecycle tests**

Cover `all` normalization, `selected` persistence, unique `(github_account_id, account_type)`, audience listing, and idempotent deletion.

- [ ] **Step 2: Run focused state tests and confirm RED**

Run: `go test ./internal/state -run 'TestSandboxServiceDefaultAudience' -count=1`

- [ ] **Step 3: Implement records and store methods**

Add the nullable-compatible `audience_mode` column and `sandbox_service_default_audiences` table with a composite unique index. Normalize type and login display metadata.

- [ ] **Step 4: Write failing admin API tests**

Cover auth, invalid mode, selected-empty response, adding known owners, rejecting unknown owners, duplicate add idempotency, deletion, audit events, and absence from ordinary-user payloads.

- [ ] **Step 5: Run focused server tests and confirm RED**

Run: `go test ./internal/server -run 'TestAdminSandboxServiceDefaultAudience' -count=1`

- [ ] **Step 6: Implement admin handlers and routes**

Save `audience_mode` with the existing default. Add audiences by resolving a login through GitHub server-side; never accept a client-supplied numeric identity as authoritative. Return known suggestions only to admins.

- [ ] **Step 7: Run state and admin tests and confirm GREEN**

Run: `go test ./internal/state ./internal/server -count=1`

### Task 3: Enforce Repository Owner Eligibility

**Files:**
- Modify: `internal/server/server_sandbox_service.go`
- Modify: `internal/server/server_sandbox_catalog.go`
- Modify: `internal/server/server_user_handlers.go`
- Test: `internal/server/server_test.go`

**Interfaces:**
- Consumes: stable owner lookups and audience store APIs from Tasks 1 and 2.
- Produces: one eligibility path shared by runner creation and ordinary-user catalogs.

- [ ] **Step 1: Write failing resolver tests**

Cover `all`, selected match/miss, empty selected audience, user and organization owners, scoped override, corrupt scoped config, and saved snapshot behavior after audience removal.

- [ ] **Step 2: Run resolver tests and confirm RED**

Run: `go test ./internal/server -run 'TestSandboxService.*Audience' -count=1`

- [ ] **Step 3: Implement eligibility resolution**

Resolve installation owners by stable identity and personal-account owners from the GitHub OAuth subject. Evaluate audience only after scoped resolution returns `errSandboxServiceNotConfigured`; preserve all other errors.

- [ ] **Step 4: Verify runner, catalog, and Preferences behavior**

Run: `go test ./internal/server -count=1`

Expected: non-eligible users report `resolved_source: none` without audience metadata.

### Task 4: Add Admin Audience Controls and Documentation

**Files:**
- Modify: `ui/src/admin-types.ts`
- Modify: `ui/src/components/sandbox-service-default-section.tsx`
- Modify: `ui/src/components/sandbox-service-default-utils.ts`
- Test: `ui/src/components/sandbox-service-default-utils.test.js`
- Modify: `README.md`
- Modify: `docs/testing.md`
- Modify: `docs/zh/testing.md`
- Modify: `docs/deployment-smoke.md`
- Modify: `docs/zh/deployment-smoke.md`
- Modify: `.agents/rules/project-architecture.md`
- Modify: `.agents/rules/testing-and-verification.md`

**Interfaces:**
- Produces: an `Availability` control with `All accounts` and `Selected accounts`, a manual login input with known-owner suggestions, and a selected-owner list.

- [ ] **Step 1: Write failing UI utility tests**

Test audience labels, candidate filtering, identity keys, and selected-empty status copy.

- [ ] **Step 2: Run UI tests and confirm RED**

Run: `cd ui && bun test`

- [ ] **Step 3: Implement audience controls**

Use a segmented control for mode, a manual login input with suggestions, type badges, and icon-only remove buttons with tooltips. Keep Region/API Key compact and do not restore custom endpoint support.

- [ ] **Step 4: Update operator and verification docs**

Document owner matching, selected-empty behavior, scoped precedence, manual preconfiguration, runtime owner caching, audit actions, and deployment checks in English and Chinese guides.

- [ ] **Step 5: Run UI verification and confirm GREEN**

Run: `cd ui && bun test && cd .. && task ui-lint`

### Task 5: Full Verification and Browser Smoke

**Files:**
- Modify: this plan's execution status after verification.

- [ ] **Step 1: Run repository gates**

```bash
go test ./internal/state -count=1
go test ./internal/server -count=1
task test
task lint
git diff --check
```

- [ ] **Step 2: Run authenticated browser smoke**

Verify `/admin/sandbox_service` at desktop and narrow width: switch modes, add/remove a known user or organization, confirm selected-empty warning and reload persistence, and ensure Region/API Key remain aligned. Verify eligible and ineligible account routes report the expected effective source.

- [ ] **Step 3: Record execution status**

Record completed checks and any environment limitation without claiming unverified behavior.

## Acceptance Criteria

- Existing defaults continue using `all` without migration-time behavior change.
- `selected` with no entries matches nobody.
- Audience entries use GitHub numeric account identity and survive login renames.
- Valid GitHub users and organizations can be added from admin UI/API before sign-in or installation sync.
- Scoped credentials always override the admin audience decision.
- Saved runner snapshots continue after an audience entry is removed.
- Runner creation, account/org catalogs, and Preferences share eligibility behavior.
- Ordinary users cannot read or mutate audience membership.
- Admin changes are audited without secrets.
- State, server, UI, full repository, lint, and browser checks pass or limitations are recorded.
