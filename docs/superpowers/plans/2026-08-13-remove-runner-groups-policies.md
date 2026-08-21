# Remove Runner Groups and Runner Policies Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the internal Runner Group and Runner Policy product models without requiring any existing GitHub Actions workflow change and without interrupting queued, running, retrying, or future runner requests.

**Architecture:** Keep Runner Spec label matching, ordering, GitHub runner registration, Sandbox credential resolution, and template resolution unchanged. First prove the existing policy-aware matcher and the policy-free matcher are equivalent for production traffic, then cut admission over to all enabled specs, retire the admin API/UI, and only remove obsolete database tables after a rollback-safe soak period.

**Tech Stack:** Go, GORM, sqlite/Postgres/MySQL, React, TypeScript, Bun, Vite, GitHub Actions webhooks, Qiniu Sandbox.

## Global Constraints

- Existing workflow `runs-on` labels must remain unchanged.
- Preserve `required_labels ⊆ job_labels ⊆ labels`, priority ordering, label-count tie-breaking, and name tie-breaking exactly.
- Preserve every Runner Spec's name, labels, required labels, template ID/default template name, GitHub `runner_group`, concurrency, idle capacity, priority, and enabled state.
- Do not change Sandbox Service credentials, precedence, audience, region selection, or provider-side template permissions.
- Do not modify or delete `runner_requests`; admitted requests already contain the selected spec and GitHub runner group.
- Do not drop `runner_groups`, `runner_group_specs`, or `repository_policies` in the behavioral cutover release.
- Keep the persisted `runner_profiles.default_available` column as a compatibility column until all supported database backends have a separately tested removal path.
- Keep `runner_group` in Runner Spec and Runner Request APIs because it means GitHub Organization Runner Group, not the internal Runner Group model being removed.
- Preserve the repository allowlist and exact GitHub App repository authorization checks.
- Treat production database contents and deployment state as current evidence; take a fresh snapshot immediately before each rollout.

---

## Verified Production Baseline (2026-08-13)

The production admin UI currently shows:

- 10 Runner Specs, all with `Default = Yes`.
- 5 managed specs: `qiniu-ubuntu-slim`, `qiniu-ubuntu-22.04`, `qiniu-ubuntu-24.04`, `qiniu-ubuntu-26.04`, and `qiniu-ubuntu-latest`.
- 5 custom specs: `github-runner-ubuntu-24-04`, `qbox-dora-ubuntu-16-04`, `qbox-dora-ubuntu-24-04`, `qbox-kodo-ubuntu-16-04`, and `qbox-kodo-web-ubuntu-20-04`.
- 1 enabled internal Group named `qbox/*`, containing the five custom specs.
- 7 enabled Policies; all target a spec directly and none targets the Group.
- `1024XEngineer/*`, `goplus/*`, and `qbox/*` target `github-runner-ubuntu-24-04`; the other four policies target qbox custom specs.

Because every production spec is already default-available, the current matcher admits the same enabled candidate set before it evaluates these seven policies. The Group and Policies are therefore redundant for current production matching. The implementation must still prove this with tests and shadow comparison rather than relying only on this snapshot.

## Final Runtime Contract

```text
GitHub workflow labels
        |
        v
all enabled Runner Specs
        |
        v
unchanged label subset matching and priority ordering
        |
        v
selected Runner Spec
        |
        v
unchanged scoped/default Sandbox credential resolution
        |
        v
public-name resolution or custom template ID creation
```

The four public managed templates remain visible to every user through a credential-independent public runner-template catalog. qbox custom templates remain governed by the effective Sandbox account/organization credentials and provider permissions. Removing Group/Policy must not move template authorization into Runner Spec.

## Rollout Sequence

1. **Release A — observe only:** add a policy-free matcher and compare it with the legacy matcher, but continue returning the legacy result.
2. **Release B — semantic cutover:** return the policy-free result, remove Group/Policy from the active admin UI, and keep rollback-compatible database data and temporary API shims.
3. **Release C — code removal:** remove Group/Policy types, store methods, handlers, routes, tests, and compatibility shims after the soak gates pass.
4. **Database cleanup window:** drop the three obsolete tables only after Release C is stable and the rollback window has expired. This is an explicit operator action, never an automatic startup migration.

---

### Task 1: Freeze the compatibility contract in tests

**Files:**
- Modify: `internal/state/store_test.go`
- Modify: `internal/server/server_test.go`
- Modify: `internal/server/server_default_templates_test.go`
- Create: `internal/state/testdata/runner-catalog-production-2026-08-13.json`

