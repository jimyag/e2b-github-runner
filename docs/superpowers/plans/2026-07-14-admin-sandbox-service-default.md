# Admin Sandbox Service Default Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an admin-managed global Sandbox service default that runner jobs and ordinary-user catalogs use only when their installation/account scope has no complete Sandbox service configuration.

**Architecture:** Store the global default in a dedicated singleton `sandbox_service_defaults` table instead of pretending it belongs to an account. Keep account and organization Preferences as the first-choice credential sources, add the admin default at the end of the resolver, and snapshot both encrypted credentials and the selected source onto each runner request so lifecycle cleanup remains stable after settings change.

**Tech Stack:** Go 1.24, `net/http`, GORM, sqlite/Postgres/MySQL, React 19, TypeScript, Vite, Tailwind CSS, shadcn-style components.

## Execution Status

Implemented on `codex/admin-sandbox-service-default` on 2026-07-14. State migration, admin API, runtime resolution, source snapshots, admin/account UI, documentation, focused tests, `task test`, and `task lint` are complete. The commit steps remain intentionally deferred because no commit or push was requested. The route and health checks pass locally, but authenticated browser interaction remains pending because the local runnerd instance requires GitHub OAuth and has no OAuth configuration.

## Global Constraints

- Resolution order is request snapshot, GitHub installation custom/inherited config, eligible personal-account config, enabled admin default, then `sandbox service not configured`.
- An explicit account or installation config wins over the admin default.
- A corrupt preference, invalid encrypted value, or decrypt failure is an error; it must not silently switch billing identity.
- A missing preference, endpoint, or API key is incomplete configuration and may use the enabled admin default.
- The admin default is disabled by default and preserves its endpoint/key while disabled.
- API keys are encrypted with `auth.encryption_key`, never returned by APIs, and never written to audit payloads or logs.
- `/user/sandbox/templates` and `/user/sandbox/instances` remain ordinary-user APIs and use the same effective resolver as runner creation.
- Admin management uses explicit role-gated endpoints under `/admin/api/`; it does not expose provider resource catalogs.
- New state columns/tables must migrate through GORM on sqlite, Postgres, and MySQL; no handwritten migration is needed for a new optional table and nullable runner source column.
- Do not hand-edit generated files under `internal/server/ui/`.

## Design Decision

Three persistence options were considered:

1. Reuse `account_preferences` with `scope_type=system`: smallest diff, but weakens the account ownership model and makes future authorization mistakes easier.
2. Add generic `system_preferences` and `system_secrets`: reusable, but introduces a framework before a second system setting exists.
3. Add a dedicated singleton `sandbox_service_defaults` record: explicit ownership, compact API, and no fake account identity.

Use option 3. The singleton has fixed ID `1`, `enabled`, `api_url`, encrypted API key, API-key update timestamp, and ordinary created/updated timestamps.

---

### Task 1: Persist the global default and runner config source

**Files:**
- Create: `internal/state/sandbox_service_defaults.go`
- Modify: `internal/state/records.go`
- Modify: `internal/state/store.go`
- Modify: `internal/state/db.go`
- Modify: `internal/state/conversions.go`
- Modify: `internal/state/runner_requests.go`
- Test: `internal/state/store_test.go`

**Interfaces:**
- Produces: `state.SandboxServiceDefault`, `GetSandboxServiceDefault()`, `UpsertSandboxServiceDefault(defaultConfig)`, and `DeleteSandboxServiceDefaultAPIKey()`.
- Produces: `RunnerRequest.SandboxConfigSource` and `RunnerState.SandboxConfigSource` persisted as `sandbox_config_source`.

- [ ] **Step 1: Write failing store tests**

Add tests that create a fresh store, verify `GetSandboxServiceDefault()` initially returns `state.ErrNotFound`, upsert ID `1`, update it while preserving an encrypted key, clear only the key, and round-trip `SandboxConfigSource` through `CreateRequest`, `WriteState`, `ReadRequest`, and `ReadState`.

```go
saved, err := store.UpsertSandboxServiceDefault(state.SandboxServiceDefault{
	Enabled:             true,
	APIURL:              "https://sandbox.example.test",
	APIKeyEncrypted:     "encrypted-key",
	APIKeyUpdatedAt:     &now,
})
if err != nil || saved.ID != 1 || !saved.Enabled {
	t.Fatalf("save sandbox default: saved=%#v err=%v", saved, err)
}
```

- [ ] **Step 2: Run the focused state tests and confirm failure**

Run: `go test ./internal/state -run 'TestSandboxServiceDefault|TestRunnerRequestSandboxConfigSource' -count=1`

