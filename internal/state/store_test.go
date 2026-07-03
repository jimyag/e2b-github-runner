package state

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

func closeTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteStoreUsesWALAndBusyTimeout(t *testing.T) {
	store := NewWithOptions(Options{
		Backend:        BackendSQLite,
		DatabaseDSN:    t.TempDir() + "/runnerd.db",
		MigrateOnStart: true,
	}).(*DBStore)
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	db, err := store.dbOrEnsure()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if got := sqlDB.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConnections = %d, want 1", got)
	}

	var journalMode string
	if err := db.Raw("PRAGMA journal_mode").Scan(&journalMode).Error; err != nil {
		t.Fatal(err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}

	var busyTimeout int
	if err := db.Raw("PRAGMA busy_timeout").Scan(&busyTimeout).Error; err != nil {
		t.Fatal(err)
	}
	if busyTimeout != 15000 {
		t.Fatalf("busy_timeout = %d, want 15000", busyTimeout)
	}

	var foreignKeys int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error; err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}
}

func TestMySQLStoreBackendIsRecognized(t *testing.T) {
	store := NewWithOptions(Options{
		Backend:        "mysql",
		DatabaseDSN:    "not a valid mysql dsn",
		MigrateOnStart: false,
	}).(*DBStore)

	err := store.Ensure()
	if err == nil {
		t.Fatal("expected invalid mysql DSN to fail")
	}
	if strings.Contains(err.Error(), "unsupported state backend") {
		t.Fatalf("mysql backend should be recognized, got %v", err)
	}
}

func TestMySQLDSNWithParseTime(t *testing.T) {
	dsn, err := mysqlDSNWithParseTime("runner:secret@tcp(mysql.example:3306)/runnerd")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dsn, "parseTime=true") {
		t.Fatalf("expected parseTime=true in DSN, got %q", dsn)
	}

	dsn, err = mysqlDSNWithParseTime("runner:secret@tcp(mysql.example:3306)/runnerd?parseTime=false&charset=utf8mb4")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dsn, "parseTime=true") || !strings.Contains(dsn, "charset=utf8mb4") {
		t.Fatalf("expected parseTime=true with existing params preserved, got %q", dsn)
	}
}