**Interfaces:**
- Consumes: current `DBStore.MatchProfile(repositoryFullName string, labels []string)` behavior.
- Produces: a production-shaped fixture and an explicit matrix of workflow labels to selected spec names.

- [ ] **Step 1: Add the production-shaped catalog fixture**

Store only non-secret matching fields. Include all ten spec names, labels, required labels, priority, enabled/default availability, the one Group membership list, and the seven direct Policies. Template IDs are not needed for matching and must be omitted from the fixture.

- [ ] **Step 2: Add a table-driven legacy compatibility test**

Cover at least these unchanged workflow label sets:

```go
tests := []struct {
	name       string
	repository string
	labels     []string
	wantSpec   string
}{
	{"managed slim canonical", "outside/example", []string{"qiniu", "ubuntu-slim"}, "qiniu-ubuntu-slim"},
	{"managed 2204 canonical", "outside/example", []string{"qiniu", "ubuntu-22.04"}, "qiniu-ubuntu-22.04"},
	{"managed 2404 canonical", "outside/example", []string{"qiniu", "ubuntu-24.04"}, "qiniu-ubuntu-24.04"},
	{"managed 2604 canonical", "outside/example", []string{"qiniu", "ubuntu-26.04"}, "qiniu-ubuntu-26.04"},
	{"managed latest canonical", "outside/example", []string{"qiniu", "ubuntu-latest"}, "qiniu-ubuntu-latest"},
	{"managed 2404 advertised", "outside/example", []string{"self-hosted", "linux", "x64", "qiniu", "ubuntu-24.04"}, "qiniu-ubuntu-24.04"},
	{"legacy generic custom", "qbox/example", []string{"self-hosted", "e2b"}, "github-runner-ubuntu-24-04"},
	{"legacy public custom", "goplus/example", []string{"self-hosted", "e2b", "github-runner-ubuntu-24-04"}, "github-runner-ubuntu-24-04"},
	{"legacy public custom outside policy", "outside/example", []string{"self-hosted", "e2b", "github-runner-ubuntu-24-04"}, "github-runner-ubuntu-24-04"},
	{"qbox dora 1604", "qbox/example", []string{"self-hosted", "e2b", "qbox-dora-ubuntu-16-04"}, "qbox-dora-ubuntu-16-04"},
	{"qbox dora 2404", "qbox/example", []string{"self-hosted", "e2b", "qbox-dora-ubuntu-24-04"}, "qbox-dora-ubuntu-24-04"},
	{"qbox kodo 1604", "qbox/example", []string{"self-hosted", "e2b", "qbox-kodo-ubuntu-16-04"}, "qbox-kodo-ubuntu-16-04"},
	{"qbox kodo web", "qbox/example", []string{"self-hosted", "e2b", "qbox-kodo-web-ubuntu-20-04"}, "qbox-kodo-web-ubuntu-20-04"},
}
```

- [ ] **Step 3: Add ordering and negative tests**

Assert that disabled specs remain ineligible, unmatched labels still return `profile_labels_not_matched`, and priority/label-count/name ordering remains byte-for-byte equivalent.

- [ ] **Step 4: Add queued-request and retry compatibility tests**

Create a request selected under the legacy matcher, restart the store through the migration path, and assert that `ProfileName`, `RunnerGroup`, `RequestedLabels`, Sandbox snapshot fields, and timestamps remain unchanged. Requeue a failed request and assert it starts from its persisted spec without re-reading Group/Policy. Replay every distinct non-empty `requested_labels_json` value observed in the production snapshot and require the legacy and policy-free matcher to select the same spec; this covers deployed custom label combinations beyond the named regression cases.

- [ ] **Step 5: Run the focused baseline**

Run:

```bash
go test ./internal/state -run 'Test.*(Profile|Policy|Group|Migration|Retry)' -count=1
go test ./internal/server -run 'Test.*(Profile|Policy|Group|DefaultTemplate|Retry)' -count=1
```

Expected: PASS before introducing the new matcher.

- [ ] **Step 6: Commit the compatibility fixture**

```bash
git add internal/state/store_test.go internal/server/server_test.go internal/server/server_default_templates_test.go internal/state/testdata/runner-catalog-production-2026-08-13.json
git commit -m "test(runner): freeze catalog migration compatibility"
```

---

### Task 2: Add a policy-free matcher in shadow mode

