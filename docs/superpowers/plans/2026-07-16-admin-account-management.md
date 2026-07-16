# Admin Account Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an admin account list that supports search, role filtering, pagination, and audited role changes.

**Architecture:** Extend the existing DB-backed identity store with portable account queries and an audited transactional role-update method. Expose those operations through explicit role-gated `/admin/api/accounts` routes, then render them in a self-contained React admin section that follows the existing sidebar, card, table, and request patterns.

**Tech Stack:** Go 1.24, GORM, `net/http`, React 19, TypeScript, Tailwind CSS, Radix UI, Bun tests.

## Global Constraints

- Keep role truth on local `accounts`; OAuth provider, subject, and login remain identity metadata.
- Support sqlite, PostgreSQL, and MySQL without backend-specific query syntax.
- Edit UI source under `ui/`; regenerate `internal/server/ui/` only through `task ui-build` or `task build`.
- Do not modify the signed-in administrator's own role through the admin API.
- Keep at least one administrator after every committed role change.
- Do not commit unless the user explicitly asks.

---

### Task 1: Account store queries and role updates

**Files:**
- Modify: `internal/state/store.go`
- Modify: `internal/state/identity_audit.go`
- Test: `internal/state/store_test.go`

**Interfaces:**
- Produces: `AccountListOptions`, `AccountListItem`, `AccountRoleUpdate`, `GetAccount(int64) (Account, error)`, `ListAccounts(AccountListOptions) ([]AccountListItem, int64, error)`, and `UpdateAccountRoleWithAudit(AccountRoleUpdate) (Account, error)`.
- Consumes: existing `accountRecord`, `oauthIdentityRecord`, `normalizePlatformRole`, and `ErrNotFound`.

- [ ] **Step 1: Write failing store tests**

```go
func TestListAccountsSearchesFiltersAndPaginates(t *testing.T) {
	store := New(t.TempDir())
	// Create admin/user identities, link a second provider, then assert role
	// filtering, case-insensitive login search, deterministic pagination,
	// identity aggregation, and the filtered total.
}

func TestUpdateAccountRoleValidatesAndPersists(t *testing.T) {
	store := New(t.TempDir())
	// Assert user -> admin, invalid role rejection, and missing account handling.
}
```

- [ ] **Step 2: Run the tests and verify RED**

Run: `go test ./internal/state -run 'Test(ListAccounts|UpdateAccountRole)' -count=1`

Expected: compilation fails because the new store API does not exist.

- [ ] **Step 3: Implement minimal portable store behavior**

```go
type AccountListOptions struct {
	Query  string
	Role   string
	Limit  int
	Offset int
}

type AccountListItem struct {
	Account
	OAuthIdentities []OAuthIdentity `json:"oauth_identities"`
}

func (s *DBStore) ListAccounts(options AccountListOptions) ([]AccountListItem, int64, error)
func (s *DBStore) UpdateAccountRoleWithAudit(update AccountRoleUpdate) (Account, error)
```

Use a correlated `EXISTS` query over `oauth_identities` for search, order accounts by `updated_at DESC, id DESC`, fetch identities for the selected page, and normalize roles through `normalizePlatformRole`. Run the role update and audit insert in one serializable transaction, verify the actor is still an administrator, and reject a demotion that would leave no administrators.

- [ ] **Step 4: Run the focused state tests and verify GREEN**

Run: `go test ./internal/state -run 'Test(ListAccounts|UpdateAccountRole)' -count=1`

Expected: PASS.

### Task 2: Role-gated admin account API

**Files:**
- Create: `internal/server/server_admin_accounts.go`
- Modify: `internal/server/server.go`
- Test: `internal/server/server_test.go`

**Interfaces:**
- Consumes: store APIs from Task 1, `parsePagination`, `writePaginationHeaders`, and `adminSessionFromRequest`.
- Produces: `GET /admin/api/accounts` and `PATCH /admin/api/accounts/{id}/role`.

- [ ] **Step 1: Write failing HTTP tests**

```go
func TestAdminAccountsRequiresAdmin(t *testing.T) {}
func TestAdminAccountsSearchFilterAndPagination(t *testing.T) {}
func TestAdminAccountRoleUpdateIsAudited(t *testing.T) {}
func TestAdminAccountRoleUpdateRejectsCurrentAccount(t *testing.T) {}
```

The list test asserts `accounts`, `current_account_id`, `total`, `limit`, and `offset`. The update test asserts the persisted role and an `account.role.update` audit event.

- [ ] **Step 2: Run the tests and verify RED**

Run: `go test ./internal/server -run 'TestAdminAccount' -count=1`

Expected: requests return 404 because the routes do not exist.

- [ ] **Step 3: Implement handlers and routes**

```go
s.mux.HandleFunc("GET /admin/api/accounts", s.handleAdminListAccounts)
s.mux.HandleFunc("PATCH /admin/api/accounts/{id}/role", s.handleAdminUpdateAccountRole)
```

