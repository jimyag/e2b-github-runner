# Admin Account Statistics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace runner-oriented summary cards on the Accounts page with global account, role, and linked-identity statistics.

**Architecture:** Count local accounts and their roles from `accounts`, and count provider bindings separately from `oauth_identities`. Return the unfiltered statistics alongside each admin account-list response so search, filtering, and pagination never change the summary-card totals. The Accounts section owns the contextual cards while `App` keeps runner cards on every other admin section.

**Tech Stack:** Go 1.24, GORM, `net/http`, React 19, TypeScript, Tailwind CSS, Bun tests.

## Global Constraints

- Account totals and role totals come from local `accounts`.
- Linked identity totals come from `oauth_identities` and may exceed the account total.
- Summary statistics are global and do not change with list query, role filter, pagination, or page size.
- Keep the existing four-column responsive metric-card layout and visual language.
- Do not commit unless the user explicitly asks.

---

### Task 1: Account statistics store and API response

**Files:**
- Modify: `internal/state/store.go`
- Modify: `internal/state/identity_audit.go`
- Test: `internal/state/store_test.go`
- Modify: `internal/server/server_admin_accounts.go`
- Test: `internal/server/server_test.go`

**Interfaces:**
- Produces: `AccountStats` and `GetAccountStats() (AccountStats, error)`.
- Extends: `GET /admin/api/accounts` with a `stats` object.

- [ ] **Step 1: Write failing store coverage**

```go
func TestGetAccountStatsCountsAccountsRolesAndIdentities(t *testing.T) {
	store := New(t.TempDir())
	if _, _, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "100", OAuthLogin: "alpha"}, "admin"); err != nil {
		t.Fatal(err)
	}
	user, _, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "200", OAuthLogin: "bravo"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LinkOAuthIdentityToAccount(user.ID, OAuthIdentity{OAuthProvider: "gitlab", OAuthSubject: "gl-200", OAuthLogin: "bravo"}); err != nil {
		t.Fatal(err)
	}
	stats, err := store.GetAccountStats()
	if err != nil {
		t.Fatal(err)
	}
	want := AccountStats{TotalAccounts: 2, AdminAccounts: 1, UserAccounts: 1, OAuthIdentities: 3}
	if stats != want {
		t.Fatalf("unexpected account stats: got=%#v want=%#v", stats, want)
	}
}
```

- [ ] **Step 2: Run the store test and verify RED**

Run: `go test ./internal/state -run TestGetAccountStats -count=1`

Expected: compilation fails because `AccountStats` and `GetAccountStats` do not exist.

- [ ] **Step 3: Implement the portable counters**

```go
type AccountStats struct {
	TotalAccounts   int64 `json:"total_accounts"`
	AdminAccounts   int64 `json:"admin_accounts"`
	UserAccounts    int64 `json:"user_accounts"`
	OAuthIdentities int64 `json:"oauth_identities"`
}

func (s *DBStore) GetAccountStats() (AccountStats, error)
```

Use parameterized GORM `Count` queries against `accountRecord` and `oauthIdentityRecord`; do not derive identities from the current page.

- [ ] **Step 4: Extend the existing HTTP test before the handler**

Add `Stats state.AccountStats` to the local response type in `TestAdminAccountsSearchFilterAndPagination`, then assert the filtered request still reports 3 accounts, 1 admin, 2 users, and 4 identities.

- [ ] **Step 5: Run the HTTP test and verify RED**

Run: `go test ./internal/server -run TestAdminAccountsSearchFilterAndPagination -count=1`

Expected: the decoded `stats` object contains zero values because the handler does not yet return it.

- [ ] **Step 6: Return statistics from the account-list handler**

Add `Stats state.AccountStats \`json:"stats"\`` to `adminAccountsResponse`, call `GetAccountStats`, and return a generic HTTP 500 if counting fails.

- [ ] **Step 7: Run focused state and server tests and verify GREEN**

Run: `go test ./internal/state -run 'Test(GetAccountStats|ListAccounts|UpdateAccountRole)' -count=1`

Run: `go test ./internal/server -run 'TestAdminAccount' -count=1`

Expected: PASS.

### Task 2: Contextual Accounts-page metric cards

**Files:**
- Modify: `ui/src/admin-types.ts`
- Modify: `ui/src/components/accounts-section.tsx`
- Test: `ui/src/components/accounts-section.test.js`
- Modify: `ui/src/App.tsx`

**Interfaces:**
- Consumes: `AdminAccountsResponse.stats`.
- Produces: four cards labeled `Accounts`, `Administrators`, `Users`, and `Linked identities`.

- [ ] **Step 1: Write the failing component assertion**

Render `AccountsSection` and assert all four account metric labels and their descriptions exist.

- [ ] **Step 2: Run the component test and verify RED**

Run: `cd ui && bun test src/components/accounts-section.test.js`

Expected: FAIL because the metric cards do not exist.

- [ ] **Step 3: Implement contextual metrics**

Add the `AdminAccountStats` response type, store the response statistics in `AccountsSection`, and render a responsive four-card grid before the account table. Hide the runner metric grid in `App` only while `section === "accounts"`. Reload account data after a role change so role cards update immediately.

- [ ] **Step 4: Run UI tests and static validation**

Run: `cd ui && bun test`

Run: `task ui-lint`

Expected: PASS.

### Task 3: Full verification and browser smoke

**Files:**
- Review all files changed by Tasks 1-2.

**Interfaces:**
- Verifies the shipped admin page at `/admin/accounts`.

- [ ] **Step 1: Run repository gates**

Run: `task lint`

Run: `task test`

Run: `task build`

Run: `git diff --check`

Expected: all commands exit 0.

- [ ] **Step 2: Verify the real page**

Reload `/admin/accounts` and confirm the top cards show account totals, no runner cards remain on that page, the account list still loads, and the browser console contains no errors.