**Files:**
- Modify: `internal/state/store.go`
- Modify: `internal/state/catalog.go`
- Modify: `internal/state/store_test.go`
- Modify: `internal/server/server_runner_lifecycle.go`
- Modify: `internal/server/server_webhooks.go`
- Modify: `internal/server/server_admin_handlers.go`
- Modify: `internal/metrics/metrics.go`
- Modify: `internal/metrics/metrics_test.go`
- Modify: `internal/server/server_test.go`

**Interfaces:**
- Produces temporarily: `CompareProfileMatches(repositoryFullName string, labels []string) (ProfileMatchComparison, error)`.
- Preserves temporarily: legacy `MatchProfile(repositoryFullName string, labels []string) (ProfileMatch, error)`.
- Produces: `metrics.RecordCatalogMatchComparison(legacyProfile, enabledProfile, result string)`.

```go
type ProfileMatchComparison struct {
	Legacy ProfileMatch
	Enabled ProfileMatch
}
```

- [ ] **Step 1: Write failing tests for the new matcher**

Assert that the enabled-spec result returned by `CompareProfileMatches`:

- considers every enabled spec without loading Policy or Group rows;
- rejects disabled specs;
- preserves the existing subset and ordering rules;
- returns `profile_labels_not_matched` when no enabled candidate matches;
- returns the same result with old Group/Policy tables present, empty, or absent.

- [ ] **Step 2: Extract one shared candidate selector**

Keep sorting in one helper so the migration cannot accidentally fork priority behavior:

```go
func selectMatchingProfile(profiles []RunnerProfile, labels []string) *RunnerProfile
```

The helper filters only by `Enabled` and label matching, then applies the existing priority, label-count, and name ordering.

- [ ] **Step 3: Implement one-snapshot legacy/enabled comparison**

Load profiles, policies, and groups once, then compute both results from that immutable in-memory snapshot. Do not call two independently loading matcher methods because an admin catalog mutation between the reads could produce a false mismatch. Keep `repositoryFullName` in the input and both `ProfileMatch` responses for API compatibility, even though it does not affect the enabled-spec candidate set.

- [ ] **Step 4: Add a server admission wrapper that performs shadow comparison**

Use one helper from webhook, workflow-run reconciliation, manual label matching, and Match Test:

```go
func (s *Server) matchProfileForAdmission(repository string, labels []string) (state.ProfileMatch, error)
```

In Release A it calls `CompareProfileMatches`, records `same`, `legacy_only`, `enabled_only`, or `different_profile`, logs mismatches with repository, labels, names, and reasons, and returns the legacy result.

- [ ] **Step 5: Add expvar comparison counters**

Expose counters under `e2b_runner_catalog_match_migration_total`. Do not use repository names as metric keys; include them only in structured logs to avoid unbounded metric cardinality.

- [ ] **Step 6: Verify shadow mode does not change admission**

Run:

```bash
go test ./internal/state ./internal/server ./internal/metrics -count=1
task test
```

Expected: PASS; all existing handlers still return the legacy match result.

- [ ] **Step 7: Commit Release A**

```bash
git add internal/state internal/server internal/metrics
git commit -m "feat(runner): shadow policy-free spec matching"
```

---

### Task 3: Deploy Release A and prove live equivalence

**Files:**
- Admin Diagnostics contains the read-only observation UI and API added after Release A.
- Read-only production database export and deployment logs are external evidence and must not be committed.

**Interfaces:**
- Consumes: Admin Diagnostics historical replay, per-Spec lifecycle evidence, current-process `e2b_runner_catalog_match_migration_total`, and the manual operator checks below.
- Produces: a signed-off migration evidence record containing database backup identity, catalog snapshot, traffic window, historical comparison counts, and all enabled-Spec lifecycle evidence.

- [ ] **Step 1: Export the production database before deployment**

Use `GET /diagnostics/pprof` on the deployment being upgraded to record the effective backend and the resolved, credential-redacted database location. Compare it with the service's `--config` path and deployment database attachment before taking a backup; do not infer the production database from the process working directory. For sqlite, set both variables below to absolute paths approved by the operator, require the source to exist, and copy through the sqlite backup command rather than copying the live WAL database file:

```bash
test -n "$RUNNERD_MIGRATION_SQLITE_SOURCE"
test -f "$RUNNERD_MIGRATION_SQLITE_SOURCE"
test -n "$RUNNERD_MIGRATION_SQLITE_BACKUP"
test ! -e "$RUNNERD_MIGRATION_SQLITE_BACKUP"
sqlite3 "$RUNNERD_MIGRATION_SQLITE_SOURCE" ".backup '$RUNNERD_MIGRATION_SQLITE_BACKUP'"
sqlite3 "$RUNNERD_MIGRATION_SQLITE_BACKUP" "PRAGMA integrity_check;"
sqlite3 "$RUNNERD_MIGRATION_SQLITE_SOURCE" \
  "SELECT 'runner_profiles', COUNT(*) FROM runner_profiles UNION ALL SELECT 'runner_groups', COUNT(*) FROM runner_groups UNION ALL SELECT 'repository_policies', COUNT(*) FROM repository_policies;"
sqlite3 "$RUNNERD_MIGRATION_SQLITE_BACKUP" \
  "SELECT 'runner_profiles', COUNT(*) FROM runner_profiles UNION ALL SELECT 'runner_groups', COUNT(*) FROM runner_groups UNION ALL SELECT 'repository_policies', COUNT(*) FROM repository_policies;"
```

Expected: `ok`, and identical source/backup row counts for all three catalog tables. For Postgres or MySQL, use the platform's managed snapshot/point-in-time recovery facility for the exact effective database reported by Diagnostics, record its snapshot identifier, and verify the same three row counts through both the live connection and a restored disposable database before authorizing deployment.

- [ ] **Step 2: Capture the pre-deploy catalog rows read-only**

```sql
SELECT name, labels_json, required_labels_json, runner_group,
       max_concurrency, min_idle, priority, enabled, default_available
FROM runner_profiles
ORDER BY name;

SELECT name, description, enabled
FROM runner_groups
ORDER BY name;

SELECT group_name, spec_name
FROM runner_group_specs
ORDER BY group_name, spec_name;

SELECT id, repository_full_name, profile_name, runner_group_name, enabled
FROM repository_policies
ORDER BY id;

SELECT requested_labels_json, COUNT(*) AS request_count
FROM runner_requests
WHERE requested_labels_json IS NOT NULL AND requested_labels_json <> ''
GROUP BY requested_labels_json
ORDER BY requested_labels_json;
```

- [ ] **Step 3: Deploy Release A without changing any admin data**

Do not edit Specs, Group, Policies, Sandbox Service, or workflows during the observation window.

- [ ] **Step 4: Observe at least 72 hours and cover every distinct production label family**

The gate is not time alone. Open Admin Diagnostics and select the 72-hour (or longer) Release A readiness window. Every enabled production Spec must show at least one registered, completed, and cleanup-finalized request. The current production catalog should therefore expose rows for all five managed labels and all five custom labels. Use existing workflows or disposable test PRs with unchanged production label syntax; do not edit Runner Specs, Groups, Policies, Sandbox Service, or `runs-on` labels to manufacture evidence.

- [ ] **Step 5: Require strict parity before cutover**

Acceptance:

```text
legacy_only = 0
different_profile = 0
shadow matcher errors = 0
successful runner registration exists for every production label family
no increase in admission, profile_lookup, template_resolution, or sandbox_create failures
```

Use the historical replay counts in Admin Diagnostics as the durable source across restarts. Require `same = request_count`, with `legacy_only = 0`, `enabled_only = 0`, `different_profile = 0`, `errors = 0`, and `truncated = false`. Current-process counters are supplemental only.

`enabled_only` is expected only for installations that previously had non-default specs not covered by a Policy. It broadens availability but does not break an existing workflow. Current production should also have `enabled_only = 0` because every spec is already default-available. The automated panel does not replace the manual backup/restore, continuous-service, and unchanged-workflow-label sign-offs.

- [ ] **Step 6: Stop the rollout if parity fails**

Do not proceed to Release B. Keep Release A returning the legacy matcher, inspect the exact overlapping specs or stale data, and either fix the spec configuration or amend the plan with an explicit compatibility mapping.

---

### Task 4: Cut admission over to enabled Runner Specs

**Files:**
- Modify: `internal/state/catalog.go`
- Modify: `internal/state/store.go`
- Modify: `internal/server/server_runner_lifecycle.go`
- Modify: `internal/server/server_admin_handlers.go`
- Modify: `internal/server/server_test.go`
- Modify: `internal/state/store_test.go`

**Interfaces:**
- `MatchProfile(repositoryFullName string, labels []string)` becomes the policy-free matcher.
- Manual explicit-spec creation preserves its existing label check but no longer calls Group/Policy authorization.

- [ ] **Step 1: Write a failing cutover test**