Expected: compile failure because the state type and store methods do not exist.

- [ ] **Step 3: Add the model and store API**

Define the public state value and singleton record:

```go
type SandboxServiceDefault struct {
	ID                  int64      `json:"id"`
	Enabled             bool       `json:"enabled"`
	APIURL              string     `json:"api_url"`
	APIKeyEncrypted     string     `json:"-"`
	APIKeyUpdatedAt     *time.Time `json:"api_key_updated_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}
```

Implement one-row upsert with ID forced to `1`. Use an update map when clearing the encrypted key so GORM does not skip zero values. Add `sandboxServiceDefaultRecord` to `AutoMigrate`; do not add a legacy SQL backfill because this is a new table.

- [ ] **Step 4: Persist the selected source on runner requests**

Add `SandboxConfigSource string` to the record and public request/state types, conversion functions, create path, state update map, and retry reset path. A manual retry already clears the saved endpoint/key so it can resolve current settings; clear the saved source in the same transaction. Keep the JSON field visible as `sandbox_config_source,omitempty`; the encrypted key and endpoint remain hidden.

- [ ] **Step 5: Run state compatibility tests**

Run: `go test ./internal/state -count=1`

Expected: PASS, including existing old sqlite upgrade coverage.

- [ ] **Step 6: Commit the state unit**

```bash
git add internal/state
git commit -m "feat: persist sandbox service default"
```

### Task 2: Add role-gated admin API and audit events

**Files:**
- Create: `internal/server/server_admin_sandbox_service.go`
- Modify: `internal/server/server.go`
- Test: `internal/server/server_test.go`

**Interfaces:**
- Produces: `GET /admin/api/sandbox-service-default`.
- Produces: `PUT /admin/api/sandbox-service-default` with `{enabled, api_url, api_key?}`.
- Produces: `DELETE /admin/api/sandbox-service-default/api-key`.
- Produces audit actions `sandbox_default.configure` and `sandbox_default.api_key.delete` on resource `sandbox_service_default/global`.

- [ ] **Step 1: Write failing handler tests**

Cover unauthenticated `401`, empty `GET`, create/update, key preservation when `api_key` is omitted, disabled persistence, enabled-without-complete-config rejection, invalid URL rejection, delete-key idempotence, encrypted-at-rest verification, and audit payloads that omit the key.

```go
req := adminRequest(http.MethodPut, "/admin/api/sandbox-service-default", bytes.NewBufferString(`{
  "enabled": true,
  "api_url": "https://sandbox.example.test",
  "api_key": "secret-value"
}`))
rec := httptest.NewRecorder()
srv.mux.ServeHTTP(rec, req)
if rec.Code != http.StatusOK {
	t.Fatalf("PUT default: status=%d body=%s", rec.Code, rec.Body.String())
}
```

- [ ] **Step 2: Run handler tests and confirm failure**

Run: `go test ./internal/server -run 'TestAdminSandboxServiceDefault' -count=1`

Expected: `GET` is handled by the admin UI fallback or returns a non-API response because routes do not exist.

- [ ] **Step 3: Implement request/response contracts**

Use a response that never serializes ciphertext:

```go
type adminSandboxServiceDefaultResponse struct {
	Enabled    bool `json:"enabled"`
	Configured bool `json:"configured"`
	APIURL     string `json:"api_url"`
	APIKey     struct {
		Configured bool   `json:"configured"`
		UpdatedAt  string `json:"updated_at,omitempty"`
	} `json:"api_key"`
}
```

Normalize `api_url` with the existing `normalizeHTTPURL`. Preserve the existing encrypted key when `api_key` is omitted, encrypt a supplied key with `auth.encryption_key`, and require both URL and key before saving `enabled=true`.

- [ ] **Step 4: Register explicit admin routes and record audits**

Register the three method-qualified paths before relying on the `GET /admin/` UI handler. Authenticate via `adminSessionFromRequest` so the audit actor is `github:<subject>`.

- [ ] **Step 5: Run focused and package tests**

Run: `go test ./internal/server -run 'TestAdminSandboxServiceDefault' -count=1`

Expected: PASS.

Run: `go test ./internal/server -count=1`

Expected: PASS.

- [ ] **Step 6: Commit the admin API unit**

```bash
git add internal/server/server.go internal/server/server_admin_sandbox_service.go internal/server/server_test.go
git commit -m "feat: add admin sandbox default api"
```

### Task 3: Resolve the admin default after scoped credentials

**Files:**
- Modify: `internal/server/server_sandbox_service.go`
- Modify: `internal/server/server_sandbox_catalog.go`
- Modify: `internal/server/server_runner_lifecycle.go`
- Modify: `internal/server/server_user_handlers.go`
- Test: `internal/server/server_test.go`

**Interfaces:**
- Consumes: `state.GetSandboxServiceDefault()` from Task 1.
- Produces: resolver source values `installation`, `account`, `inherited_account`, `admin_default`, and `request_snapshot`.
- Produces: `UserPreferences.sandbox.resolved_source` without exposing the admin endpoint or key.

- [ ] **Step 1: Write failing resolver tests**

Add table-driven coverage for:

- enabled default used when installation and personal account are unconfigured;
- organization installation uses the default without falling back to an unrelated personal account;
- account/installation config overrides the default;
- inherited account config overrides the default;
- disabled or incomplete default returns `errSandboxServiceNotConfigured`;
- malformed JSON or decrypt failure does not fall through;
- request snapshot continues using its saved source after the global default changes;
- user template/instance catalogs use the effective default;
- the Preferences response reports `admin_default` only when it is the effective source.

```go
svc, snapshot, err := srv.sandboxServiceAndConfigForRunnerRequest(state.RunnerRequest{
	ID:                   "req-1",
	GitHubInstallationID: 987,
})
if err != nil || svc == nil || snapshot.Source != sandboxConfigSourceAdminDefault {
	t.Fatalf("resolve admin default: service=%T snapshot=%#v err=%v", svc, snapshot, err)
}
```

- [ ] **Step 2: Run resolver tests and confirm failure**

Run: `go test ./internal/server -run 'TestSandboxService.*AdminDefault|TestSandboxCatalog.*AdminDefault|TestUserPreferences.*AdminDefault' -count=1`

Expected: FAIL because no global fallback exists.

- [ ] **Step 3: Extend the snapshot and exact-scope resolver**

Add `Source string` to `sandboxServiceConfigSnapshot`. Keep `sandboxServiceForScope` limited to its exact account/installation semantics and mark inherited recursion as `inherited_account`.

- [ ] **Step 4: Add one global resolver boundary**

Implement `sandboxServiceForAdminDefault()` and a helper that calls it only for `errors.Is(err, errSandboxServiceNotConfigured)`. Treat absent endpoint/key as not configured; return decode/decrypt errors directly.

For runner requests, try exact installation, then the existing eligible personal account lookup, then the global default. For ordinary-user catalogs, try the requested account/installation scope and then the global default.

- [ ] **Step 5: Snapshot and expose the selected source**

When runner startup saves `SandboxAPIURL` and `SandboxAPIKeyEncrypted`, also save `SandboxConfigSource`. When reading an older snapshot with an empty source, label it `request_snapshot` without rewriting history.

Add `resolved_source` to ordinary-user Preferences. It may report `custom`, `inherited`, `admin_default`, or `none`; it must not return global endpoint/key metadata.

- [ ] **Step 6: Run server tests**

Run: `go test ./internal/server -count=1`

Expected: PASS, including `TestSandboxServiceDoesNotFallBackToAccountForOrgInstallation`.

- [ ] **Step 7: Commit the resolver unit**

```bash
git add internal/server internal/state
git commit -m "feat: fall back to admin sandbox default"
```

### Task 4: Add the admin management page

**Files:**
- Create: `ui/src/components/sandbox-service-default-section.tsx`
- Modify: `ui/src/admin-types.ts`
- Modify: `ui/src/components/app-sidebar.tsx`
- Modify: `ui/src/App.tsx`
- Modify: `ui/src/components/runner-requests-section.tsx`

**Interfaces:**
- Consumes: Task 2 admin API.
- Consumes: `RunnerState.sandbox_config_source` from Task 1.
- Produces: admin route `/admin/sandbox-service`.

- [ ] **Step 1: Add frontend types and route**

Add `SandboxServiceDefault` and `sandbox_service` to `AdminSection`. Add a `CloudCog` sidebar item labeled `Sandbox Service` and render the new section only for that route.

- [ ] **Step 2: Build the management form**

Create one un-nested settings surface with:

- an `Enable fallback for unconfigured accounts` switch;
- the existing Sandbox region select plus a custom endpoint input;
- a password input whose placeholder distinguishes configured from missing;
- `Save settings` and icon-backed `Remove API key` commands;
- a confirmation dialog before removing the key;
- status text for `Enabled`, `Disabled`, and `Incomplete`.

On first render fetch `GET /admin/api/sandbox-service-default`; save via `PUT`; delete via the dedicated `DELETE` endpoint. Never place the saved key in React state.

- [ ] **Step 3: Show resolver source on runner details**

Map source values to `GitHub installation`, `Account`, `Inherited account`, `Admin default`, and `Saved request snapshot`, then add a compact `Sandbox config` detail row. Do not show the endpoint or key.

- [ ] **Step 4: Verify UI type and lint checks**

Run: `task ui-lint`

Expected: PASS.

- [ ] **Step 5: Commit the admin UI unit**

```bash
git add ui/src
git commit -m "feat: manage sandbox default in admin"
```

### Task 5: Make fallback visible to ordinary users

**Files:**
- Modify: `ui/src/admin-types.ts`
- Modify: `ui/src/components/user-dashboard.tsx`
- Test: `internal/server/server_test.go`

**Interfaces:**
- Consumes: `UserPreferences.sandbox.resolved_source` from Task 3.
- Produces: a non-secret runtime source label in account and organization Preferences.

- [ ] **Step 1: Add response assertions**

Assert that an unconfigured account gets `resolved_source: "admin_default"` only while the global default is enabled and complete, while configured/inherited scopes retain their own source.

- [ ] **Step 2: Render effective-source copy**

When `resolved_source === "admin_default"`, show `Using the platform Sandbox service default.` below the account/organization settings status. Keep local fields empty so the global key is never mistaken for an account-owned credential.

Update the delete confirmation copy: removing an account key may cause jobs to use the platform default when enabled, rather than always claiming jobs cannot start.

- [ ] **Step 3: Verify UI and server tests**

Run: `go test ./internal/server -count=1`

Expected: PASS.

Run: `task ui-lint`

Expected: PASS.

- [ ] **Step 4: Commit the ordinary-user visibility unit**

```bash
git add internal/server/server_test.go ui/src
git commit -m "feat: show effective sandbox config source"
```

### Task 6: Document and verify the complete behavior

**Files:**
- Modify: `README.md`
- Modify: `docs/testing.md`
- Modify: `docs/zh/testing.md`
- Modify: `docs/deployment-smoke.md`
- Modify: `docs/zh/deployment-smoke.md`
- Modify: `docs/runner-architecture-comparison.md`
- Modify: `docs/zh/runner-architecture-comparison.md`
- Modify: `.agents/rules/project-architecture.md`
- Modify: `.agents/rules/testing-and-verification.md`

**Interfaces:**
- Documents: ownership, precedence, admin route/API, encryption, disabled-by-default behavior, and verification expectations.

- [ ] **Step 1: Update operator and architecture docs**

Document this exact precedence:

```text
saved runner request snapshot
  -> installation custom/inherited configuration
  -> eligible personal account configuration
  -> enabled admin Sandbox service default
  -> not configured error