func TestCreateRequestIsIdempotent(t *testing.T) {
	store := New(t.TempDir())
	req := RunnerRequest{ID: "123", Source: "test", Labels: []string{"self-hosted"}, RunnerName: "e2b-123"}
	created, st, err := store.CreateRequest(req, []byte(`{"x":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if !created || st.Status != StatusQueued {
		t.Fatalf("unexpected first create: created=%v state=%#v", created, st)
	}

	created, st, err = store.CreateRequest(req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("expected duplicate request to reuse existing row")
	}
	if st.ID != "123" {
		t.Fatalf("unexpected state id: %q", st.ID)
	}
}

func TestCreateRequestConflictingWorkflowJobReturnsExistingState(t *testing.T) {
	store := New(t.TempDir())
	_, st, err := store.CreateRequest(RunnerRequest{
		ID:         "first",
		Source:     "test",
		JobID:      12345,
		Labels:     []string{"self-hosted"},
		RunnerName: "e2b-first",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	created, conflict, err := store.CreateRequest(RunnerRequest{
		ID:         "second",
		Source:     "test",
		JobID:      12345,
		Labels:     []string{"self-hosted"},
		RunnerName: "e2b-second",
	}, nil)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	if created {
		t.Fatal("expected conflicting workflow job to reuse existing row")
	}
	if conflict.ID != st.ID || conflict.WorkflowJobID != 12345 {
		t.Fatalf("unexpected conflicting state: %#v", conflict)
	}
}

func TestListMismatchedCompletedStates(t *testing.T) {
	store := New(t.TempDir())
	_, st, err := store.CreateRequest(RunnerRequest{
		ID:         "mismatch",
		Source:     "test",
		JobID:      1001,
		Labels:     []string{"self-hosted"},
		RunnerName: "e2b-mismatch",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Status = StatusCompleted
	st.AssignedJobID = 2002
	st.AssignedJobName = "other"
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}
	_, matched, err := store.CreateRequest(RunnerRequest{
		ID:         "matched",
		Source:     "test",
		JobID:      3003,
		Labels:     []string{"self-hosted"},
		RunnerName: "e2b-matched",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	matched.Status = StatusCompleted
	matched.AssignedJobID = 3003
	if err := store.WriteState(matched); err != nil {
		t.Fatal(err)
	}

	states, err := store.ListMismatchedCompletedStates(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].ID != "mismatch" {
		t.Fatalf("expected only mismatched completed state, got %#v", states)
	}
}

func TestListFailedWorkflowJobStates(t *testing.T) {
	store := New(t.TempDir())
	_, st, err := store.CreateRequest(RunnerRequest{
		ID:                 "failed-recovery",
		Source:             "test",
		JobID:              1001,
		RepositoryFullName: "o/r",
		Labels:             []string{"self-hosted"},
		RunnerName:         "e2b-failed-recovery",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Status = StatusFailed
	st.FailureStage = "recovery"
	st.FailureReason = "cleanup_failed"
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}
	_, other, err := store.CreateRequest(RunnerRequest{
		ID:                 "failed-start",
		Source:             "test",
		JobID:              2002,
		RepositoryFullName: "o/r",
		Labels:             []string{"self-hosted"},
		RunnerName:         "e2b-failed-start",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	other.Status = StatusFailed
	other.FailureStage = "sandbox_start"
	if err := store.WriteState(other); err != nil {
		t.Fatal(err)
	}

	states, err := store.ListFailedWorkflowJobStates(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].ID != "failed-recovery" {
		t.Fatalf("expected only failed recovery workflow job state, got %#v", states)
	}
}

func TestUpsertAccountForOAuthIdentityMaintainsRoleByOAuthIdentity(t *testing.T) {
	store := New(t.TempDir())
	account, identity, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "GitHub", OAuthSubject: "12345", OAuthLogin: "OctoCat"}, "ADMIN")
	if err != nil {
		t.Fatal(err)
	}
	if identity.OAuthProvider != "github" || identity.OAuthSubject != "12345" || identity.OAuthLogin != "octocat" || account.Role != "admin" {
		t.Fatalf("unexpected created account/identity: account=%#v identity=%#v", account, identity)
	}

	updatedAccount, updatedIdentity, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "12345", OAuthLogin: "renamed"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if updatedIdentity.ID != identity.ID || updatedIdentity.OAuthLogin != "renamed" || updatedAccount.Role != "user" {
		t.Fatalf("unexpected updated account/identity: account=%#v identity=%#v firstAccount=%#v firstIdentity=%#v", updatedAccount, updatedIdentity, account, identity)
	}

	gotAccount, gotIdentity, err := store.GetAccountByOAuthIdentity("GITHUB", "12345")
	if err != nil {
		t.Fatal(err)
	}
	if gotIdentity.ID != identity.ID || gotAccount.Role != "user" {
		t.Fatalf("unexpected fetched account/identity: account=%#v identity=%#v", gotAccount, gotIdentity)
	}

	_, gitlabIdentity, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "gitlab", OAuthSubject: "12345", OAuthLogin: "octocat"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if gitlabIdentity.ID == identity.ID {
		t.Fatalf("expected provider to separate oauth identities: github=%#v gitlab=%#v", identity, gitlabIdentity)
	}

	_, reusedLoginIdentity, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "67890", OAuthLogin: "renamed"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if reusedLoginIdentity.ID == identity.ID {
		t.Fatalf("expected stable subject to separate reused login identities: first=%#v reused=%#v", identity, reusedLoginIdentity)
	}
}

func TestEnsureAccountForOAuthIdentityCreatesDefaultWithoutOverwritingExistingRole(t *testing.T) {
	store := New(t.TempDir())
	createdAccount, createdIdentity, err := store.EnsureAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "12345", OAuthLogin: "octocat"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if createdAccount.Role != "user" {
		t.Fatalf("unexpected created role: %#v", createdAccount)
	}

	if _, _, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "12345", OAuthLogin: "octocat"}, "admin"); err != nil {
		t.Fatal(err)
	}
	ensuredAccount, ensuredIdentity, err := store.EnsureAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "12345", OAuthLogin: "renamed"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if ensuredIdentity.ID != createdIdentity.ID || ensuredIdentity.OAuthLogin != "renamed" || ensuredAccount.Role != "admin" {
		t.Fatalf("ensure should preserve existing admin role, got account=%#v identity=%#v createdAccount=%#v createdIdentity=%#v", ensuredAccount, ensuredIdentity, createdAccount, createdIdentity)
	}
}

func TestLinkOAuthIdentityToAccountSharesRoleAcrossProviders(t *testing.T) {
	store := New(t.TempDir())
	githubAccount, _, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "12345", OAuthLogin: "octocat"}, "admin")
	if err != nil {
		t.Fatal(err)
	}

	gitlabAccount, gitlabIdentity, err := store.LinkOAuthIdentityToAccount(githubAccount.ID, OAuthIdentity{OAuthProvider: "gitlab", OAuthSubject: "abcde", OAuthLogin: "octocat"})
	if err != nil {
		t.Fatal(err)
	}
	if gitlabIdentity.AccountID != githubAccount.ID || gitlabAccount.Role != "admin" {
		t.Fatalf("expected linked identity to share account role, github=%#v gitlabAccount=%#v gitlabIdentity=%#v", githubAccount, gitlabAccount, gitlabIdentity)
	}

	gotGitHubAccount, gotGitHubIdentity, err := store.GetAccountByOAuthIdentity("github", "12345")
	if err != nil {
		t.Fatal(err)
	}
	gotGitLabAccount, gotGitLabIdentity, err := store.GetAccountByOAuthIdentity("gitlab", "abcde")
	if err != nil {
		t.Fatal(err)
	}
	if gotGitHubAccount.ID != gotGitLabAccount.ID || gotGitHubAccount.Role != gotGitLabAccount.Role || gotGitHubIdentity.AccountID != gotGitLabIdentity.AccountID {
		t.Fatalf("expected provider identities to resolve to same account, githubAccount=%#v githubIdentity=%#v gitlabAccount=%#v gitlabIdentity=%#v", gotGitHubAccount, gotGitHubIdentity, gotGitLabAccount, gotGitLabIdentity)
	}
}

func TestLinkOAuthIdentityToAccountRejectsIdentityOnDifferentAccount(t *testing.T) {
	store := New(t.TempDir())
	first, _, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "12345", OAuthLogin: "octocat"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "gitlab", OAuthSubject: "abcde", OAuthLogin: "octocat"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatalf("expected separate accounts before linking, first=%#v second=%#v", first, second)
	}

	_, _, err = store.LinkOAuthIdentityToAccount(first.ID, OAuthIdentity{OAuthProvider: "gitlab", OAuthSubject: "abcde", OAuthLogin: "octocat"})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict linking identity from another account, got %v", err)
	}
}

func TestMigrateDoesNotCreateOAuthIdentityForeignKey(t *testing.T) {
	store := New(t.TempDir()).(*DBStore)
	db, err := store.dbOrEnsure()
	if err != nil {
		t.Fatal(err)
	}

	var foreignKeys []struct {
		ID int `gorm:"column:id"`
	}
	if err := db.Raw("PRAGMA foreign_key_list(oauth_identities)").Scan(&foreignKeys).Error; err != nil {
		t.Fatal(err)
	}
	if len(foreignKeys) != 0 {
		t.Fatalf("expected oauth_identities to have no database foreign keys, got %d", len(foreignKeys))
	}
}

func TestMigrateDropsLegacyOAuthIdentityForeignKey(t *testing.T) {
	dir := t.TempDir()
	databaseURL := dir + "/runnerd.db"
	store := NewWithOptions(Options{
		Backend:        BackendSQLite,
		DatabaseDSN:    databaseURL,
		MigrateOnStart: false,
	}).(*DBStore)
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := db.Exec(`CREATE TABLE accounts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		role TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE oauth_identities (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER NOT NULL,
		oauth_provider TEXT NOT NULL,
		oauth_subject TEXT NOT NULL,
		oauth_login TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		CONSTRAINT fk_oauth_identities_account FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
	);`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO accounts (id, role, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		1, "admin", now, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO oauth_identities (id, account_id, oauth_provider, oauth_subject, oauth_login, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		1, 1, "github", "12345", "octocat", now, now).Error; err != nil {
		t.Fatal(err)
	}
	closeTestDB(t, db)

	migrated := NewWithOptions(Options{
		Backend:        BackendSQLite,
		DatabaseDSN:    databaseURL,
		MigrateOnStart: true,
	}).(*DBStore)
	db, err = migrated.dbOrEnsure()
	if err != nil {
		t.Fatal(err)
	}

	var foreignKeys []struct {
		ID int `gorm:"column:id"`
	}
	if err := db.Raw("PRAGMA foreign_key_list(oauth_identities)").Scan(&foreignKeys).Error; err != nil {
		t.Fatal(err)
	}
	if len(foreignKeys) != 0 {
		t.Fatalf("expected legacy oauth_identities foreign keys to be removed, got %d", len(foreignKeys))
	}
}

func TestLinkOAuthIdentityToAccountRequiresExistingAccount(t *testing.T) {
	store := New(t.TempDir())
	_, _, err := store.LinkOAuthIdentityToAccount(999, OAuthIdentity{
		OAuthProvider: "github",
		OAuthSubject:  "12345",
		OAuthLogin:    "octocat",
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected missing account to be rejected by store logic, got %v", err)
	}
}

func TestMigrateDoesNotHandleLegacyUsers(t *testing.T) {
	dir := t.TempDir()
	databaseURL := dir + "/runnerd.db"
	store := NewWithOptions(Options{
		Backend:        BackendSQLite,
		DatabaseDSN:    databaseURL,
		MigrateOnStart: false,
	}).(*DBStore)
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		oauth_login TEXT NOT NULL,
		role TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO users (id, oauth_login, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		42, "octocat", "admin", now, now).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	migrated := NewWithOptions(Options{
		Backend:        BackendSQLite,
		DatabaseDSN:    databaseURL,
		MigrateOnStart: true,
	}).(*DBStore)
	db, err = migrated.dbOrEnsure()
	if err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable("users") {
		t.Fatal("expected legacy users table to be left untouched")
	}
	var count int64
	if err := db.Table("users").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected legacy users row to remain untouched, got %d", count)
	}
}

func TestMigrateBackfillsLegacyRunnerProfileDefaultAvailable(t *testing.T) {
	dir := t.TempDir()
	databaseURL := dir + "/runnerd.db"
	store := NewWithOptions(Options{
		Backend:        BackendSQLite,
		DatabaseDSN:    databaseURL,
		MigrateOnStart: false,
	}).(*DBStore)
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := db.Exec(`CREATE TABLE runner_profiles (
		name TEXT PRIMARY KEY,
		labels_json TEXT NOT NULL,
		template_id TEXT NOT NULL,
		runner_group TEXT,
		max_concurrency INTEGER NOT NULL,
		min_idle INTEGER NOT NULL DEFAULT 0,
		priority INTEGER NOT NULL DEFAULT 0,
		enabled BOOLEAN NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO runner_profiles (name, labels_json, template_id, runner_group, max_concurrency, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"default", `["self-hosted"]`, "base", "default", 1, true, now, now).Error; err != nil {
		t.Fatal(err)
	}
	closeTestDB(t, db)

	migrated := NewWithOptions(Options{
		Backend:        BackendSQLite,
		DatabaseDSN:    databaseURL,
		MigrateOnStart: true,
	}).(*DBStore)
	profile, err := migrated.GetProfile("default")
	if err != nil {
		t.Fatal(err)
	}
	if !profile.DefaultAvailable {
		t.Fatal("expected legacy runner profile to default to globally available")
	}
}

func TestMigrateBackfillsLegacyRepositoryPolicyRunnerGroupName(t *testing.T) {
	dir := t.TempDir()
	databaseURL := dir + "/runnerd.db"
	store := NewWithOptions(Options{
		Backend:        BackendSQLite,
		DatabaseDSN:    databaseURL,
		MigrateOnStart: false,
	}).(*DBStore)
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := db.Exec(`CREATE TABLE repository_policies (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		repository_full_name TEXT NOT NULL,
		profile_name TEXT NOT NULL,
		enabled BOOLEAN NOT NULL,
		created_at TIMESTAMP NOT NULL
	);`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO repository_policies (repository_full_name, profile_name, enabled, created_at) VALUES (?, ?, ?, ?)`,
		"owner/repo", "default", true, now).Error; err != nil {
		t.Fatal(err)
	}
	closeTestDB(t, db)

	migrated := NewWithOptions(Options{
		Backend:        BackendSQLite,
		DatabaseDSN:    databaseURL,
		MigrateOnStart: true,
	}).(*DBStore)
	policies, err := migrated.ListRepositoryPolicies()
	if err != nil {
		t.Fatal(err)
	}
	if len(policies) != 1 {
		t.Fatalf("expected one repository policy, got %d", len(policies))
	}
	if policies[0].RunnerGroupName != "" {
		t.Fatalf("expected legacy repository policy runner group to default empty, got %q", policies[0].RunnerGroupName)
	}
}

func TestMigrateResetsLegacyAccountPreferencesAndSecrets(t *testing.T) {
	dir := t.TempDir()
	databaseURL := dir + "/runnerd.db"
	store := NewWithOptions(Options{
		Backend:        BackendSQLite,
		DatabaseDSN:    databaseURL,
		MigrateOnStart: false,
	}).(*DBStore)
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := db.Exec(`CREATE TABLE account_preferences (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER NOT NULL,
		namespace TEXT NOT NULL,
		key TEXT NOT NULL,
		value_json TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE account_secrets (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER NOT NULL,
		key_type TEXT NOT NULL,
		encrypted_value TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO account_preferences (account_id, namespace, key, value_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		1, "sandbox", "service", `{"api_url":"https://legacy.example.test"}`, now, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO account_secrets (account_id, key_type, encrypted_value, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		1, AccountSecretTypeSandboxAPIKey, "legacy-encrypted-value", now, now).Error; err != nil {
		t.Fatal(err)
	}
	closeTestDB(t, db)

	migrated := NewWithOptions(Options{
		Backend:        BackendSQLite,
		DatabaseDSN:    databaseURL,
		MigrateOnStart: true,
	}).(*DBStore)
	if err := migrated.Ensure(); err != nil {
		t.Fatal(err)
	}
	migratedDB, err := migrated.dbOrEnsure()
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"account_preferences", "account_secrets"} {
		var count int64
		if err := migratedDB.Table(table).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("expected legacy %s rows to be reset, got %d", table, count)
		}
		for _, column := range []string{"scope_type", "scope_id"} {
			if !migratedDB.Migrator().HasColumn(table, column) {
				t.Fatalf("expected %s.%s after migration", table, column)
			}
		}
	}
}

func TestRepositoryPolicyIndexesArePortable(t *testing.T) {
	store := New(t.TempDir()).(*DBStore)
	db, err := store.dbOrEnsure()
	if err != nil {
		t.Fatal(err)
	}

	var indexSQL []string
	if err := db.Raw(`SELECT sql FROM sqlite_master WHERE type = 'index' AND tbl_name = 'repository_policies' AND name = 'idx_repository_policies_unique' AND sql IS NOT NULL`).Scan(&indexSQL).Error; err != nil {
		t.Fatal(err)
	}
	if len(indexSQL) != 1 {
		t.Fatalf("expected portable repository policy unique index, got %d", len(indexSQL))
	}
	for _, sql := range indexSQL {
		if strings.Contains(strings.ToUpper(sql), " WHERE ") {
			t.Fatalf("repository policy index should not use a partial WHERE clause: %s", sql)
		}
		if !strings.Contains(strings.ToUpper(sql), "UNIQUE") {
			t.Fatalf("repository policy index should enforce uniqueness: %s", sql)
		}
	}

	duplicate := repositoryPolicyRecord{
		RepositoryFullName: "owner/repo",
		ProfileName:        "default",
		Enabled:            true,
		CreatedAt:          time.Now().UTC(),
	}
	if err := db.Create(&duplicate).Error; err != nil {
		t.Fatal(err)
	}
	duplicate.ID = 0
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("expected duplicate repository policy insert to fail")
	}
}

func TestUpsertRepositoryPolicyConcurrentCreateIsIdempotent(t *testing.T) {
	store := New(t.TempDir())
	const workers = 12
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(enabled bool) {
			defer wg.Done()
			_, err := store.UpsertRepositoryPolicy(RepositoryPolicy{
				RepositoryFullName: "owner/repo",
				ProfileName:        "default",
				Enabled:            enabled,
			})
			errs <- err
		}(i%2 == 0)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("expected concurrent policy upsert to be idempotent, got %v", err)
		}
	}

	policies, err := store.ListRepositoryPolicies()
	if err != nil {
		t.Fatal(err)
	}
	var matches int
	for _, policy := range policies {
		if policy.RepositoryFullName == "owner/repo" && policy.ProfileName == "default" {
			matches++
		}
	}
	if matches != 1 {
		t.Fatalf("expected one repository policy after concurrent upsert, got %d", matches)
	}
}

func TestLargePayloadColumnsUseTextType(t *testing.T) {
	store := New(t.TempDir()).(*DBStore)
	db, err := store.dbOrEnsure()
	if err != nil {
		t.Fatal(err)
	}

	for table, columns := range map[string][]string{
		"runner_requests": {"github_payload_json"},
		"runner_events":   {"message", "payload_json"},
		"audit_events":    {"payload_json"},
	} {
		for _, column := range columns {
			var info struct {
				Type string
			}
			if err := db.Raw("SELECT type FROM pragma_table_info(?) WHERE name = ?", table, column).Scan(&info).Error; err != nil {
				t.Fatal(err)
			}
			if !strings.EqualFold(info.Type, "text") {
				t.Fatalf("%s.%s type = %q, want text", table, column, info.Type)
			}
		}
	}
}

func TestEnsureAccountForOAuthIdentityConcurrentCreateIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	const workers = 12
	stores := make([]Store, 0, workers)
	for i := 0; i < workers; i++ {
		store := New(dir)
		if err := store.Ensure(); err != nil {
			t.Fatal(err)
		}
		stores = append(stores, store)
	}
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	type accountIdentity struct {
		account  Account
		identity OAuthIdentity
	}
	results := make(chan accountIdentity, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(store Store) {
			defer wg.Done()
			account, identity, err := store.EnsureAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "12345", OAuthLogin: "octocat"}, "user")
			if err != nil {
				errs <- err
				return
			}
			results <- accountIdentity{account: account, identity: identity}
		}(stores[i])
	}
	wg.Wait()
	close(errs)
	close(results)

	for err := range errs {
		t.Fatalf("expected concurrent ensure to be idempotent, got %v", err)
	}
	var first accountIdentity
	for result := range results {
		if first.identity.ID == 0 {
			first = result
			continue
		}
		if result.identity.ID != first.identity.ID || result.account.ID != first.account.ID || result.account.Role != "user" {
			t.Fatalf("expected same identity/account from concurrent ensure, first=%#v result=%#v", first, result)
		}
	}
}