Insert an enabled spec with `default_available = false`, no Policy, and matching labels. Assert that the final `MatchProfile` selects it. Also assert that a disabled spec with a matching Policy remains unavailable.

- [ ] **Step 2: Make the policy-free matcher authoritative**

Return the enabled-spec result from all admission paths. Keep the Release A comparison counter for one additional release, but reverse the behavior so comparison failure is observable and cannot alter the selected result.

- [ ] **Step 3: Remove `ensureRepositoryAllowsProfile`**

Replace it with a narrowly named requested-label validation helper. Do not alter GitHub repository allowlist checks or Sandbox credential resolution.

- [ ] **Step 4: Prove existing queued/running/retrying work remains stable**

Assert that workers load the persisted `ProfileName` and `RunnerGroup` from `runner_requests`, not from Group/Policy. Exercise a queued request across process restart and a failed request through manual retry.

- [ ] **Step 5: Run the behavioral suite**

```bash
go test ./internal/state ./internal/server -count=1
task test
```

Expected: PASS with no workflow fixture changes.

- [ ] **Step 6: Commit the semantic cutover**

```bash
git add internal/state internal/server
git commit -m "refactor(runner): match all enabled runner specs"
```

---

### Task 5: Retire Group/Policy from the admin product surface with one-release compatibility shims

**Files:**
- Modify: `internal/server/server.go`
- Modify: `internal/server/server_admin_handlers.go`
- Modify: `internal/server/server_helpers_test.go`
- Modify: `ui/src/App.tsx`
- Modify: `ui/src/admin-types.ts`
- Modify: `ui/src/app-load-policy.ts`
- Modify: `ui/src/app-load-policy.test.js`
- Modify: `ui/src/components/app-sidebar.tsx`
- Modify: `ui/src/components/admin-sections.tsx`
- Modify: `ui/src/components/runner-specs-section.tsx`
- Modify: `ui/src/components/runner-specs-section.test.js`
- Modify: `ui/src/hooks/use-runner-catalog.ts`
- Modify: `ui/src/locales/en.ts`
- Modify: `ui/src/locales/zh.ts`
- Delete: `ui/src/components/runner-groups-section.tsx`
- Delete: `ui/src/components/runner-policies-section.tsx`
- Delete: matching Group/Policy UI tests if present.

**Interfaces:**
- Admin navigation retains Runner Specs and removes Runner Groups and Runner Policies.
- `/admin/runner_groups` and `/admin/runner_policies` redirect to `/admin/runner_specs` in the SPA route parser.
- For Release B only, legacy management API GETs return their existing data with deprecation headers; mutations return `410 Gone`.

- [ ] **Step 1: Write UI route and load-policy tests**

Assert that the two old admin URLs resolve to Runner Specs, Overview no longer loads Policies, Runner Specs no longer loads Groups, and no background request calls `/runner_groups` or `/runner_policies`.

- [ ] **Step 2: Remove Group/Policy cards, dialogs, state, requests, sidebar items, and translations**

Keep the Runner Specs fields for labels, template, GitHub Runner Group, concurrency, minimum idle, priority, and enabled state.

- [ ] **Step 3: Remove internal Group membership and `Default Available` from Runner Specs UI**

Do not rename the GitHub `runner_group` API or database column in this migration. Label it explicitly as `GitHub Runner Group`.

- [ ] **Step 4: Add one-release HTTP compatibility shims**

For Release B:

```text
GET /runner_groups       -> 200 existing rows + Deprecation: true
GET /runner_policies     -> 200 existing rows + Deprecation: true
POST/PATCH/DELETE        -> 410 Gone
Sunset                   -> date of Release C rollout
Link                     -> /admin/runner_specs; rel="successor-version"
```

This protects an already-open old admin bundle while preventing obsolete configuration changes during the cutover. Ordinary-user APIs and workflows are unaffected.

- [ ] **Step 5: Verify UI and embedded production assets**

```bash
cd ui && bun run test
task ui-i18n-check
task ui-production-smoke
task build
```

Expected: PASS; generated `internal/server/ui/` changes come only from `task build`.

- [ ] **Step 6: Commit the product-surface retirement**

```bash
git add ui internal/server internal/server/ui
git commit -m "refactor(admin): retire runner groups and policies"
```

---

### Task 6: Expose the four managed public templates without Sandbox credentials

