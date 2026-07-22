# Homepage Performance Phase One Implementation Plan

> **For Codex:** Execute this plan in order with test-driven development and verify the local development stack before completion.

**Goal:** Remove avoidable homepage request contention without changing the database schema or migrating business data.

**Architecture:** Make UI data loading route-aware so only the visible surface polls dynamic data. Keep API response shapes compatible while bounding runner-history reads with pagination headers, and project only fields required by `RunnerState`. Preserve the exact GitHub installation/repository authorization intersection and its existing 30-second maximum cache age, but refresh valid entries before expiry outside the request critical path. Add low-cardinality route-pattern HTTP timing counters to existing expvar diagnostics.

**Tech Stack:** React 19, TypeScript, Bun tests, Go `net/http`, GORM, expvar, SQLite/Postgres/MySQL-compatible queries.

---

### Task 1: Route-aware UI loading and polling

**Files:**
- Create: `ui/src/app-load-policy.ts`
- Create: `ui/src/app-load-policy.test.js`
- Modify: `ui/src/App.tsx`

1. Add failing table-driven Bun tests for the resources required by each admin section and user route, plus which routes may poll.
2. Run `cd ui && bun test src/app-load-policy.test.js` and confirm the new tests fail because the policy module is missing.
3. Implement pure load-policy helpers: overview loads requests/specs/policies; catalog pages load only their dependencies; audit loads only audit events; user Jobs loads GitHub App metadata and runner requests; account pages load GitHub App metadata and preferences; only Jobs and dynamic admin request surfaces poll.
4. Replace the unconditional `loadAll` and `loadUserAll` five-second effects with route-gated loaders. Keep mutation refresh callbacks scoped to the active admin section.
5. Run the focused policy test and `cd ui && bun run build`.

### Task 2: Lightweight runner list projection and authorized pagination

**Files:**
- Modify: `internal/state/store.go`
- Modify: `internal/state/runner_requests.go`
- Modify: `internal/state/store_test.go`

1. Add failing state tests that request an offset page through exact `(github_installation_id, repository_full_name)` pairs, verify global order and total count across query batches, and assert the list projection excludes `github_payload_json`, labels JSON, and encrypted Sandbox credentials while retaining all public `RunnerState` fields.
2. Run `go test ./internal/state -run 'TestListStates(Page|ForGitHubInstallationRepositories)' -count=1` and confirm the new pagination/projection tests fail.
3. Add a shared `RunnerState` list projection and apply it to admin and authorized user list queries.
4. Change the authorized list store method to accept `limit` and `offset` and return a total. Normalize access pairs before parameter-safe batching so batch counts are disjoint, fetch enough rows per batch for a globally ordered page, then slice after merge.
5. Update internal callers that need the full bounded working set to use offset zero and ignore the total.
6. Run `go test ./internal/state -count=1`.

### Task 3: User API pagination and request-duration metrics

**Files:**
- Modify: `internal/server/server_user_handlers.go`
- Modify: `internal/server/server_terminal.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/server_test.go`
- Modify: `internal/metrics/metrics.go`
- Modify: `internal/metrics/metrics_test.go`

1. Add failing handler tests for default user page size, explicit `limit/offset`, `X-Total-Count`, `X-Limit`, `X-Offset`, and `Link`, while retaining repository-pair authorization.
2. Add a failing metrics test for route-pattern request count and cumulative duration keys.
3. Run the focused server and metrics tests and confirm the additions fail.
4. Reuse the existing pagination parser and headers in `GET /user/runner_requests`, with the same default 100 and maximum 500 as the admin endpoint. Bound ordinary-user offset pagination to the most recent 500 rows and stop `Link` at that window.
5. Record HTTP request count and cumulative milliseconds after mux dispatch using `r.Pattern`, method, and status, falling back to `unmatched` to avoid path-ID cardinality.
6. Run `go test ./internal/server ./internal/metrics -count=1`.

### Task 4: Non-blocking permission cache refresh

**Files:**
- Modify: `internal/server/server.go`
- Modify: `internal/server/server_user_authorization.go`
- Modify: `internal/server/server_test.go`

1. Add failing tests proving a still-valid cache entry returns immediately after its refresh threshold, only one background refresh runs, successful refresh replaces access, and expired/missing entries still fail closed on rejected or unavailable OAuth credentials.
2. Run the focused authorization-cache tests and confirm the background-refresh test fails.
3. Track `refreshAfter`, `expiresAt`, and refresh-in-flight state per account. Keep expiry at 30 seconds, trigger one background refresh after 20 seconds, and use singleflight for cold/expired refreshes.
4. On invalid credentials, invalidate the cached authorization; on transient errors, keep only the still-valid entry until its existing expiry.
5. Run `go test ./internal/server -run 'TestUserRunnerAuthorization' -count=1`.

### Task 5: Contract documentation and verification

**Files:**
- Modify: `README.md`
- Modify: `README.zh.md`
- Modify: `docs/testing.md`
- Modify: `docs/zh/testing.md`

1. Document user runner-list pagination headers and the route-aware polling behavior in English and Chinese.
2. Run formatting: `gofmt` on changed Go files and the repository UI formatter/lint path where available.
3. Run focused verification: `cd ui && bun run test`, `cd ui && bun run build`, `go test ./internal/state ./internal/server ./internal/metrics -count=1`.
4. Run `task lint` and `task test`; distinguish any unrelated baseline failure rather than weakening coverage.
5. Follow `.agents/skills/runnerd-dev-smoke/SKILL.md`: inspect local prerequisites, start `task dev`, verify `/healthz`, Vite-proxied `/`, and relevant paginated APIs, then stop only processes started by this task.
6. Review `git diff --check`, `git status --short`, and the final diff. Confirm no migration, schema, hand-edited generated UI asset, secret, local database, or local configuration changes are present.