The list handler accepts `q`, `role`, `limit`, and `offset`. The patch body is `{"role":"admin"}` or `{"role":"user"}`. Resolve the signed-in account, reject its own ID with HTTP 409, and call the atomic store operation with the stable OAuth provider and subject as the audit actor. The audit event contains only old/new authorization metadata.

- [ ] **Step 4: Run focused server tests and verify GREEN**

Run: `go test ./internal/server -run 'TestAdminAccount' -count=1`

Expected: PASS.

### Task 3: Admin accounts UI

**Files:**
- Create: `ui/src/components/accounts-section.tsx`
- Create: `ui/src/components/accounts-section-utils.ts`
- Create: `ui/src/components/accounts-section-utils.test.js`
- Modify: `ui/src/admin-types.ts`
- Modify: `ui/src/components/app-sidebar.tsx`
- Modify: `ui/src/App.tsx`

**Interfaces:**
- Consumes: `request(url, options)`, admin account API response, existing `Card`, `Table`, `Select`, `Dialog`, `Badge`, `Button`, and `Input` primitives.
- Produces: the `/admin/accounts` section with search, role filter, 10/20/50 page sizes, previous/next navigation, and confirmed role changes.

- [ ] **Step 1: Write failing UI utility tests**

```js
test("builds account list query parameters", () => {
  expect(accountListQuery({ query: " Octo ", role: "admin", limit: 20, offset: 40 }))
    .toBe("q=Octo&role=admin&limit=20&offset=40")
})

test("calculates bounded page metadata", () => {
  expect(accountPageMeta(41, 20, 40)).toEqual({ page: 3, pages: 3, canPrevious: true, canNext: false })
})
```

- [ ] **Step 2: Run the test and verify RED**

Run: `cd ui && bun test src/components/accounts-section-utils.test.js`

Expected: module import fails because the utility does not exist.

- [ ] **Step 3: Implement utilities and the UI section**

```ts
export function accountListQuery(input: AccountListQuery): string
export function accountPageMeta(total: number, limit: number, offset: number): AccountPageMeta
export function AccountsSection({ request }: { request: RequestFunction })
```

Use an explicit search form, a role filter, identity chips, a protected “You” row, a confirmation dialog for privilege changes, accessible button labels, a responsive table wrapper, and empty/error/loading states. Add `accounts` to `adminSections`, the sidebar, and the `App` section renderer.

- [ ] **Step 4: Run UI tests and static verification**

Run: `cd ui && bun test src/components/accounts-section-utils.test.js`

Run: `task ui-lint`

Expected: PASS.

### Task 4: Documentation, full verification, and UI smoke

**Files:**
- Modify: `README.md`
- Modify: `docs/testing.md`
- Modify: `docs/zh/testing.md`
- Regenerate: `internal/server/ui/`

**Interfaces:**
- Documents: `/admin/accounts`, `GET /admin/api/accounts`, and `PATCH /admin/api/accounts/{id}/role`.

- [ ] **Step 1: Document the shipped admin surface**

Add concise English and Chinese testing examples for list/search/filter/pagination and role update. State that admins cannot change their own role through this API and that successful changes are audited.

- [ ] **Step 2: Format and run focused tests**

Run: `gofumpt -w internal/state/store.go internal/state/identity_audit.go internal/state/store_test.go internal/server/server.go internal/server/server_admin_accounts.go internal/server/server_test.go`

Run: `go test ./internal/state ./internal/server -count=1`

Expected: PASS.

- [ ] **Step 3: Run full repository gates**

Run: `task lint`

Run: `task test`

Run: `task build`

Run: `git diff --check`

Expected: all commands exit 0.

- [ ] **Step 4: Run the real development stack and browser smoke**

Start `task dev` with the existing valid local config and non-conflicting ports. Verify `/healthz`, `/admin/accounts`, search, admin/user filters, pagination, role confirmation, persistence, and audit visibility. Stop only the process started for this verification.

### Task 5: Final code review

**Files:**
- Review all files changed by Tasks 1-4.

**Interfaces:**
- Consumes: the user requirements and the code-review checklist.
- Produces: no unresolved critical or important finding.

- [ ] **Step 1: Inspect the complete diff**

Run: `git diff --check && git diff --stat && git diff`

- [ ] **Step 2: Review requirement and security coverage**

Confirm admin-only routing, strict role validation, parameterized search, pagination bounds, current-account protection, audit logging, UI error states, and documentation accuracy.

- [ ] **Step 3: Fix findings with RED/GREEN tests**

For every behavioral finding, add a failing regression test, observe the expected failure, implement the narrow fix, and rerun the focused suite.

- [ ] **Step 4: Repeat the final verification gate**

Run: `task lint && task test && task build && git diff --check`

Expected: exit 0 with no unresolved review findings.
