package state

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
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

func TestSandboxServiceDefaultLifecycle(t *testing.T) {
	store := New(t.TempDir())

	if _, err := store.GetSandboxServiceDefault(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetSandboxServiceDefault() error = %v, want ErrNotFound", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	saved, err := store.UpsertSandboxServiceDefault(SandboxServiceDefault{
		Enabled:         true,
		APIURL:          "https://sandbox.example.test",
		APIKeyEncrypted: "encrypted-key",
		APIKeyUpdatedAt: &now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID != 1 || !saved.Enabled || saved.APIKeyEncrypted != "encrypted-key" {
		t.Fatalf("unexpected saved default: %#v", saved)
	}

	saved, err = store.UpsertSandboxServiceDefault(SandboxServiceDefault{
		Enabled: false,
		APIURL:  "https://sandbox.example.test/v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Enabled || saved.APIURL != "https://sandbox.example.test/v2" || saved.APIKeyEncrypted != "encrypted-key" {
		t.Fatalf("expected update to preserve encrypted key: %#v", saved)
	}
	if saved.APIKeyUpdatedAt == nil || !saved.APIKeyUpdatedAt.Equal(now) {
		t.Fatalf("expected update to preserve key timestamp: %#v", saved.APIKeyUpdatedAt)
	}

	if err := store.DeleteSandboxServiceDefaultAPIKey(); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteSandboxServiceDefaultAPIKey(); err != nil {
		t.Fatalf("expected key deletion to be idempotent: %v", err)
	}
	saved, err = store.GetSandboxServiceDefault()
	if err != nil {
		t.Fatal(err)
	}
	if saved.APIKeyEncrypted != "" || saved.APIKeyUpdatedAt != nil || saved.APIURL == "" {
		t.Fatalf("expected only API key metadata to be cleared: %#v", saved)
	}
}

func TestSandboxServiceDefaultAudienceLifecycle(t *testing.T) {
	store := New(t.TempDir())
	saved, err := store.UpsertSandboxServiceDefault(SandboxServiceDefault{
		Enabled: true,
		APIURL:  "https://sandbox.example.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.AudienceMode != SandboxServiceDefaultAudienceModeAll {
		t.Fatalf("default audience mode = %q", saved.AudienceMode)
	}
	saved, err = store.UpsertSandboxServiceDefault(SandboxServiceDefault{
		Enabled:      false,
		APIURL:       saved.APIURL,
		AudienceMode: SandboxServiceDefaultAudienceModeSelected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.AudienceMode != SandboxServiceDefaultAudienceModeSelected {
		t.Fatalf("saved audience mode = %q", saved.AudienceMode)
	}

	audience, err := store.UpsertSandboxServiceDefaultAudience(SandboxServiceDefaultAudience{
		GitHubAccountID: 9001,
		AccountType:     "Organization",
		AccountLogin:    "Octo-Org",
		AccountName:     "Octo Org",
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpsertSandboxServiceDefaultAudience(SandboxServiceDefaultAudience{
		GitHubAccountID: 9001,
		AccountType:     "organization",
		AccountLogin:    "renamed-org",
		AccountName:     "Renamed Org",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != audience.ID || updated.AccountLogin != "renamed-org" {
		t.Fatalf("expected duplicate identity to update display metadata: first=%#v updated=%#v", audience, updated)
	}
	audiences, err := store.ListSandboxServiceDefaultAudiences()
	if err != nil {
		t.Fatal(err)
	}
	if len(audiences) != 1 || audiences[0].GitHubAccountID != 9001 || audiences[0].AccountType != "organization" {
		t.Fatalf("unexpected audiences: %#v", audiences)
	}
	allowed, err := store.SandboxServiceDefaultAudienceContains(9001, "Organization")
	if err != nil || !allowed {
		t.Fatalf("expected audience membership, allowed=%v err=%v", allowed, err)
	}
	if allowed, err := store.SandboxServiceDefaultAudienceContains(9002, "organization"); err != nil || allowed {
		t.Fatalf("unexpected audience membership, allowed=%v err=%v", allowed, err)
	}
	if err := store.DeleteSandboxServiceDefaultAudience(audience.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteSandboxServiceDefaultAudience(audience.ID); err != nil {
		t.Fatalf("expected audience deletion to be idempotent: %v", err)
	}
}

func TestSandboxServiceDefaultAudiencesSortCaseInsensitively(t *testing.T) {
	store := New(t.TempDir())
	for githubAccountID, accountLogin := range map[int64]string{
		1: "beta",
		2: "Alpha",
		3: "charlie",
	} {
		if _, err := store.UpsertSandboxServiceDefaultAudience(SandboxServiceDefaultAudience{
			GitHubAccountID: githubAccountID,
			AccountType:     "organization",
			AccountLogin:    accountLogin,
		}); err != nil {
			t.Fatal(err)
		}
	}
	audiences, err := store.ListSandboxServiceDefaultAudiences()
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(audiences))
	for _, audience := range audiences {
		got = append(got, audience.AccountLogin)
	}
	if want := []string{"Alpha", "beta", "charlie"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("audience order = %#v, want %#v", got, want)
	}
}

func TestGitHubInstallationOwnerCacheLifecycle(t *testing.T) {
	store := New(t.TempDir())
	if _, err := store.GetGitHubInstallationOwner(987); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetGitHubInstallationOwner() error = %v, want ErrNotFound", err)
	}
	owner, err := store.UpsertGitHubInstallationOwner(987, GitHubInstallationAccount{
		GitHubAccountID: 9001,
		AccountType:     "Organization",
		AccountLogin:    "octo-org",
		AccountName:     "Octo Org",
	})
	if err != nil {
		t.Fatal(err)
	}
	if owner.GitHubAccountID != 9001 || owner.AccountType != "organization" || owner.AccountLogin != "octo-org" {
		t.Fatalf("unexpected cached owner: %#v", owner)
	}
	updated, err := store.UpsertGitHubInstallationOwner(987, GitHubInstallationAccount{
		GitHubAccountID: 9001,
		AccountType:     "organization",
		AccountLogin:    "renamed-org",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.AccountLogin != "renamed-org" {
		t.Fatalf("expected display metadata update, got %#v", updated)
	}
	loaded, err := store.GetGitHubInstallationOwner(987)
	if err != nil || loaded.AccountLogin != "renamed-org" {
		t.Fatalf("unexpected loaded owner: %#v err=%v", loaded, err)
	}
}

func TestRunnerRequestSandboxConfigSourceRoundTripAndRetryReset(t *testing.T) {
	store := New(t.TempDir())
	_, st, err := store.CreateRequest(RunnerRequest{
		ID:                     "sandbox-source",
		Source:                 "test",
		Labels:                 []string{"self-hosted"},
		RunnerName:             "e2b-sandbox-source",
		SandboxAPIURL:          "https://sandbox.example.test",
		SandboxAPIKeyEncrypted: "encrypted-key",
		SandboxConfigSource:    "admin_default",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if st.SandboxConfigSource != "admin_default" {
		t.Fatalf("created state source = %q", st.SandboxConfigSource)
	}

	req, err := store.ReadRequest("sandbox-source")
	if err != nil {
		t.Fatal(err)
	}
	if req.SandboxConfigSource != "admin_default" {
		t.Fatalf("request source = %q", req.SandboxConfigSource)
	}

	st.Status = StatusFailed
	st.SandboxConfigSource = "account"
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}
	st, err = store.ReadState("sandbox-source")
	if err != nil {
		t.Fatal(err)
	}
	if st.SandboxConfigSource != "account" {
		t.Fatalf("updated state source = %q", st.SandboxConfigSource)
	}

	retried, err := store.RetryRequest("sandbox-source", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if retried.SandboxConfigSource != "" || retried.SandboxAPIURL != "" || retried.SandboxAPIKeyEncrypted != "" {
		t.Fatalf("expected retry to clear sandbox config snapshot: %#v", retried)
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

func TestListAccountsSearchesFiltersAndPaginates(t *testing.T) {
	store := New(t.TempDir())
	admin, _, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "100", OAuthLogin: "alpha"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	user, _, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "200", OAuthLogin: "bravo"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LinkOAuthIdentityToAccount(user.ID, OAuthIdentity{OAuthProvider: "gitlab", OAuthSubject: "gl-200", OAuthLogin: "Bravo-Lab"}); err != nil {
		t.Fatal(err)
	}
	third, _, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "300", OAuthLogin: "charlie"}, "user")
	if err != nil {
		t.Fatal(err)
	}

	firstPage, total, err := store.ListAccounts(AccountListOptions{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(firstPage) != 2 {
		t.Fatalf("unexpected first page: total=%d accounts=%#v", total, firstPage)
	}
	secondPage, secondTotal, err := store.ListAccounts(AccountListOptions{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatal(err)
	}
	if secondTotal != 3 || len(secondPage) != 1 || secondPage[0].ID == firstPage[0].ID || secondPage[0].ID == firstPage[1].ID {
		t.Fatalf("unexpected second page: total=%d accounts=%#v first=%#v", secondTotal, secondPage, firstPage)
	}

	users, userTotal, err := store.ListAccounts(AccountListOptions{Role: " USER ", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if userTotal != 2 || len(users) != 2 {
		t.Fatalf("unexpected user filter: total=%d accounts=%#v", userTotal, users)
	}
	for _, item := range users {
		if item.Role != "user" || item.ID == admin.ID {
			t.Fatalf("role filter returned the wrong account: %#v", item)
		}
	}

	searched, searchTotal, err := store.ListAccounts(AccountListOptions{Query: " BRAVO-LAB ", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if searchTotal != 1 || len(searched) != 1 || searched[0].ID != user.ID || len(searched[0].OAuthIdentities) != 2 {
		t.Fatalf("unexpected login search result: total=%d accounts=%#v", searchTotal, searched)
	}
	if searched[0].OAuthIdentities[0].OAuthProvider != "github" || searched[0].OAuthIdentities[1].OAuthProvider != "gitlab" {
		t.Fatalf("expected stable provider ordering, got %#v", searched[0].OAuthIdentities)
	}

	bySubject, subjectTotal, err := store.ListAccounts(AccountListOptions{Query: "300", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if subjectTotal != 1 || len(bySubject) != 1 || bySubject[0].ID != third.ID {
		t.Fatalf("unexpected subject search result: total=%d accounts=%#v", subjectTotal, bySubject)
	}
}

func TestListAccountsRejectsUnexpectedIdentityAccount(t *testing.T) {
	store := New(t.TempDir()).(*DBStore)
	if _, _, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "100", OAuthLogin: "alpha"}, "admin"); err != nil {
		t.Fatal(err)
	}
	db, err := store.dbOrEnsure()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove("test:append-unexpected-account-identity")
	})
	if err := db.Callback().Query().After("gorm:query").Register("test:append-unexpected-account-identity", func(query *gorm.DB) {
		if identities, ok := query.Statement.Dest.(*[]oauthIdentityRecord); ok {
			*identities = append(*identities, oauthIdentityRecord{
				ID:            999,
				AccountID:     999,
				OAuthProvider: "github",
				OAuthSubject:  "unexpected",
				OAuthLogin:    "unexpected",
			})
		}
	}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := store.ListAccounts(AccountListOptions{Limit: 10}); err == nil || !strings.Contains(err.Error(), "unexpected account") {
		t.Fatalf("expected unexpected identity account error, got %v", err)
	}
}

func TestGetAccountStatsCountsAccountsRolesAndIdentities(t *testing.T) {
	store := New(t.TempDir()).(*DBStore)
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
	db, err := store.dbOrEnsure()
	if err != nil {
		t.Fatal(err)
	}
	queryCount := 0
	countQuery := func(*gorm.DB) {
		queryCount++
	}
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove("test:count-account-stats-query")
		_ = db.Callback().Row().Remove("test:count-account-stats-row")
	})
	if err := db.Callback().Query().Before("gorm:query").Register("test:count-account-stats-query", countQuery); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Row().Before("gorm:row").Register("test:count-account-stats-row", countQuery); err != nil {
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
	if queryCount != 2 {
		t.Fatalf("GetAccountStats executed %d queries, want 2", queryCount)
	}
}

func TestListAccountsRejectsInvalidRole(t *testing.T) {
	store := New(t.TempDir())
	if _, _, err := store.ListAccounts(AccountListOptions{Role: "owner", Limit: 10}); err == nil {
		t.Fatal("expected invalid role filter to fail")
	}
}

func TestUpdateAccountRoleValidatesAndPersists(t *testing.T) {
	store := New(t.TempDir())
	actor, _, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "admin", OAuthLogin: "admin"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	account, _, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "12345", OAuthLogin: "octocat"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	fetched, err := store.GetAccount(account.ID)
	if err != nil || fetched.ID != account.ID || fetched.Role != "user" {
		t.Fatalf("unexpected fetched account: account=%#v err=%v", fetched, err)
	}

	updated, err := store.UpdateAccountRoleWithAudit(AccountRoleUpdate{
		ActorAccountID: actor.ID,
		AccountID:      account.ID,
		Role:           " ADMIN ",
		AuditActor:     "github:admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != account.ID || updated.Role != "admin" || !updated.UpdatedAt.After(account.UpdatedAt) {
		t.Fatalf("unexpected updated account: before=%#v after=%#v", account, updated)
	}
	persisted, _, err := store.GetAccountByOAuthIdentity("github", "12345")
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Role != "admin" {
		t.Fatalf("expected persisted admin role, got %#v", persisted)
	}
	if _, err := store.UpdateAccountRoleWithAudit(AccountRoleUpdate{ActorAccountID: actor.ID, AccountID: account.ID, Role: "owner", AuditActor: "github:admin"}); err == nil {
		t.Fatal("expected invalid role update to fail")
	}
	if _, err := store.UpdateAccountRoleWithAudit(AccountRoleUpdate{ActorAccountID: actor.ID, AccountID: 0, Role: "user", AuditActor: "github:admin"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected missing account to return ErrNotFound, got %v", err)
	}
	if _, err := store.GetAccount(0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected missing account lookup to return ErrNotFound, got %v", err)
	}
}

func TestUpdateAccountRoleWithAuditSkipsNoOpChanges(t *testing.T) {
	store := New(t.TempDir())
	actor, _, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "actor", OAuthLogin: "actor"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	target, _, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "target", OAuthLogin: "target"}, "user")
	if err != nil {
		t.Fatal(err)
	}

	updated, err := store.UpdateAccountRoleWithAudit(AccountRoleUpdate{
		ActorAccountID: actor.ID,
		AccountID:      target.ID,
		Role:           "user",
		AuditActor:     "github:actor",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated != target {
		t.Fatalf("expected no-op role update to return the unchanged account: before=%#v after=%#v", target, updated)
	}
	events, err := store.ListAuditEvents(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no audit event for a no-op role update, got %#v", events)
	}
}

func TestUpdateAccountRoleWithAuditRollsBackWhenAuditInsertFails(t *testing.T) {
	store := New(t.TempDir()).(*DBStore)
	actor, _, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "actor", OAuthLogin: "actor"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	target, _, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "target", OAuthLogin: "target"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.dbOrEnsure()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER fail_account_role_audit
		BEFORE INSERT ON audit_events
		WHEN NEW.action = 'account.role.update'
		BEGIN
			SELECT RAISE(ABORT, 'forced audit failure');
		END`).Error; err != nil {
		t.Fatal(err)
	}

	_, err = store.UpdateAccountRoleWithAudit(AccountRoleUpdate{
		ActorAccountID: actor.ID,
		AccountID:      target.ID,
		Role:           "admin",
		AuditActor:     "github:actor",
	})
	if err == nil || !strings.Contains(err.Error(), "forced audit failure") {
		t.Fatalf("expected forced audit failure, got %v", err)
	}
	persisted, err := store.GetAccount(target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Role != "user" {
		t.Fatalf("expected failed audit insert to roll back role update, got %#v", persisted)
	}
}

func TestConcurrentAccountRoleUpdatesKeepAnAdministrator(t *testing.T) {
	store := New(t.TempDir()).(*DBStore)
	first, _, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "first", OAuthLogin: "first"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "second", OAuthLogin: "second"}, "admin")
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errorsByUpdate := make(chan error, 2)
	updates := []AccountRoleUpdate{
		{ActorAccountID: first.ID, AccountID: second.ID, Role: "user", AuditActor: "github:first"},
		{ActorAccountID: second.ID, AccountID: first.ID, Role: "user", AuditActor: "github:second"},
	}
	for _, update := range updates {
		update := update
		go func() {
			<-start
			_, updateErr := store.UpdateAccountRoleWithAudit(update)
			errorsByUpdate <- updateErr
		}()
	}
	close(start)
	firstErr := <-errorsByUpdate
	secondErr := <-errorsByUpdate
	if (firstErr == nil) == (secondErr == nil) {
		t.Fatalf("expected exactly one role update to succeed, got errors %v and %v", firstErr, secondErr)
	}
	failedErr := firstErr
	if failedErr == nil {
		failedErr = secondErr
	}
	if !errors.Is(failedErr, ErrConflict) {
		t.Fatalf("expected losing role update to return ErrConflict, got %v", failedErr)
	}
	stats, err := store.GetAccountStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.AdminAccounts != 1 {
		t.Fatalf("expected one administrator to remain, got %#v", stats)
	}
}

func TestGetOAuthIdentityForAccountByProvider(t *testing.T) {
	store := New(t.TempDir())
	account, _, err := store.UpsertAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "12345", OAuthLogin: "octocat"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LinkOAuthIdentityToAccount(account.ID, OAuthIdentity{OAuthProvider: "gitlab", OAuthSubject: "abcde", OAuthLogin: "octocat-gitlab"}); err != nil {
		t.Fatal(err)
	}

	identity, err := store.GetOAuthIdentityForAccount(account.ID, " GITHUB ")
	if err != nil {
		t.Fatal(err)
	}
	if identity.AccountID != account.ID || identity.OAuthProvider != "github" || identity.OAuthLogin != "octocat" {
		t.Fatalf("unexpected github identity: %#v", identity)
	}
	if _, err := store.GetOAuthIdentityForAccount(account.ID, "missing"); err != ErrNotFound {
		t.Fatalf("expected missing provider identity to return ErrNotFound, got %v", err)
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

func TestMigrateAddsGitHubInstallationLookupIndex(t *testing.T) {
	databaseURL := t.TempDir() + "/runnerd.db"
	store := NewWithOptions(Options{
		Backend:        BackendSQLite,
		DatabaseDSN:    databaseURL,
		MigrateOnStart: false,
	}).(*DBStore)
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE github_installations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER NOT NULL,
		installation_id INTEGER NOT NULL,
		github_account_id INTEGER,
		account_type TEXT,
		account_login TEXT NOT NULL,
		account_name TEXT,
		account_avatar TEXT,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE UNIQUE INDEX idx_github_installations_account_installation ON github_installations (account_id, installation_id)`).Error; err != nil {
		t.Fatal(err)
	}
	closeTestDB(t, db)

	migrated := NewWithOptions(Options{
		Backend:        BackendSQLite,
		DatabaseDSN:    databaseURL,
		MigrateOnStart: true,
	}).(*DBStore)
	migratedDB, err := migrated.dbOrEnsure()
	if err != nil {
		t.Fatal(err)
	}
	if !migratedDB.Migrator().HasIndex(&githubInstallationRecord{}, "idx_github_installations_installation") {
		t.Fatal("expected installation_id lookup index after migration")
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

func TestListStatesForGitHubInstallationRepositoriesPreservesPairs(t *testing.T) {
	store := New(t.TempDir())
	requests := []RunnerRequest{
		{ID: "first-visible", Source: "test", GitHubInstallationID: 987, RepositoryFullName: "o/first"},
		{ID: "first-cross-pair", Source: "test", GitHubInstallationID: 987, RepositoryFullName: "other/second"},
		{ID: "second-visible", Source: "test", GitHubInstallationID: 654, RepositoryFullName: "other/second"},
		{ID: "second-cross-pair", Source: "test", GitHubInstallationID: 654, RepositoryFullName: "o/first"},
	}
	for _, request := range requests {
		if _, _, err := store.CreateRequest(request, nil); err != nil {
			t.Fatal(err)
		}
	}

	states, err := store.ListStatesForGitHubInstallationRepositories([]GitHubInstallationRepositoryAccess{
		{InstallationID: 987, Repositories: []string{"o/first"}},
		{InstallationID: 654, Repositories: []string{"other/second"}},
	}, 10)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(states))
	for _, runnerState := range states {
		ids = append(ids, runnerState.ID)
	}
	sort.Strings(ids)
	if !reflect.DeepEqual(ids, []string{"first-visible", "second-visible"}) {
		t.Fatalf("unexpected pair-filtered states: %#v", ids)
	}
}

func TestListStatesForGitHubInstallationRepositoriesMatchesRepositoryCaseInsensitively(t *testing.T) {
	store := New(t.TempDir())
	if _, _, err := store.CreateRequest(RunnerRequest{
		ID:                   "visible",
		Source:               "test",
		GitHubInstallationID: 987,
		RepositoryFullName:   "Octo/Visible",
	}, nil); err != nil {
		t.Fatal(err)
	}

	states, err := store.ListStatesForGitHubInstallationRepositories([]GitHubInstallationRepositoryAccess{
		{InstallationID: 987, Repositories: []string{"octo/visible"}},
	}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].ID != "visible" {
		t.Fatalf("expected case-insensitive repository match, got %#v", states)
	}
}

func TestListStatesForGitHubInstallationRepositoriesFiltersBeforeLimit(t *testing.T) {
	store := New(t.TempDir())
	now := time.Now().UTC()
	for _, request := range []RunnerRequest{
		{
			ID:                   "authorized-older",
			Source:               "test",
			GitHubInstallationID: 987,
			RepositoryFullName:   "o/visible",
			CreatedAt:            now.Add(-time.Minute),
		},
		{
			ID:                   "unauthorized-newer",
			Source:               "test",
			GitHubInstallationID: 987,
			RepositoryFullName:   "o/hidden",
			CreatedAt:            now,
		},
	} {
		if _, _, err := store.CreateRequest(request, nil); err != nil {
			t.Fatal(err)
		}
	}

	states, err := store.ListStatesForGitHubInstallationRepositories([]GitHubInstallationRepositoryAccess{
		{InstallationID: 987, Repositories: []string{"o/visible"}},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].ID != "authorized-older" {
		t.Fatalf("expected authorization filter before limit, got %#v", states)
	}
}

func TestGitHubInstallationRepositoryAccessQueryBatchesStayWithinParameterLimit(t *testing.T) {
	repositories := make([]string, 0, 1200)
	for index := range 1200 {
		repositories = append(repositories, fmt.Sprintf("o/repo-%d", index))
	}

	batches := githubInstallationRepositoryAccessQueryBatches([]GitHubInstallationRepositoryAccess{
		{InstallationID: 987, Repositories: repositories},
	}, maxRunnerRequestRepositoryAccessQueryParameters)
	if len(batches) < 2 {
		t.Fatalf("expected large repository access to be split, got %d batch", len(batches))
	}
	for index, batch := range batches {
		if batch.parameterCount > maxRunnerRequestRepositoryAccessQueryParameters {
			t.Fatalf("batch %d has %d parameters, limit %d", index, batch.parameterCount, maxRunnerRequestRepositoryAccessQueryParameters)
		}
	}
}

func TestListStatesForGitHubInstallationRepositoriesHandlesLargeAccessSet(t *testing.T) {
	store := New(t.TempDir())
	now := time.Now().UTC()
	for _, request := range []RunnerRequest{
		{
			ID:                   "older-in-first-batch",
			Source:               "test",
			GitHubInstallationID: 987,
			RepositoryFullName:   "o/repo-0",
			CreatedAt:            now.Add(-time.Minute),
		},
		{
			ID:                   "newer-in-later-batch",
			Source:               "test",
			GitHubInstallationID: 987,
			RepositoryFullName:   "o/repo-1199",
			CreatedAt:            now,
		},
	} {
		if _, _, err := store.CreateRequest(request, nil); err != nil {
			t.Fatal(err)
		}
	}
	repositories := make([]string, 0, 1200)
	for index := range 1200 {
		repositories = append(repositories, fmt.Sprintf("o/repo-%d", index))
	}

	states, err := store.ListStatesForGitHubInstallationRepositories([]GitHubInstallationRepositoryAccess{
		{InstallationID: 987, Repositories: repositories},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].ID != "newer-in-later-batch" {
		t.Fatalf("expected global newest state from a later query batch, got %#v", states)
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

func TestGitHubInstallationAccountIdentityRoundTripAndLookup(t *testing.T) {
	store := New(t.TempDir())
	first, _, err := store.EnsureAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "100", OAuthLogin: "alice"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := store.EnsureAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "200", OAuthLogin: "bob"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	for _, accountID := range []int64{first.ID, second.ID} {
		if _, err := store.UpsertGitHubInstallation(GitHubInstallation{
			AccountID:       accountID,
			InstallationID:  987,
			GitHubAccountID: 9001,
			AccountType:     "Organization",
			AccountLogin:    "Octo-Org",
			AccountName:     "Octo Org",
			AccountAvatar:   "https://avatars.example/o.png",
		}); err != nil {
			t.Fatal(err)
		}
	}

	installations, err := store.ListGitHubInstallations(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(installations) != 1 || installations[0].GitHubAccountID != 9001 || installations[0].AccountType != "organization" {
		t.Fatalf("unexpected installation identity: %#v", installations)
	}
	accounts, err := store.ListGitHubInstallationAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 || accounts[0].GitHubAccountID != 9001 || accounts[0].AccountLogin != "Octo-Org" {
		t.Fatalf("unexpected installation accounts: %#v", accounts)
	}
	byInstallation, err := store.GitHubInstallationAccountForInstallation(987)
	if err != nil || byInstallation.GitHubAccountID != 9001 {
		t.Fatalf("lookup by installation: account=%#v err=%v", byInstallation, err)
	}
	byLogin, err := store.GitHubInstallationAccountForLogin("octo-org")
	if err != nil || byLogin.GitHubAccountID != 9001 {
		t.Fatalf("lookup by login: account=%#v err=%v", byLogin, err)
	}
}

func TestUpsertGitHubInstallationRollsBackWhenOwnerCacheFails(t *testing.T) {
	store := New(t.TempDir()).(*DBStore)
	account, _, err := store.EnsureAccountForOAuthIdentity(OAuthIdentity{OAuthProvider: "github", OAuthSubject: "100", OAuthLogin: "alice"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.dbOrEnsure()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropTable(&githubInstallationOwnerRecord{}); err != nil {
		t.Fatal(err)
	}

	if _, err := store.UpsertGitHubInstallation(GitHubInstallation{
		AccountID:       account.ID,
		InstallationID:  987,
		GitHubAccountID: 9001,
		AccountType:     "organization",
		AccountLogin:    "octo-org",
	}); err == nil {
		t.Fatal("UpsertGitHubInstallation() error = nil, want owner cache failure")
	}
	var count int64
	if err := db.Model(&githubInstallationRecord{}).
		Where("account_id = ? AND installation_id = ?", account.ID, 987).
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("installation rows = %d, want transaction rollback", count)
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

func TestMigratePreservesAdditiveRunnerRequestColumns(t *testing.T) {
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
	if err := db.Exec(`CREATE TABLE runner_requests (
		id TEXT PRIMARY KEY,
		source TEXT NOT NULL,
		workflow_job_id INTEGER,
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
		queued_at DATETIME NOT NULL,
		last_attempt_at DATETIME,
		next_retry_at DATETIME,
		creating_at DATETIME,
		running_at DATETIME,
		stopping_at DATETIME,
		completed_at DATETIME,
		failed_at DATETIME,
		lease_owner TEXT,
		lease_expires_at DATETIME,
		updated_at DATETIME NOT NULL,
		version INTEGER NOT NULL DEFAULT 0
	);`).Error; err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"ALTER TABLE runner_requests ADD COLUMN github_installation_id INTEGER",
		"ALTER TABLE runner_requests ADD COLUMN workflow_run_id INTEGER",
		"ALTER TABLE runner_requests ADD COLUMN workflow_name TEXT",
		"ALTER TABLE runner_requests ADD COLUMN workflow_run_attempt INTEGER",
		"ALTER TABLE runner_requests ADD COLUMN head_branch TEXT",
		"ALTER TABLE runner_requests ADD COLUMN head_sha TEXT",
		"ALTER TABLE runner_requests ADD COLUMN github_job_url TEXT",
		"ALTER TABLE runner_requests ADD COLUMN pull_request_number INTEGER",
		"ALTER TABLE runner_requests ADD COLUMN github_context_backfilled NUMERIC NOT NULL DEFAULT FALSE",
		"ALTER TABLE runner_requests ADD COLUMN sandbox_api_url TEXT",
		"ALTER TABLE runner_requests ADD COLUMN sandbox_api_key_encrypted TEXT",
		"ALTER TABLE runner_requests ADD COLUMN sandbox_config_source TEXT",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	originalUpdatedAt := time.Date(2026, time.July, 15, 15, 49, 3, 0, time.UTC)
	payload := `{"installation":{"id":135340026},"workflow_job":{"run_id":29401048027,"workflow_name":"CI Check And Test","run_attempt":2,"head_branch":"feature/migration-integrity","head_sha":"abc123"}}`
	if err := db.Exec(
		`INSERT INTO runner_requests (
		id, source, workflow_job_id, repository_full_name, requested_labels_json,
		labels_json, profile_name, runner_group, runner_name, status,
		github_payload_json, queued_at, updated_at, version,
		github_installation_id, workflow_run_id, workflow_name, workflow_run_attempt,
		head_branch, head_sha, github_job_url, github_context_backfilled,
		sandbox_api_url, sandbox_api_key_encrypted, sandbox_config_source
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"87401142867", "github_webhook", 87401142867, "qbox/las", `["github-runner-ubuntu-24-04"]`,
		`["self-hosted","e2b","github-runner-ubuntu-24-04"]`, "github-runner-ubuntu-24-04", "Default",
		"e2b-87401142867", StatusCompleted, payload, originalUpdatedAt.Add(-time.Minute), originalUpdatedAt, 7,
		135340026, 29401048027, "CI Check And Test", 2, "feature/migration-integrity", "abc123",
		"https://github.com/qbox/las/actions/runs/29401048027/job/87401142867", true,
		"https://sandbox.example.test", "encrypted-api-key", "admin_default",
	).Error; err != nil {
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
	var record runnerRequestRecord
	if err := db.First(&record, "id = ?", "87401142867").Error; err != nil {
		t.Fatal(err)
	}
	if record.GitHubInstallationID != 135340026 {
		t.Fatalf("github installation id changed during migration: %d", record.GitHubInstallationID)
	}
	if record.SandboxAPIURL != "https://sandbox.example.test" ||
		record.SandboxAPIKeyEncrypted != "encrypted-api-key" ||
		record.SandboxConfigSource != "admin_default" {
		t.Fatalf("sandbox configuration changed during migration: %#v", record)
	}
	if !record.UpdatedAt.Equal(originalUpdatedAt) {
		t.Fatalf("updated_at changed during migration: got %s want %s", record.UpdatedAt, originalUpdatedAt)
	}
}

func TestMigrateRepairsMissingRunnerRequestInstallationID(t *testing.T) {
	dir := t.TempDir()
	databaseURL := dir + "/runnerd.db"
	store := NewWithOptions(Options{
		Backend:        BackendSQLite,
		DatabaseDSN:    databaseURL,
		MigrateOnStart: true,
	}).(*DBStore)
	db, err := store.dbOrEnsure()
	if err != nil {
		t.Fatal(err)
	}
	workflowJobID := int64(87401142867)
	originalUpdatedAt := time.Date(2026, time.July, 15, 15, 49, 3, 0, time.UTC)
	record := runnerRequestRecord{
		ID:                      "87401142867",
		Source:                  "github_webhook",
		WorkflowJobID:           &workflowJobID,
		GitHubInstallationID:    0,
		WorkflowRunID:           29401048027,
		WorkflowName:            "CI Check And Test",
		WorkflowRunAttempt:      2,
		HeadBranch:              "feature/migration-integrity",
		HeadSHA:                 "abc123",
		GitHubContextBackfilled: true,
		RepositoryFullName:      "qbox/las",
		RequestedLabelsJSON:     `["github-runner-ubuntu-24-04"]`,
		LabelsJSON:              `["self-hosted","e2b","github-runner-ubuntu-24-04"]`,
		ProfileName:             "github-runner-ubuntu-24-04",
		RunnerGroup:             "Default",
		RunnerName:              "e2b-87401142867",
		Status:                  StatusCompleted,
		GitHubPayloadJSON:       `{"installation":{"id":135340026},"workflow_job":{"run_id":29401048027}}`,
		QueuedAt:                originalUpdatedAt.Add(-time.Minute),
		UpdatedAt:               originalUpdatedAt,
		Version:                 7,
	}
	if err := db.Create(&record).Error; err != nil {
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
	var repaired runnerRequestRecord
	if err := db.First(&repaired, "id = ?", record.ID).Error; err != nil {
		t.Fatal(err)
	}
	if repaired.GitHubInstallationID != 135340026 {
		t.Fatalf("github installation id was not repaired: %d", repaired.GitHubInstallationID)
	}
	if !repaired.UpdatedAt.Equal(originalUpdatedAt) {
		t.Fatalf("updated_at changed during repair: got %s want %s", repaired.UpdatedAt, originalUpdatedAt)
	}
}

func TestMigrateDoesNotRewriteUnrecoverableRunnerRequestInstallationID(t *testing.T) {
	dir := t.TempDir()
	databaseURL := dir + "/runnerd.db"
	store := NewWithOptions(Options{
		Backend:        BackendSQLite,
		DatabaseDSN:    databaseURL,
		MigrateOnStart: true,
	}).(*DBStore)
	db, err := store.dbOrEnsure()
	if err != nil {
		t.Fatal(err)
	}
	workflowJobID := int64(87401142868)
	now := time.Now().UTC()
	record := runnerRequestRecord{
		ID:                      "87401142868",
		Source:                  "github_webhook",
		WorkflowJobID:           &workflowJobID,
		GitHubContextBackfilled: true,
		RepositoryFullName:      "qbox/las",
		RequestedLabelsJSON:     `["github-runner-ubuntu-24-04"]`,
		LabelsJSON:              `["self-hosted","e2b","github-runner-ubuntu-24-04"]`,
		ProfileName:             "github-runner-ubuntu-24-04",
		RunnerGroup:             "Default",
		RunnerName:              "e2b-87401142868",
		Status:                  StatusCompleted,
		GitHubPayloadJSON:       `{"installation":null}`,
		QueuedAt:                now.Add(-time.Minute),
		UpdatedAt:               now,
	}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_unrecoverable_installation_rewrite
		BEFORE UPDATE ON runner_requests
		WHEN OLD.id = '87401142868'
		BEGIN
			SELECT RAISE(ABORT, 'unrecoverable installation row was rewritten');
		END`).Error; err != nil {
		t.Fatal(err)
	}
	closeTestDB(t, db)

	for start := 1; start <= 2; start++ {
		migrated := NewWithOptions(Options{
			Backend:        BackendSQLite,
			DatabaseDSN:    databaseURL,
			MigrateOnStart: true,
		}).(*DBStore)
		db, err = migrated.dbOrEnsure()
		if err != nil {
			t.Fatalf("migration start %d rewrote unrecoverable installation row: %v", start, err)
		}
		closeTestDB(t, db)
	}
}

func TestMigrateSQLiteRunnerRequestSnapshot(t *testing.T) {
	sourcePath := strings.TrimSpace(os.Getenv("RUNNERD_SQLITE_SNAPSHOT"))
	if sourcePath == "" {
		t.Skip("set RUNNERD_SQLITE_SNAPSHOT to verify a disposable copy of a production SQLite database")
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	databaseURL := filepath.Join(t.TempDir(), "runnerd.db")
	destination, err := os.Create(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(destination, source); err != nil {
		destination.Close()
		t.Fatal(err)
	}
	if err := destination.Close(); err != nil {
		t.Fatal(err)
	}

	type counts struct {
		Total                            int64 `gorm:"column:total"`
		GitHubInstallationIDs            int64 `gorm:"column:github_installation_ids"`
		RecoverableGitHubInstallationIDs int64 `gorm:"column:recoverable_github_installation_ids"`
		SandboxAPIURLs                   int64 `gorm:"column:sandbox_api_urls"`
		SandboxAPIKeys                   int64 `gorm:"column:sandbox_api_keys"`
		SandboxConfigSources             int64 `gorm:"column:sandbox_config_sources"`
	}
	readCounts := func(db *gorm.DB) counts {
		t.Helper()
		var result counts
		if err := db.Raw(`SELECT
			COUNT(*) AS total,
			SUM(CASE WHEN github_installation_id > 0 THEN 1 ELSE 0 END) AS github_installation_ids,
			SUM(CASE
				WHEN github_installation_id > 0 THEN 0
				WHEN json_valid(github_payload_json) THEN
					CASE WHEN CAST(json_extract(github_payload_json, '$.installation.id') AS INTEGER) > 0 THEN 1 ELSE 0 END
				ELSE 0
			END) AS recoverable_github_installation_ids,
			SUM(CASE WHEN sandbox_api_url <> '' THEN 1 ELSE 0 END) AS sandbox_api_urls,
			SUM(CASE WHEN sandbox_api_key_encrypted <> '' THEN 1 ELSE 0 END) AS sandbox_api_keys,
			SUM(CASE WHEN sandbox_config_source <> '' THEN 1 ELSE 0 END) AS sandbox_config_sources
			FROM runner_requests`).Scan(&result).Error; err != nil {
			t.Fatal(err)
		}
		return result
	}
	openStore := func(migrate bool) *gorm.DB {
		t.Helper()
		store := NewWithOptions(Options{
			Backend:        BackendSQLite,
			DatabaseDSN:    databaseURL,
			MigrateOnStart: migrate,
		}).(*DBStore)
		db, err := store.dbOrEnsure()
		if err != nil {
			t.Fatal(err)
		}
		return db
	}

	db := openStore(false)
	before := readCounts(db)
	closeTestDB(t, db)

	db = openStore(true)
	afterFirstStart := readCounts(db)
	closeTestDB(t, db)
	if afterFirstStart.Total != before.Total {
		t.Fatalf("runner request count changed after migration: before=%d after=%d", before.Total, afterFirstStart.Total)
	}
	expectedGitHubInstallationIDs := before.GitHubInstallationIDs + before.RecoverableGitHubInstallationIDs
	if afterFirstStart.GitHubInstallationIDs != expectedGitHubInstallationIDs ||
		afterFirstStart.RecoverableGitHubInstallationIDs != 0 {
		t.Fatalf(
			"github installation ids were not fully repaired: before=%#v after=%#v expected_ids=%d",
			before,
			afterFirstStart,
			expectedGitHubInstallationIDs,
		)
	}
	if afterFirstStart.SandboxAPIURLs != before.SandboxAPIURLs ||
		afterFirstStart.SandboxAPIKeys != before.SandboxAPIKeys ||
		afterFirstStart.SandboxConfigSources != before.SandboxConfigSources {
		t.Fatalf("sandbox snapshot counts changed after migration: before=%#v after=%#v", before, afterFirstStart)
	}

	db = openStore(true)
	afterSecondStart := readCounts(db)
	closeTestDB(t, db)
	if afterSecondStart != afterFirstStart {
		t.Fatalf("second migration changed runner request data: first=%#v second=%#v", afterFirstStart, afterSecondStart)
	}
	t.Logf("runner request migration counts: before=%#v after=%#v", before, afterSecondStart)
}