**Files:**
- Create: `internal/runnercatalog/public_templates.go`
- Create: `internal/runnercatalog/public_templates_test.go`
- Create: `internal/server/server_public_templates.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/server_test.go`
- Modify: `ui/src/admin-types.ts`
- Modify: `ui/src/components/sandbox-catalog-sections.tsx`
- Modify: `ui/src/components/sandbox-catalog-sections.test.js`
- Modify: `ui/src/locales/en.ts`
- Modify: `ui/src/locales/zh.ts`

**Interfaces:**
- Produces: unauthenticated `GET /api/public/runner-templates`.
- Produces: `runnercatalog.PublicTemplates() []PublicTemplate` derived only from runnerd-owned managed catalog metadata.
- Preserves: scoped `GET /user/sandbox/templates?region=<id>` for credential-bound provider templates, including qbox custom templates.

```go
type PublicTemplate struct {
	DefaultTemplateName string     `json:"default_template_name"`
	RunnerSpecNames     []string   `json:"runner_spec_names"`
	WorkflowLabels      [][]string `json:"workflow_labels"`
}
```

- [ ] **Step 1: Write failing public-catalog tests**

Assert that a signed-out request with no Sandbox configuration returns HTTP 200 and exactly four unique templates: slim, 22.04, 24.04, and 26.04. The 24.04 entry must expose both `ubuntu-24.04` and `ubuntu-latest` workflow label pairs because those managed specs share one stable public template. Assert that no custom Spec name or provider template ID appears.

- [ ] **Step 2: Derive public metadata from the managed runner catalog**

Deduplicate `runnercatalog.DefaultProfiles()` by `DefaultTemplateName`, sort templates and aliases deterministically, and expose only stable public names, managed Spec names, and supported workflow label pairs. Do not call Sandbox and do not persist a region-specific template ID.

- [ ] **Step 3: Add the public endpoint without authentication or credentials**

Register `GET /api/public/runner-templates` outside admin/user authorization helpers. Return cacheable runnerd-owned metadata; never include Sandbox credentials, private template IDs, qbox custom Specs, build history, or account/organization information.

- [ ] **Step 4: Keep public and credential-bound catalogs separate in the UI**

Always render the four public managed templates from `/api/public/runner-templates`. Load `/user/sandbox/templates` separately for the selected manageable account/organization; an unconfigured or forbidden scoped catalog may show its own configuration message but must not hide or fail the public section.

- [ ] **Step 5: Verify public visibility and scoped isolation**

```bash
go test ./internal/runnercatalog ./internal/server -run 'Test.*PublicTemplate' -count=1
cd ui && bun test src/components/sandbox-catalog-sections.test.js
task ui-i18n-check
task ui-production-smoke
```

Expected: signed-out and signed-in users see the same four public templates without a Sandbox API key; scoped users additionally see only the provider templates returned by their effective credentials.

- [ ] **Step 6: Commit the public catalog**

```bash
git add internal/runnercatalog internal/server ui/src
git commit -m "feat(runner): expose public template catalog"
```

---

### Task 7: Remove the Group/Policy code models while preserving old databases

**Files:**
- Modify: `internal/state/store.go`
- Modify: `internal/state/records.go`
- Modify: `internal/state/catalog.go`
- Modify: `internal/state/conversions.go`
- Modify: `internal/state/db.go`
- Modify: `internal/state/store_test.go`
- Modify: `internal/state/store_extra_test.go`
- Modify: `internal/runnercatalog/defaults.go`
- Modify: `internal/runnercatalog/defaults_test.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/server_admin_handlers.go`
- Modify: `internal/server/server_helpers_test.go`

**Interfaces:**
- Remove: `RunnerGroup`, `RepositoryPolicy`, and all Group/Policy methods from `RunnerCatalogStore`.
- Remove: Group/Policy record types from active `AutoMigrate` models.
- Remove: `DefaultAvailable` from the public `RunnerProfile` JSON contract.
- Preserve internally: a write-only compatibility mapping for the existing `runner_profiles.default_available` physical column until database cleanup is separately proven.

- [ ] **Step 1: Write old-database upgrade tests before deleting types**

Build a legacy sqlite fixture containing all current tables and rows, open it with the new store, and assert:

- startup succeeds;
- every Runner Spec is unchanged except that `default_available` is ignored by runtime behavior;
- old Group/Policy tables and rows remain untouched;
- new profile insert/update succeeds despite the old `default_available NOT NULL` column;
- existing custom indexes on `runner_profiles` and all `runner_requests` fields remain intact.

- [ ] **Step 2: Remove Group/Policy from the state interface and catalog implementation**

