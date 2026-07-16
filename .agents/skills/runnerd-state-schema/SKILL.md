---
name: runnerd-state-schema
description: Use when changing qiniu-ci-runner state records, GORM tags, indexes, migration compatibility, old sqlite upgrade behavior, or DB-backed runner state semantics.
---

# Runnerd State Schema

Start by saying: "I am using the runnerd-state-schema skill to change or verify state schema behavior."

## Goal

Keep runnerd's database schema model-driven through GORM while preserving known older state database upgrade paths.

## When To Use

- Editing `internal/state/records.go`.
- Editing migration logic in `internal/state/db.go`.
- Changing state indexes, uniqueness, required columns, defaults, or relationship constraints.
- Reviewing schema changes for sqlite/Postgres/MySQL compatibility.
- Investigating failures around `AutoMigrate`, old sqlite files, or `NOT NULL` column additions.

## Rules

- Prefer GORM tags and `AutoMigrate` for normal schema source of truth.
- Use handwritten SQL only for genuine GORM gaps.
- Keep every legacy compatibility action narrow and explicit. This includes column additions, obsolete constraint removal, and destructive reset of legacy tables that cannot represent the current scope model. Document required operator reconfiguration and user reauthentication.
- GORM foreign-key creation is disabled intentionally. Preserve the foreign-keyless schema convention unless a separately tested migration changes it.
- Do not reintroduce legacy `users` migration behavior unless the user explicitly asks.
- Avoid `default:true` on business booleans when zero-value preservation matters.
- Fresh database tests are not enough for required-column changes; add or preserve old-schema upgrade coverage.

## Workflow

1. Inspect current state records and migration helpers:

```bash
sed -n '1,260p' internal/state/records.go
sed -n '1,220p' internal/state/db.go
```

2. Check existing tests for old-schema fixtures and migration behavior:

```bash
rg -n "Migrate|Legacy|default_available|runner_group_name|AutoMigrate" internal/state
```

3. For schema edits, update the model tags first. Change `migrateLegacySchemaColumns` only when old databases cannot safely migrate through `AutoMigrate` alone. Cover column additions, constraint removal, or legacy-table reset with an old-schema fixture that proves the required cleanup and asserts preservation or intentional data loss according to the compatibility contract.

4. Add or update tests before relying on the fix. Include old sqlite upgrade coverage for required columns, unique indexes with existing rows, and relationship changes.

5. Run:

```bash
go test ./internal/state -count=1
```

6. If callers or server behavior changed, also run:

```bash
go test ./...
task test
```

7. Sync docs if behavior or maintenance rules changed:

- `README.md` and `README.zh.md`
- `TODO.md`
- `AGENTS.md`
- `docs/testing.md` and `docs/zh/testing.md`
- `.agents/rules/project-architecture.md`
- `.agents/rules/testing-and-verification.md`

## Output

Report:

- schema files changed;
- whether old-schema compatibility was needed;
- tests added or preserved;
- verification commands and results;
- any unverified DB backend or multi-instance boundary.
