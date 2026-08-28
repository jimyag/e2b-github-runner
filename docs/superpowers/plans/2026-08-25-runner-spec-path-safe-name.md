# Runner Spec Path-Safe Name Implementation Plan

> **Lifecycle:** Historical plan, completed in `0bab1e0` (`fix(runner): reject path-unsafe Runner Spec names (#86)`). The checklist below records the original execution sequence and is not active work; see [`README.md`](README.md) and the repository `TODO.md` for current status.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reject newly written Runner Spec names that contain `/` or equal `.` or `..`, while preserving startup, read, match, and lifecycle compatibility for historical rows with those names.

**Architecture:** Keep the database schema unchanged. Add one shared state-layer validator and call it from both Runner Spec write paths, `UpsertProfile` and `ReconcileManagedProfiles`; leave reads, migration, matching, and single-resource routes unchanged. Reuse the existing audited mutation transaction so rejected HTTP creates commit neither a profile nor an audit event.

**Tech Stack:** Go 1.26.3, GORM, `net/http`, React 19, TypeScript, Bun test.

## Global Constraints

- Do not add a database constraint, migration, rename, delete, disable, or repair historical records.
- After trimming, reject names containing `/` and names exactly equal to `.` or `..`.
- Do not introduce a strict ASCII allowlist or reject backslashes, spaces, Unicode, or other characters in this issue.
- Do not change `{name}` routes to `{name...}` or redesign the single-resource API.
- Preserve existing managed/custom matching and runner lifecycle behavior.
- Do not commit or push without a separate user request.

---

### Task 1: State and HTTP rejection contract

**Files:**
- Modify: `internal/state/catalog.go`
- Test: `internal/state/store_extra_test.go`
- Test: `internal/state/store_test.go`
- Test: `internal/server/server_test.go`

**Interfaces:**
- Produces: `validateProfileName(name string) error`, shared by `UpsertProfile` and `ReconcileManagedProfiles` after trimming.
- Preserves: `ListProfiles`, `GetProfile`, `MatchProfile`, migration, and audited mutation signatures.

- [x] **Step 1: Write failing state write-boundary tests**

Add a table test that calls `UpsertProfile` with `owner/spec`, `.`, and `..`, expects an error containing `profile name must not contain '/' or be '.' or '..'`, and proves `GetProfile` returns `ErrNotFound`. Add a reconciliation test using the same invalid names and prove no profile or reconciliation audit event is committed.

- [x] **Step 2: Write the failing historical-row compatibility test**

Create a valid custom profile, rename its primary key directly to `legacy/unsafe` through GORM to model a pre-fix database, reopen the SQLite store with migration enabled, reconcile a legal managed profile, and assert `GetProfile`, `ListProfiles`, and `MatchProfile` still return the historical custom profile without renaming or deleting it.

- [x] **Step 3: Write the failing HTTP atomic rejection test**

POST a complete custom Runner Spec payload for each invalid name. Assert `400 Bad Request`, the stable validation text, `state.ErrNotFound` from `GetProfile`, and zero committed `profile.create` audit events.

- [x] **Step 4: Run RED tests**

Run:

```bash
go test ./internal/state -run 'Test(UpsertProfileRejectsPathUnsafeName|ReconcileManagedProfilesRejectsPathUnsafeName|LegacyPathUnsafeProfileRemainsReadableAndMatchable)' -count=1
go test ./internal/server -run TestCreateProfileRejectsPathUnsafeNameAtomically -count=1
```

Expected: failures show current writes accept path-unsafe names.

- [x] **Step 5: Implement the minimal shared validator**

Add this behavior in `internal/state/catalog.go` and call it from both write paths after `strings.TrimSpace`:

```go
func validateProfileName(name string) error {
	if strings.Contains(name, "/") || name == "." || name == ".." {
		return fmt.Errorf("profile name must not contain '/' or be '.' or '..'")
	}
	return nil
}
```

Keep the existing empty-name error and do not invoke the helper from reads or migrations.

- [x] **Step 6: Run GREEN tests**

Run the two commands from Step 4 and confirm all named tests pass.

---

### Task 2: Admin UI error-path regression

**Files:**
- Test: `ui/src/hooks/use-runner-catalog.test.js`
- Modify only if needed for an observable test boundary: `ui/src/hooks/use-runner-catalog.ts`

**Interfaces:**
- Consumes: backend validation errors already thrown by `App.requestResponse`.
- Preserves: `submitRunnerSpecChanges` and `useRunnerCatalog` behavior.

- [x] **Step 1: Add a real save-error regression**

Exercise the save orchestration with a request that rejects using the backend validation error. Assert the error message is shown, the success notification is absent, the dialog close callback is not invoked, and catalog reload is not invoked. Prefer mounting a small real React hook harness with `happy-dom`; extract a production orchestration function only if the hook cannot be exercised without framework-only assertions.

- [x] **Step 2: Run the focused UI test**

Run:

```bash
cd ui && bun test src/hooks/use-runner-catalog.test.js
```

Expected: the regression passes against the existing error path, or fails only at the smallest missing observable boundary that is then implemented without adding client-only validation.

---

### Task 3: Durable contract documentation and verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `.agents/rules/project-architecture.md`
- Modify: `.agents/rules/testing-and-verification.md`
- Modify: `docs/testing.md`
- Modify: `docs/zh/testing.md`

**Interfaces:**
- Documents the same three invalid-name cases and historical-row compatibility; introduces no new runtime interface.

- [x] **Step 1: Document the state and API contract**

Add concise durable rules to `AGENTS.md` and `.agents/rules/`, and add paired English/Chinese notes near the custom Runner Spec POST examples in testing docs. State that old rows remain readable/matchable and are not migrated automatically.

- [x] **Step 2: Run focused and full verification**

Run:

```bash
go test ./internal/state -count=1
go test ./internal/server -count=1
cd ui && bun run test
go test ./...
task ui-i18n-check
GOTOOLCHAIN=go1.26.3 task lint
GOTOOLCHAIN=go1.26.3 task test
git diff --check
```

- [x] **Step 3: Audit the final diff against Issue #85**

Confirm every acceptance item has direct test or source evidence, inspect `git diff --stat` and `git diff`, and verify no generated UI assets, local database, secret, or unrelated file entered the diff.