Delete CRUD methods, conversion helpers, record types, repository wildcard helpers used only by Policy, and Group-link cleanup from `DeleteProfile`.

- [ ] **Step 3: Stop auto-migrating obsolete tables without dropping them**

Remove Group/Policy models from `db.AutoMigrate`. Do not call `DropTable` in `migrate`, `Ensure`, or server startup.

- [ ] **Step 4: Preserve `default_available` database write compatibility**

Existing Postgres/MySQL/sqlite databases may have a non-null column without a server default. Keep an internal legacy column field: write `true` for newly inserted profiles, but exclude it from conflict/update assignments so existing values remain unchanged for old-binary rollback. The public `RunnerProfile`, API request payload, UI, and matcher must not expose or consume it.

- [ ] **Step 5: Remove final HTTP compatibility shims in Release C**

Delete `/runner_groups` and `/runner_policies` routes and handler tests. Old admin SPA routes continue to redirect to `/admin/runner_specs` so bookmarks remain harmless.

- [ ] **Step 6: Run state compatibility tests first**

```bash
go test ./internal/state -count=1
RUNNERD_SQLITE_SNAPSHOT=/absolute/path/to/disposable/runnerd-export.db \
  go test ./internal/state -run TestMigrateSQLiteRunnerRequestSnapshot -count=1 -v
```

Expected: PASS. The snapshot must be a disposable copy, never the live production database.

- [ ] **Step 7: Run full verification**

```bash
task lint
task test
task build
task docker-check
```

Expected: PASS, or any unrelated baseline failure is documented with passing changed-scope checks.

- [ ] **Step 8: Commit code-model removal**

```bash
git add internal/state internal/runnercatalog internal/server
git commit -m "refactor(state): remove runner group and policy models"
```

---

### Task 8: Synchronize operator and user documentation

**Files:**
- Modify: `README.md`
- Modify: `README.zh.md`
- Modify: `TODO.md`
- Modify: `AGENTS.md`
- Modify: `docs/testing.md`
- Modify: `docs/zh/testing.md`
- Modify: `docs/runner-architecture-comparison.md`
- Modify: `docs/zh/runner-architecture-comparison.md`
- Modify: `docs/deployment-smoke.md`
- Modify: `docs/zh/deployment-smoke.md`
- Modify: `ui/src/content/site-docs/en/custom-templates.md`
- Modify: `ui/src/content/site-docs/zh/custom-templates.md`
- Modify: `ui/src/content/site-docs/en/runner-labels.md`
- Modify: `ui/src/content/site-docs/zh/runner-labels.md`
- Modify: `.agents/rules/project-architecture.md`
- Modify: `.agents/rules/testing-and-verification.md`

**Interfaces:**
- Documents one availability switch: Runner Spec `enabled`.
- Documents Sandbox credential/audience/provider permission as the template authorization boundary.

- [ ] **Step 1: Remove Group/Policy and `default_available` instructions**

Delete guidance that tells operators to create a Repository Policy for a custom spec. Replace it with: create an enabled spec with unique labels, then ensure the intended account/organization has Sandbox credentials that can access the template.

- [ ] **Step 2: Clarify the two meanings of group**

State that the internal Runner Group model no longer exists. `runner_group` remains an optional GitHub Organization Runner Group passed to GitHub runner registration.

- [ ] **Step 3: Document public versus scoped template catalogs**

Document `/api/public/runner-templates` as credential-independent runnerd-owned metadata available to everyone. Keep `/user/sandbox/templates` documented as the credential-bound account/organization provider catalog; public visibility does not imply that every scope can create every template.

- [ ] **Step 4: Document rollback-safe database handling**

State that Release B/C ignore but preserve obsolete tables, and that table deletion is an explicit post-soak operator action.

- [ ] **Step 5: Run documentation verification**

```bash
task ui-i18n-check
task ui-production-smoke
rg -n "Runner Polic|Repository Polic|internal Runner Group|default_available|runner_groups|runner_policies" \
  README.md README.zh.md TODO.md AGENTS.md docs ui/src/content/site-docs .agents/rules
```

Expected: remaining matches refer only to migration history or the GitHub Runner Group distinction.

- [ ] **Step 6: Commit documentation**

```bash
git add README.md README.zh.md TODO.md AGENTS.md docs ui/src/content/site-docs .agents/rules
git commit -m "docs(runner): document policy-free runner specs"
```

---

### Task 9: Deploy Release B/C with explicit canary and rollback gates

**Files:**
- No additional repository file changes.