func TestUpsertAccountForOAuthIdentityRejectsInvalidRole(t *testing.T) {
	store := New(t.TempDir())
	if _, _, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "12345", OAuthLogin: "octocat"}, "owner"); err == nil {
		t.Fatal("expected invalid role error")
	}
}

func TestIsTransientStoreErrorRecognizesPostgresSQLSTATE(t *testing.T) {
	for _, message := range []string{
		"ERROR: transaction failed (SQLSTATE 40001)",
		"ERROR: transaction failed (SQLSTATE 40P01)",
		"Error 1213 (40001): Deadlock found when trying to get lock; try restarting transaction",
		"Error 1205 (HY000): Lock wait timeout exceeded; try restarting transaction",
	} {
		t.Run(message, func(t *testing.T) {
			if !isTransientStoreError(errors.New(message)) {
				t.Fatalf("expected transient store error for %q", message)
			}
		})
	}
}

func TestWriteStateUsesVersionCAS(t *testing.T) {
	store := New(t.TempDir())
	_, st, err := store.CreateRequest(RunnerRequest{
		ID:         "123",
		Source:     "test",
		Labels:     []string{"self-hosted"},
		RunnerName: "e2b-123",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	st.Status = StatusRunning
	st.ProcessPID = 42
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	st.Status = StatusFailed
	err = store.WriteState(st)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}
}

func TestListStatesAndActiveCount(t *testing.T) {
	store := New(t.TempDir())
	if _, _, err := store.CreateRequest(RunnerRequest{
		ID:         "active",
		Source:     "test",
		Labels:     []string{"self-hosted"},
		RunnerName: "e2b-active",
	}, nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if _, st, err := store.CreateRequest(RunnerRequest{
		ID:         "done",
		Source:     "test",
		Labels:     []string{"self-hosted"},
		RunnerName: "e2b-done",
	}, nil); err != nil {
		t.Fatal(err)
	} else {
		st.Status = StatusCompleted
		if err := store.WriteState(st); err != nil {
			t.Fatal(err)
		}
	}

	states, err := store.ListStates()
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 {
		t.Fatalf("expected 2 states, got %d", len(states))
	}
	if states[0].ID != "done" {
		t.Fatalf("expected newest state first, got %q", states[0].ID)
	}
	active, err := store.ActiveCount()
	if err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("expected 1 active runner, got %d", active)
	}
}

func TestListStatesPage(t *testing.T) {
	store := New(t.TempDir())
	for i := 0; i < 5; i++ {
		if _, _, err := store.CreateRequest(RunnerRequest{
			ID:         fmt.Sprintf("runner-%d", i),
			Source:     "test",
			Labels:     []string{"self-hosted"},
			RunnerName: fmt.Sprintf("e2b-runner-%d", i),
			CreatedAt:  time.Unix(int64(i), 0).UTC(),
		}, nil); err != nil {
			t.Fatal(err)
		}
	}

	paged, total, err := store.ListStatesPage(2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Fatalf("total = %d, want 5", total)
	}
	if len(paged) != 2 {
		t.Fatalf("len = %d, want 2", len(paged))
	}
	if paged[0].ID != "runner-3" || paged[1].ID != "runner-2" {
		t.Fatalf("unexpected page order: %#v", []string{paged[0].ID, paged[1].ID})
	}
}

func TestGitHubInstallationsAndRepositoryRunnerList(t *testing.T) {
	store := New(t.TempDir())
	account, _, err := store.EnsureAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "12345", OAuthLogin: "octocat"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	installation, err := store.UpsertGitHubInstallation(GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "o",
		AccountName:    "Octo Org",
		AccountAvatar:  "https://avatars.example/o.png",
		Repositories:   []string{"o/r", "o/another", "bad*", "missing-slash"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(installation.Repositories) != 0 {
		t.Fatalf("upsert should not persist the full installation repository scope, got %#v", installation.Repositories)
	}
	if _, err := store.UpsertGitHubInstallation(GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "renamed",
		AccountName:    "Renamed Org",
		AccountAvatar:  "https://avatars.example/renamed.png",
		Repositories:   []string{"o/r"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.CreateRequest(RunnerRequest{
		ID:                   "visible",
		Source:               "test",
		GitHubInstallationID: 987,
		RepositoryFullName:   "o/r",
		Labels:               []string{"self-hosted"},
		RunnerName:           "e2b-visible",
	}, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.CreateRequest(RunnerRequest{
		ID:                   "hidden",
		Source:               "test",
		GitHubInstallationID: 456,
		RepositoryFullName:   "other/r",
		Labels:               []string{"self-hosted"},
		RunnerName:           "e2b-hidden",
	}, nil); err != nil {
		t.Fatal(err)
	}
	states, err := store.ListStatesForRepositories([]string{"o/r"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].ID != "visible" {
		t.Fatalf("unexpected filtered states: %#v", states)
	}
	states, err = store.ListStatesForGitHubInstallations([]int64{987}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].ID != "visible" {
		t.Fatalf("unexpected installation-filtered states: %#v", states)
	}
	installations, err := store.ListGitHubInstallations(account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(installations) != 1 ||
		installations[0].AccountLogin != "renamed" ||
		installations[0].AccountName != "Renamed Org" ||
		installations[0].AccountAvatar != "https://avatars.example/renamed.png" ||
		!reflect.DeepEqual(installations[0].Repositories, []string{"o/r"}) {
		t.Fatalf("unexpected installations: %#v", installations)
	}
	if err := store.DeleteGitHubInstallation(account.ID, installations[0].ID); err != nil {
		t.Fatal(err)
	}
	installations, err = store.ListGitHubInstallations(account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(installations) != 0 {
		t.Fatalf("expected installation to be deleted, got %#v", installations)
	}
}

func TestGitHubInstallationsAllowSharedOrgInstallationAcrossAccounts(t *testing.T) {
	store := New(t.TempDir())
	first, _, err := store.EnsureAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "100", OAuthLogin: "alice"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := store.EnsureAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "200", OAuthLogin: "bob"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range []Account{first, second} {
		if _, err := store.UpsertGitHubInstallation(GitHubInstallation{
			AccountID:      account.ID,
			InstallationID: 987,
			AccountLogin:   "shared-org",
			Repositories:   []string{"shared-org/repo"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	firstInstallations, err := store.ListGitHubInstallations(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	secondInstallations, err := store.ListGitHubInstallations(second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstInstallations) != 1 || len(secondInstallations) != 1 {
		t.Fatalf("expected both users to keep their own installation link, first=%#v second=%#v", firstInstallations, secondInstallations)
	}
	if firstInstallations[0].ID == secondInstallations[0].ID {
		t.Fatalf("expected separate local installation rows for shared GitHub installation, got %#v", firstInstallations[0])
	}
	if err := store.DeleteGitHubInstallation(first.ID, firstInstallations[0].ID); err != nil {
		t.Fatal(err)
	}
	firstInstallations, err = store.ListGitHubInstallations(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	secondInstallations, err = store.ListGitHubInstallations(second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstInstallations) != 0 || len(secondInstallations) != 1 {
		t.Fatalf("expected deleting one user's link to preserve collaborator link, first=%#v second=%#v", firstInstallations, secondInstallations)
	}
}

func TestAccountScopeForPersonalGitHubInstallation(t *testing.T) {
	store := New(t.TempDir())
	account, _, err := store.EnsureAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "100", OAuthLogin: "alice"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "alice",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 456,
		AccountLogin:   "alice-org",
	}); err != nil {
		t.Fatal(err)
	}

	accountID, ok, err := store.AccountScopeForPersonalGitHubInstallation(987)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || accountID != account.ID {
		t.Fatalf("expected personal installation to resolve account scope, account_id=%d ok=%v", accountID, ok)
	}
	if accountID, ok, err := store.AccountScopeForPersonalGitHubInstallation(456); err != nil || ok || accountID != 0 {
		t.Fatalf("expected org installation not to resolve account scope, account_id=%d ok=%v err=%v", accountID, ok, err)
	}
	installationID, ok, err := store.GitHubInstallationScopeForAccountLogin("ALICE-ORG")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || installationID != 456 {
		t.Fatalf("expected account login to resolve installation scope, installation_id=%d ok=%v", installationID, ok)
	}
	if installationID, ok, err := store.GitHubInstallationScopeForAccountLogin("missing"); err != nil || ok || installationID != 0 {
		t.Fatalf("expected missing account login not to resolve, installation_id=%d ok=%v err=%v", installationID, ok, err)
	}
}

func TestAccountSecretsAreScopedToAccountAndType(t *testing.T) {
	store := New(t.TempDir())
	first, _, err := store.EnsureAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "100", OAuthLogin: "alice"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := store.EnsureAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "200", OAuthLogin: "bob"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountSecret(AccountSecret{
		ScopeType:      AccountScopeTypeAccount,
		ScopeID:        first.ID,
		KeyType:        AccountSecretTypeSandboxAPIKey,
		EncryptedValue: "v1:first",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountSecret(AccountSecret{
		ScopeType:      AccountScopeTypeAccount,
		ScopeID:        second.ID,
		KeyType:        AccountSecretTypeSandboxAPIKey,
		EncryptedValue: "v1:second",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountSecret(AccountSecret{
		ScopeType:      AccountScopeTypeAccount,
		ScopeID:        first.ID,
		KeyType:        "other_api_key",
		EncryptedValue: "v1:first-other",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountSecret(AccountSecret{
		ScopeType:      AccountScopeTypeAccount,
		ScopeID:        first.ID,
		KeyType:        AccountSecretTypeSandboxAPIKey,
		EncryptedValue: "v1:first-updated",
	}); err != nil {
		t.Fatal(err)
	}
	firstKey, err := store.GetAccountSecret(AccountScopeTypeAccount, first.ID, AccountSecretTypeSandboxAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	secondKey, err := store.GetAccountSecret(AccountScopeTypeAccount, second.ID, AccountSecretTypeSandboxAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	otherKey, err := store.GetAccountSecret(AccountScopeTypeAccount, first.ID, "other_api_key")
	if err != nil {
		t.Fatal(err)
	}
	if firstKey.EncryptedValue != "v1:first-updated" || secondKey.EncryptedValue != "v1:second" || otherKey.EncryptedValue != "v1:first-other" {
		t.Fatalf("unexpected sandbox api keys: first=%#v second=%#v", firstKey, secondKey)
	}
	if err := store.DeleteAccountSecret(AccountScopeTypeAccount, first.ID, AccountSecretTypeSandboxAPIKey); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetAccountSecret(AccountScopeTypeAccount, first.ID, AccountSecretTypeSandboxAPIKey); err != ErrNotFound {
		t.Fatalf("expected first key to be deleted, got %v", err)
	}
	if _, err := store.GetAccountSecret(AccountScopeTypeAccount, second.ID, AccountSecretTypeSandboxAPIKey); err != nil {
		t.Fatalf("second key should remain: %v", err)
	}
	if _, err := store.GetAccountSecret(AccountScopeTypeAccount, first.ID, "other_api_key"); err != nil {
		t.Fatalf("other key should remain: %v", err)
	}
}

func TestAccountPreferencesAreScopedToAccountNamespaceAndKey(t *testing.T) {
	store := New(t.TempDir())
	first, _, err := store.EnsureAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "100", OAuthLogin: "alice"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := store.EnsureAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "200", OAuthLogin: "bob"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountPreference(AccountPreference{
		ScopeType: AccountScopeTypeAccount,
		ScopeID:   first.ID,
		Namespace: "Sandbox",
		Key:       "Service",
		ValueJSON: `{"api_url":"https://sandbox-one.example.test"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountPreference(AccountPreference{
		ScopeType: AccountScopeTypeAccount,
		ScopeID:   second.ID,
		Namespace: "sandbox",
		Key:       "service",
		ValueJSON: `{"api_url":"https://sandbox-two.example.test"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountPreference(AccountPreference{
		ScopeType: AccountScopeTypeAccount,
		ScopeID:   first.ID,
		Namespace: "sandbox",
		Key:       "other",
		ValueJSON: `{"enabled":true}`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountPreference(AccountPreference{
		ScopeType: AccountScopeTypeAccount,
		ScopeID:   first.ID,
		Namespace: "sandbox",
		Key:       "service",
		ValueJSON: `{"api_url":"https://sandbox-one-updated.example.test"}`,
	}); err != nil {
		t.Fatal(err)
	}

	firstConfig, err := store.GetAccountPreference(AccountScopeTypeAccount, first.ID, "sandbox", "service")
	if err != nil {
		t.Fatal(err)
	}
	secondConfig, err := store.GetAccountPreference(AccountScopeTypeAccount, second.ID, "sandbox", "service")
	if err != nil {
		t.Fatal(err)
	}
	otherPreference, err := store.GetAccountPreference(AccountScopeTypeAccount, first.ID, "sandbox", "other")
	if err != nil {
		t.Fatal(err)
	}
	if firstConfig.ValueJSON != `{"api_url":"https://sandbox-one-updated.example.test"}` || secondConfig.ValueJSON != `{"api_url":"https://sandbox-two.example.test"}` || otherPreference.ValueJSON != `{"enabled":true}` {
		t.Fatalf("unexpected account preferences: first=%#v second=%#v other=%#v", firstConfig, secondConfig, otherPreference)
	}
}

func TestUpsertAccountPreferenceAndSecretRollsBackTogether(t *testing.T) {
	store := New(t.TempDir())
	account, _, err := store.EnsureAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "100", OAuthLogin: "alice"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.UpsertAccountPreferenceAndSecret(AccountPreference{
		ScopeType: AccountScopeTypeAccount,
		ScopeID:   account.ID,
		Namespace: "sandbox",
		Key:       "service",
		ValueJSON: `{"api_url":"https://sandbox.example.test"}`,
	}, &AccountSecret{
		ScopeType:      AccountScopeTypeAccount,
		ScopeID:        account.ID,
		KeyType:        AccountSecretTypeSandboxAPIKey,
		EncryptedValue: "",
	})
	if err == nil {
		t.Fatal("expected invalid secret to fail")
	}
	if _, err := store.GetAccountPreference(AccountScopeTypeAccount, account.ID, "sandbox", "service"); err != ErrNotFound {
		t.Fatalf("expected preference rollback, got %v", err)
	}
	if _, err := store.GetAccountSecret(AccountScopeTypeAccount, account.ID, AccountSecretTypeSandboxAPIKey); err != ErrNotFound {
		t.Fatalf("expected secret rollback, got %v", err)
	}
}

func TestListActiveStatesExcludesTerminalStates(t *testing.T) {
	store := New(t.TempDir())
	for _, tc := range []struct {
		id     string
		status string
	}{
		{id: "queued", status: StatusQueued},
		{id: "running", status: StatusRunning},
		{id: "completed", status: StatusCompleted},
		{id: "failed", status: StatusFailed},
	} {
		if _, st, err := store.CreateRequest(RunnerRequest{
			ID:         tc.id,
			Source:     "test",
			Labels:     []string{"self-hosted"},
			RunnerName: "e2b-" + tc.id,
		}, nil); err != nil {
			t.Fatal(err)
		} else if tc.status != StatusQueued {
			st.Status = tc.status
			if err := store.WriteState(st); err != nil {
				t.Fatal(err)
			}
		}
	}

	states, err := store.ListActiveStates()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, st := range states {
		got[st.ID] = true
	}
	if !got["queued"] || !got["running"] {
		t.Fatalf("active states missing expected rows: %#v", got)
	}
	if got["completed"] || got["failed"] {
		t.Fatalf("active states included terminal rows: %#v", got)
	}
}

func TestReadLogCanReturnTail(t *testing.T) {
	store := New(t.TempDir())
	if _, _, err := store.CreateRequest(RunnerRequest{
		ID:         "123",
		Source:     "test",
		Labels:     []string{"self-hosted"},
		RunnerName: "e2b-123",
	}, nil); err != nil {
		t.Fatal(err)
	}
	store.AppendLog("123", "control.log", []byte("line-1\n"))
	store.AppendLog("123", "control.log", []byte("line-2\n"))
	data, err := store.ReadLog("123", "control.log", 7)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "line-2\n" {
		t.Fatalf("unexpected log tail: %q", string(data))
	}
}

func TestProfilesAndPoliciesCanBeMatched(t *testing.T) {
	store := New(t.TempDir())
	if _, err := store.UpsertProfile(RunnerProfile{
		Name:           "default",
		Labels:         []string{"self-hosted", "e2b"},
		TemplateID:     "base",
		MaxConcurrency: 10,
		Enabled:        true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertProfile(RunnerProfile{
		Name:           "large",
		Labels:         []string{"self-hosted", "e2b", "large"},
		TemplateID:     "large",
		MaxConcurrency: 2,
		Priority:       10,
		Enabled:        true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertRepositoryPolicy(RepositoryPolicy{
		RepositoryFullName: "owner/repo",
		ProfileName:        "default",
		Enabled:            true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertRepositoryPolicy(RepositoryPolicy{
		RepositoryFullName: "owner/repo",
		ProfileName:        "large",
		Enabled:            true,
	}); err != nil {
		t.Fatal(err)
	}

	match, err := store.MatchProfile("owner/repo", []string{"self-hosted", "e2b", "large"})
	if err != nil {
		t.Fatal(err)
	}
	if match.Profile == nil || match.Profile.Name != "large" {
		t.Fatalf("expected large profile match, got %#v", match.Profile)
	}
}

func TestMatchProfileReturnsReasonWhenPolicyMissing(t *testing.T) {
	store := New(t.TempDir())
	match, err := store.MatchProfile("owner/repo", []string{"self-hosted", "e2b"})
	if err != nil {
		t.Fatal(err)
	}
	if match.Profile != nil || match.Reason != "profile_not_allowed" {
		t.Fatalf("unexpected match result: %#v", match)
	}
}

func TestDefaultAvailableProfileMatchesWithoutRepositoryPolicy(t *testing.T) {
	store := New(t.TempDir())
	if _, err := store.UpsertProfile(RunnerProfile{
		Name:             "default",
		Labels:           []string{"self-hosted", "e2b"},
		TemplateID:       "base",
		MaxConcurrency:   10,
		Enabled:          true,
		DefaultAvailable: true,
	}); err != nil {
		t.Fatal(err)
	}
	match, err := store.MatchProfile("owner/any-repo", []string{"self-hosted", "e2b"})
	if err != nil {
		t.Fatal(err)
	}
	if match.Profile == nil || match.Profile.Name != "default" {
		t.Fatalf("expected default profile match, got %#v", match.Profile)
	}
}

func TestMatchProfileAllowsRunnerSpecWithAdditionalLabels(t *testing.T) {
	store := New(t.TempDir())
	if _, err := store.UpsertProfile(RunnerProfile{
		Name:             "default",
		Labels:           []string{"self-hosted", "e2b", "las-sandbox", "github-runner-ubuntu-24-04"},
		TemplateID:       "base",
		MaxConcurrency:   10,
		Enabled:          true,
		DefaultAvailable: true,
	}); err != nil {
		t.Fatal(err)
	}
	match, err := store.MatchProfile("owner/any-repo", []string{"github-runner-ubuntu-24-04"})
	if err != nil {
		t.Fatal(err)
	}
	if match.Profile == nil || match.Profile.Name != "default" {
		t.Fatalf("expected default profile match, got %#v", match.Profile)
	}
}

func TestRunnerGroupPolicyMatchesGroupSpecs(t *testing.T) {
	store := New(t.TempDir())
	if _, err := store.UpsertProfile(RunnerProfile{
		Name:           "small",
		Labels:         []string{"self-hosted", "e2b", "small"},
		TemplateID:     "small-template",
		MaxConcurrency: 10,
		Enabled:        true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertProfile(RunnerProfile{
		Name:           "large",
		Labels:         []string{"self-hosted", "e2b", "large"},
		TemplateID:     "large-template",
		MaxConcurrency: 5,
		Priority:       20,
		Enabled:        true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertRunnerGroup(RunnerGroup{
		Name:      "ubuntu-2404",
		SpecNames: []string{"small", "large"},
		Enabled:   true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertRepositoryPolicy(RepositoryPolicy{
		RepositoryFullName: "owner/repo",
		RunnerGroupName:    "ubuntu-2404",
		Enabled:            true,
	}); err != nil {
		t.Fatal(err)
	}

	match, err := store.MatchProfile("owner/repo", []string{"self-hosted", "e2b", "large"})
	if err != nil {
		t.Fatal(err)
	}
	if match.Profile == nil || match.Profile.Name != "large" {
		t.Fatalf("expected large profile through group policy, got %#v", match.Profile)
	}
}

func TestClaimNextRunnableHonorsLeaseAndRetryWindow(t *testing.T) {
	store := New(t.TempDir())
	now := time.Now().UTC()
	if _, st, err := store.CreateRequest(RunnerRequest{
		ID:              "retryable",
		Source:          "test",
		RequestedLabels: []string{"self-hosted"},
		Labels:          []string{"self-hosted"},
		RunnerName:      "e2b-retryable",
	}, nil); err != nil {
		t.Fatal(err)
	} else {
		st.NextRetryAt = now.Add(-time.Second)
		if err := store.WriteState(st); err != nil {
			t.Fatal(err)
		}
	}

	req, st, claimed, err := store.ClaimNextRunnable("worker-1", now, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !claimed || req.ID != "retryable" || st.LeaseOwner != "worker-1" || st.LastAttemptAt.IsZero() {
		t.Fatalf("unexpected claim result: req=%#v state=%#v claimed=%v", req, st, claimed)
	}

	_, _, claimed, err = store.ClaimNextRunnable("worker-2", now, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("expected active lease to block second claim")
	}
}

func TestRetryRequestClearsFailureFields(t *testing.T) {
	store := New(t.TempDir())
	_, st, err := store.CreateRequest(RunnerRequest{
		ID:              "retry-me",
		Source:          "test",
		RequestedLabels: []string{"self-hosted"},
		Labels:          []string{"self-hosted"},
		RunnerName:      "e2b-retry-me",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Status = StatusFailed
	st.FailureStage = "sandbox_start"
	st.FailureReason = "backend_server_error"
	st.LastErrorCode = "backend_server_error"
	st.LastErrorMessage = "temporary failure"
	st.LastErrorRetryable = true
	st.NextRetryAt = time.Now().Add(time.Minute).UTC()
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	retried, err := store.RetryRequest("retry-me", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if retried.Status != StatusQueued || retried.FailureStage != "" || retried.LastErrorCode != "" || !retried.NextRetryAt.IsZero() {
		t.Fatalf("unexpected retried state: %#v", retried)
	}
}

func TestRetryRequestRejectsActiveState(t *testing.T) {
	store := New(t.TempDir())
	_, st, err := store.CreateRequest(RunnerRequest{
		ID:              "running",
		Source:          "test",
		RequestedLabels: []string{"self-hosted"},
		Labels:          []string{"self-hosted"},
		RunnerName:      "e2b-running",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Status = StatusRunning
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	if _, err := store.RetryRequest("running", time.Now().UTC()); !errors.Is(err, ErrRetryNotAllowed) {
		t.Fatalf("expected retry guard, got %v", err)
	}
}

func TestAuditEventsCanBeListed(t *testing.T) {
	store := New(t.TempDir())
	if _, err := store.AppendAuditEvent(AuditEvent{
		Actor:        "admin_api",
		Action:       "runner.retry",
		ResourceType: "runner_request",
		ResourceID:   "retry-me",
		PayloadJSON:  `{"status":"queued"}`,
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	events, err := store.ListAuditEvents(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Action != "runner.retry" {
		t.Fatalf("unexpected audit events: %#v", events)
	}
}

func TestRunnerStateDerivesGitHubJobLinkFromWorkflowJobPayload(t *testing.T) {
	store := New(t.TempDir()).(*DBStore)
	payload := []byte(`{
		"workflow_job": {
			"id": 77684492230,
			"run_id": 26392225417,
			"workflow_name": "CI",
			"run_attempt": 2,
			"head_branch": "feature/group-jobs",
			"head_sha": "abc123def456",
			"html_url": "https://github.com/qbox/las/actions/runs/26392225417/job/77684492230",
			"pull_requests": [{"number": 3335}]
		}
	}`)
	_, st, err := store.CreateRequest(RunnerRequest{
		ID:                 "77684492230",
		Source:             "github",
		JobID:              77684492230,
		RepositoryFullName: "qbox/las",
		RequestedLabels:    []string{"github-runner-ubuntu-24-04"},
		Labels:             []string{"github-runner-ubuntu-24-04"},
		RunnerName:         "e2b-77684492230",
	}, payload)
	if err != nil {
		t.Fatal(err)
	}
	if st.WorkflowRunID != 26392225417 || st.WorkflowJobID != 77684492230 || st.PullRequestNumber != 3335 {
		t.Fatalf("unexpected github metadata: %#v", st)
	}
	if st.WorkflowName != "CI" || st.WorkflowRunAttempt != 2 || st.HeadBranch != "feature/group-jobs" || st.HeadSHA != "abc123def456" {
		t.Fatalf("unexpected github context: %#v", st)
	}
	wantURL := "https://github.com/qbox/las/actions/runs/26392225417/job/77684492230?pr=3335"
	if st.GitHubJobURL != wantURL {
		t.Fatalf("unexpected github job url: %q", st.GitHubJobURL)
	}

	db, err := store.dbOrEnsure()
	if err != nil {
		t.Fatal(err)
	}
	var record runnerRequestRecord
	if err := db.First(&record, "id = ?", "77684492230").Error; err != nil {
		t.Fatal(err)
	}
	if record.WorkflowRunID != 26392225417 || record.WorkflowName != "CI" || record.WorkflowRunAttempt != 2 {
		t.Fatalf("expected github context to be persisted on runner_requests: %#v", record)
	}
	if record.HeadBranch != "feature/group-jobs" || record.HeadSHA != "abc123def456" || record.PullRequestNumber != 3335 {
		t.Fatalf("expected grouping fields to be persisted on runner_requests: %#v", record)
	}
	if record.GitHubJobURL != wantURL {
		t.Fatalf("expected github job url to be persisted, got %q", record.GitHubJobURL)
	}
	if !record.GitHubContextBackfilled {
		t.Fatalf("expected github context backfill marker to be persisted")
	}
}

func TestRunnerStateBuildsGitHubJobLinkFromWorkflowRunPayload(t *testing.T) {
	store := New(t.TempDir())
	payload := []byte(`{
		"workflow_run": {
			"id": 26392225417,
			"pull_requests": [{"number": 3335}]
		}
	}`)
	_, st, err := store.CreateRequest(RunnerRequest{
		ID:                 "77684492230",
		Source:             "github_reconcile",
		JobID:              77684492230,
		RepositoryFullName: "qbox/las",
		RequestedLabels:    []string{"github-runner-ubuntu-24-04"},
		Labels:             []string{"github-runner-ubuntu-24-04"},
		RunnerName:         "e2b-77684492230",
	}, payload)
	if err != nil {
		t.Fatal(err)
	}
	wantURL := "https://github.com/qbox/las/actions/runs/26392225417/job/77684492230?pr=3335"
	if st.WorkflowRunID != 26392225417 || st.GitHubJobURL != wantURL {
		t.Fatalf("unexpected github metadata: %#v", st)
	}
}

func TestRunnerStateUsesBackfilledGitHubContextWithoutParsingPayload(t *testing.T) {
	workflowJobID := int64(77684492230)
	st := recordToState(runnerRequestRecord{
		ID:                      "77684492230",
		Source:                  "github",
		WorkflowJobID:           &workflowJobID,
		GitHubInstallationID:    42,
		WorkflowRunID:           26392225417,
		WorkflowName:            "CI",
		WorkflowRunAttempt:      2,
		HeadBranch:              "feature/group-jobs",
		HeadSHA:                 "abc123def456",
		GitHubJobURL:            "https://github.com/qbox/las/actions/runs/26392225417/job/77684492230",
		PullRequestNumber:       3335,
		GitHubContextBackfilled: true,
		RepositoryFullName:      "qbox/las",
		RequestedLabelsJSON:     `["self-hosted"]`,
		LabelsJSON:              `["self-hosted"]`,
		RunnerName:              "e2b-77684492230",
		Status:                  StatusQueued,
		GitHubPayloadJSON:       `{"workflow_job":{"run_id":111,"workflow_name":"stale","run_attempt":9,"head_branch":"stale","head_sha":"stale","pull_requests":[{"number":1}]}}`,
	})
	if st.WorkflowRunID != 26392225417 || st.WorkflowName != "CI" || st.WorkflowRunAttempt != 2 {
		t.Fatalf("expected state to use denormalized github context, got %#v", st)
	}
	if st.HeadBranch != "feature/group-jobs" || st.HeadSHA != "abc123def456" || st.PullRequestNumber != 3335 {
		t.Fatalf("expected state to use denormalized grouping fields, got %#v", st)
	}
	wantURL := "https://github.com/qbox/las/actions/runs/26392225417/job/77684492230?pr=3335"
	if st.GitHubJobURL != wantURL {
		t.Fatalf("expected denormalized github job url, got %q", st.GitHubJobURL)
	}
}

func TestMigrateBackfillsLegacyRunnerRequestGitHubContext(t *testing.T) {
	dir := t.TempDir()
	databaseURL := dir + "/runnerd.db"
	store := NewWithOptions(Options{
		Backend:        BackendSQLite,
		DatabaseDSN:    databaseURL,
		MigrateOnStart: false,
	}).(*DBStore)
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := db.Exec(`CREATE TABLE runner_requests (
		id TEXT PRIMARY KEY,
		source TEXT NOT NULL,
		workflow_job_id INTEGER,
		github_installation_id INTEGER,
		repository_full_name TEXT,
		requested_labels_json TEXT,
		labels_json TEXT NOT NULL,
		profile_name TEXT,
		runner_group TEXT,
		runner_name TEXT NOT NULL,
		status TEXT NOT NULL,
		failure_stage TEXT,
		failure_reason TEXT,
		last_error_code TEXT,
		last_error_message TEXT,
		last_error_retryable BOOLEAN NOT NULL DEFAULT FALSE,
		retry_count INTEGER NOT NULL DEFAULT 0,
		sandbox_id TEXT,
		process_pid INTEGER,
		assigned_job_id INTEGER,
		assigned_job_name TEXT,
		error TEXT,
		github_payload_json TEXT,
		queued_at TIMESTAMP NOT NULL,
		last_attempt_at TIMESTAMP,
		next_retry_at TIMESTAMP,
		creating_at TIMESTAMP,
		running_at TIMESTAMP,
		stopping_at TIMESTAMP,
		completed_at TIMESTAMP,
		failed_at TIMESTAMP,
		lease_owner TEXT,
		lease_expires_at TIMESTAMP,
		updated_at TIMESTAMP NOT NULL,
		version INTEGER NOT NULL DEFAULT 0
	);`).Error; err != nil {
		t.Fatal(err)
	}
	payload := `{"workflow_job":{"run_id":26392225417,"workflow_name":"CI","run_attempt":2,"head_branch":"feature/group-jobs","head_sha":"abc123def456","pull_requests":[{"number":3335}]}}`
	if err := db.Exec(`INSERT INTO runner_requests (
		id, source, workflow_job_id, github_installation_id, repository_full_name,
		requested_labels_json, labels_json, runner_name, status, github_payload_json,
		queued_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"77684492230", "github", 77684492230, 42, "qbox/las",
		`["self-hosted"]`, `["self-hosted"]`, "e2b-77684492230", StatusQueued, payload,
		now, now).Error; err != nil {
		t.Fatal(err)
	}
	closeTestDB(t, db)

	migrated := NewWithOptions(Options{
		Backend:        BackendSQLite,
		DatabaseDSN:    databaseURL,
		MigrateOnStart: true,
	}).(*DBStore)
	states, err := migrated.ListStatesForGitHubInstallations([]int64{42}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 {
		t.Fatalf("expected one migrated runner request, got %d", len(states))
	}
	st := states[0]
	if st.WorkflowRunID != 26392225417 || st.PullRequestNumber != 3335 {
		t.Fatalf("expected github context after migration, got %#v", st)
	}
	if st.WorkflowName != "CI" || st.WorkflowRunAttempt != 2 || st.HeadBranch != "feature/group-jobs" || st.HeadSHA != "abc123def456" {
		t.Fatalf("unexpected github context after migration: %#v", st)
	}

	db, err = migrated.dbOrEnsure()
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{
		"workflow_run_id",
		"workflow_name",
		"workflow_run_attempt",
		"head_branch",
		"head_sha",
		"github_job_url",
		"pull_request_number",
		"github_context_backfilled",
	} {
		if !db.Migrator().HasColumn(&runnerRequestRecord{}, column) {
			t.Fatalf("expected migrated runner_requests.%s column", column)
		}
	}
	var record runnerRequestRecord
	if err := db.First(&record, "id = ?", "77684492230").Error; err != nil {
		t.Fatal(err)
	}
	if record.WorkflowRunID != 26392225417 || record.PullRequestNumber != 3335 {
		t.Fatalf("expected migrated runner request columns to be backfilled: %#v", record)
	}
	if record.WorkflowName != "CI" || record.WorkflowRunAttempt != 2 || record.HeadBranch != "feature/group-jobs" || record.HeadSHA != "abc123def456" {
		t.Fatalf("unexpected backfilled runner request columns: %#v", record)
	}
	wantURL := "https://github.com/qbox/las/actions/runs/26392225417/job/77684492230?pr=3335"
	if record.GitHubJobURL != wantURL {
		t.Fatalf("expected github job url to be backfilled, got %q", record.GitHubJobURL)
	}
	if !record.GitHubContextBackfilled {
		t.Fatalf("expected github context backfill marker to be set")
	}
	var pendingBackfills int64
	if err := db.Model(&runnerRequestRecord{}).
		Where("github_payload_json <> ''").
		Where("github_context_backfilled IS NULL OR github_context_backfilled = ?", false).
		Count(&pendingBackfills).Error; err != nil {
		t.Fatal(err)
	}
	if pendingBackfills != 0 {
		t.Fatalf("expected no pending github context backfills, got %d", pendingBackfills)
	}
}