```

Clarify that ordinary-user catalogs remain user APIs, while `/admin/sandbox-service` configures only the platform fallback.

- [ ] **Step 2: Update deployment smoke checks**

Cover two paths: a scoped credential overrides the global default, and an unconfigured test account successfully uses the enabled global default. Include a disabled-default negative check.

- [ ] **Step 3: Run state, server, UI, and full repository checks**

Run:

```bash
go test ./internal/state -count=1
go test ./internal/server -count=1
task ui-lint
go test ./...
task test
git diff --check
```

Expected: all commands PASS.

- [ ] **Step 4: Run a local browser smoke**

Start `task dev`, sign in as admin, and verify `/admin/sandbox-service` at desktop and narrow mobile widths. Save a disabled config, enable it with a key, reload to confirm the key stays masked, remove the key, and confirm the page becomes `Incomplete` without layout overlap.

Then use an ordinary account without scoped credentials to verify `/account/preferences`, `/account/sandbox-templates`, and one organization route show/use `admin_default`; add a scoped key and verify it overrides the default.

- [ ] **Step 5: Commit documentation and verification updates**

```bash
git add README.md docs .agents/rules
git commit -m "docs: describe sandbox service default"
```

## Acceptance Criteria

- Admins can configure, disable, re-enable, and remove the global API key from `/admin/sandbox-service`.
- Non-admin API requests receive `401`; no user endpoint can modify or read the admin default metadata.
- The key is encrypted at rest and absent from JSON, logs, audit payloads, and UI state after save.
- Scoped custom and inherited credentials always win.
- Unconfigured account/organization runner jobs and catalogs use the default only while it is enabled and complete.
- Corrupt scoped credentials fail visibly instead of silently switching identity.
- Each runner request records and displays the effective config source while keeping its credential snapshot private.
- Existing organization isolation test behavior remains intact.
- State migration, server tests, UI checks, full tests, and browser smoke all pass.