**Interfaces:**
- Consumes: production backup, catalog snapshot, match-comparison counters, runner lifecycle metrics, and live GitHub job evidence.
- Produces: final operational acceptance and authorization for later database cleanup.

- [ ] **Step 1: Preflight immediately before Release B**

Re-run the five catalog queries from Task 3 and compare them with the baseline. Any Spec field drift requires regenerating the expected match matrix before deployment.

- [ ] **Step 2: Deploy without deleting legacy data**

Deploy the new binary/UI, leaving the three obsolete tables and `default_available` column intact. Do not combine this rollout with a Sandbox credential, audience, template, region, or GitHub App permission change.

- [ ] **Step 3: Run live canaries using existing label syntax**

For every distinct spec label, prove:

```text
webhook admitted -> expected runner_spec_name persisted -> expected Sandbox source selected
-> expected template resolved/created -> GitHub runner registered -> job completed
-> runner and Sandbox cleaned up
```

No workflow under test may change its `runs-on` value for the migration.

- [ ] **Step 4: Monitor the full soak window**

Minimum gate: seven days and at least one successful run for every production spec. Require no regression in admission failures, profile lookup, template resolution, Sandbox creation, runner registration, job completion, retry, or cleanup.

- [ ] **Step 5: Roll back on any compatibility regression**

Rollback is the previous application image against the unchanged database. Because Group/Policy tables and `default_available` remain intact, the old binary can immediately resume legacy matching. Do not restore the full database unless the application rollback itself fails or unrelated writes were corrupted.

- [ ] **Step 6: Deploy Release C after the soak gates pass**

Remove the temporary shadow matcher, comparison metrics, compatibility API shims, and obsolete code models. Repeat the canary matrix.

---

### Task 10: Perform optional destructive database cleanup after the rollback window

**Files:**
- No startup migration code.
- Add a dated operator runbook under `docs/` only if production policy requires physical table deletion.

**Interfaces:**
- Removes only: `runner_group_specs`, `runner_groups`, and `repository_policies`.
- Preserves: `runner_profiles`, `runner_requests`, audit events, accounts, GitHub installations, Sandbox preferences/secrets/defaults, and all lifecycle records.

- [ ] **Step 1: Require explicit cleanup authorization**

Do not infer authorization from deployment success. Obtain a separate approval after Release C has completed its rollback window.

- [ ] **Step 2: Take and verify a fresh backend-native backup**

For sqlite, use `.backup` plus `PRAGMA integrity_check`. For Postgres/MySQL, require a restorable managed snapshot or tested dump.

- [ ] **Step 3: Confirm the running binary no longer reads the tables**

```bash
rg -n 'runner_groups|runner_group_specs|repository_policies|RunnerGroup|RepositoryPolicy' \
  internal ui/src --glob '!internal/server/ui/**'
```

Expected: no active-code matches other than the optional cleanup runbook or GitHub Runner Group naming.

- [ ] **Step 4: Drop tables in dependency order during a maintenance window**

```sql
DROP TABLE runner_group_specs;
DROP TABLE repository_policies;
DROP TABLE runner_groups;
```

Do not drop `runner_profiles.default_available` in this operation. It is inert and removing a column from legacy sqlite `runner_profiles` risks table recreation and data/index loss; handle it only in a separately tested schema-maintenance project.

- [ ] **Step 5: Verify immediately after cleanup**

Run the full production canary matrix and compare active/queued request counts, selected specs, failure stages, and cleanup results with the pre-cleanup baseline.

## Final Acceptance Checklist

- [ ] No existing workflow changed its `runs-on` labels.
- [ ] All ten current production label families select the same Runner Spec as before.
- [ ] Existing queued, running, failed/retried, and completed requests retain their persisted spec and GitHub runner group.
- [ ] Unauthenticated and authenticated requests to `/api/public/runner-templates` return the same four credential-independent public templates.
- [ ] qbox custom-template access remains controlled by Sandbox account/organization credentials and provider permissions.
- [ ] Repository allowlist and GitHub App authorization behavior is unchanged.
- [ ] Admin UI contains Runner Specs but no internal Runner Groups or Runner Policies.
- [ ] Group/Policy APIs and code models are absent after Release C.
- [ ] No destructive database change occurred before the rollback window expired.
- [ ] Old application rollback was tested against the preserved database before physical cleanup.
- [ ] sqlite, Postgres, and MySQL fresh-schema tests pass; old sqlite snapshot upgrade coverage passes.
