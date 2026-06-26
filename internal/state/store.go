package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/qiniu/ci-runner/internal/labelutil"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
)

const (
	StatusQueued    = "queued"
	StatusCreating  = "creating"
	StatusRunning   = "running"
	StatusStopping  = "stopping"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

const (
	BackendSQLite   = "sqlite"
	BackendPostgres = "postgres"
)

var (
	ErrConflict        = errors.New("state conflict")
	ErrNotFound        = errors.New("state record not found")
	ErrRetryNotAllowed = errors.New("retry not allowed for current state")
)

type RunnerRequest struct {
	ID                 string    `json:"id"`
	Source             string    `json:"source"`
	JobID              int64     `json:"job_id,omitempty"`
	RepositoryFullName string    `json:"repository_full_name,omitempty"`
	RequestedLabels    []string  `json:"requested_labels,omitempty"`
	Labels             []string  `json:"labels"`
	ProfileName        string    `json:"runner_spec_name,omitempty"`
	RunnerGroup        string    `json:"runner_group,omitempty"`
	RunnerName         string    `json:"runner_name"`
	CreatedAt          time.Time `json:"created_at"`
}

type RunnerState struct {
	ID                 string    `json:"id"`
	Status             string    `json:"status"`
	RepositoryFullName string    `json:"repository_full_name,omitempty"`
	RequestedLabels    []string  `json:"requested_labels,omitempty"`
	ProfileName        string    `json:"runner_spec_name,omitempty"`
	RunnerGroup        string    `json:"runner_group,omitempty"`
	RunnerName         string    `json:"runner_name"`
	SandboxID          string    `json:"sandbox_id,omitempty"`
	ProcessPID         uint32    `json:"process_pid,omitempty"`
	WorkflowJobID      int64     `json:"workflow_job_id,omitempty"`
	WorkflowRunID      int64     `json:"workflow_run_id,omitempty"`
	GitHubJobURL       string    `json:"github_job_url,omitempty"`
	PullRequestNumber  int64     `json:"pull_request_number,omitempty"`
	AssignedJobID      int64     `json:"assigned_job_id,omitempty"`
	AssignedJobName    string    `json:"assigned_job_name,omitempty"`
	Error              string    `json:"error,omitempty"`
	FailureStage       string    `json:"failure_stage,omitempty"`
	FailureReason      string    `json:"failure_reason,omitempty"`
	LastErrorCode      string    `json:"last_error_code,omitempty"`
	LastErrorMessage   string    `json:"last_error_message,omitempty"`
	LastErrorRetryable bool      `json:"last_error_retryable,omitempty"`
	RetryCount         int       `json:"retry_count,omitempty"`
	UpdatedAt          time.Time `json:"updated_at"`
	CreatedAt          time.Time `json:"created_at"`
	LastAttemptAt      time.Time `json:"last_attempt_at,omitempty"`
	NextRetryAt        time.Time `json:"next_retry_at,omitempty"`
	CreatingAt         time.Time `json:"creating_at,omitempty"`
	RunningAt          time.Time `json:"running_at,omitempty"`
	StoppingAt         time.Time `json:"stopping_at,omitempty"`
	CompletedAt        time.Time `json:"completed_at,omitempty"`
	FailedAt           time.Time `json:"failed_at,omitempty"`
	LeaseOwner         string    `json:"lease_owner,omitempty"`
	LeaseExpiresAt     time.Time `json:"lease_expires_at,omitempty"`
	Version            int64     `json:"version"`
}

type RunnerProfile struct {
	Name             string    `json:"name"`
	Labels           []string  `json:"labels"`
	TemplateID       string    `json:"template_id"`
	RunnerGroup      string    `json:"runner_group,omitempty"`
	MaxConcurrency   int       `json:"max_concurrency"`
	MinIdle          int       `json:"min_idle"`
	Priority         int       `json:"priority"`
	Enabled          bool      `json:"enabled"`
	DefaultAvailable bool      `json:"default_available"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type RunnerGroup struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	SpecNames   []string  `json:"spec_names"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type RepositoryPolicy struct {
	ID                 int64     `json:"id"`
	RepositoryFullName string    `json:"repository_full_name"`
	ProfileName        string    `json:"runner_spec_name,omitempty"`
	RunnerGroupName    string    `json:"runner_group_name,omitempty"`
	Enabled            bool      `json:"enabled"`
	CreatedAt          time.Time `json:"created_at"`
}

type ProfileMatch struct {
	RepositoryFullName string         `json:"repository_full_name"`
	Labels             []string       `json:"labels"`
	Profile            *RunnerProfile `json:"runner_spec,omitempty"`
	Reason             string         `json:"reason,omitempty"`
}

type AuditEvent struct {
	ID           int64     `json:"id"`
	Actor        string    `json:"actor"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	PayloadJSON  string    `json:"payload_json,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// User represents a platform user authenticated through an OAuth provider.
type User struct {
	ID            int64     `json:"id"`
	OAuthProvider string    `json:"oauth_provider"`
	OAuthSubject  string    `json:"oauth_subject"`
	OAuthLogin    string    `json:"oauth_login"`
	Role          string    `json:"role"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Store interface {
	Ensure() error
	CreateRequest(req RunnerRequest, payload []byte) (bool, RunnerState, error)
	ReadRequest(id string) (RunnerRequest, error)
	ReadState(id string) (RunnerState, error)
	WriteState(st RunnerState) error
	ListStates() ([]RunnerState, error)
	ListActiveStates() ([]RunnerState, error)
	ListMismatchedCompletedStates(limit int) ([]RunnerState, error)
	ListFailedWorkflowJobStates(limit int) ([]RunnerState, error)
	ListStatesPage(limit, offset int) ([]RunnerState, int64, error)
	ActiveCount() (int, error)
	InFlightCount() (int, error)
	ActiveCountForProfile(name string) (int, error)
	InFlightCountForProfile(name string) (int, error)
	ClaimNextRunnable(workerID string, now time.Time, leaseTTL time.Duration) (RunnerRequest, RunnerState, bool, error)
	ReleaseLease(id, workerID string) error
	RetryRequest(id string, now time.Time) (RunnerState, error)
	AppendLog(id, name string, data []byte)
	ReadLog(id, name string, maxBytes int64) ([]byte, error)
	ListProfiles() ([]RunnerProfile, error)
	GetProfile(name string) (RunnerProfile, error)
	UpsertProfile(profile RunnerProfile) (RunnerProfile, error)
	DeleteProfile(name string) error
	ListRunnerGroups() ([]RunnerGroup, error)
	GetRunnerGroup(name string) (RunnerGroup, error)
	UpsertRunnerGroup(group RunnerGroup) (RunnerGroup, error)
	DeleteRunnerGroup(name string) error
	ListRepositoryPolicies() ([]RepositoryPolicy, error)
	GetRepositoryPolicy(id int64) (RepositoryPolicy, error)
	UpsertRepositoryPolicy(policy RepositoryPolicy) (RepositoryPolicy, error)
	DeleteRepositoryPolicy(id int64) error
	MatchProfile(repositoryFullName string, labels []string) (ProfileMatch, error)
	GetUserByOAuthIdentity(provider, subject string) (User, error)
	EnsureUser(user User) (User, error)
	UpsertUser(user User) (User, error)
	AppendAuditEvent(event AuditEvent) (AuditEvent, error)
	ListAuditEvents(limit int) ([]AuditEvent, error)
}

type Options struct {
	Backend        string
	DatabaseURL    string
	MigrateOnStart bool
}

type DBStore struct {
	opts     Options
	mu       sync.Mutex
	db       *gorm.DB
	migrated bool
}

type runnerRequestRecord struct {
	ID                  string     `gorm:"column:id;primaryKey"`
	Source              string     `gorm:"column:source"`
	WorkflowJobID       *int64     `gorm:"column:workflow_job_id"`
	RepositoryFullName  string     `gorm:"column:repository_full_name"`
	RequestedLabelsJSON string     `gorm:"column:requested_labels_json"`
	LabelsJSON          string     `gorm:"column:labels_json"`
	ProfileName         string     `gorm:"column:profile_name"`
	RunnerGroup         string     `gorm:"column:runner_group"`
	RunnerName          string     `gorm:"column:runner_name"`
	Status              string     `gorm:"column:status"`
	FailureStage        string     `gorm:"column:failure_stage"`
	FailureReason       string     `gorm:"column:failure_reason"`
	LastErrorCode       string     `gorm:"column:last_error_code"`
	LastErrorMessage    string     `gorm:"column:last_error_message"`
	LastErrorRetryable  bool       `gorm:"column:last_error_retryable"`
	RetryCount          int        `gorm:"column:retry_count"`
	SandboxID           string     `gorm:"column:sandbox_id"`
	ProcessPID          uint32     `gorm:"column:process_pid"`
	AssignedJobID       int64      `gorm:"column:assigned_job_id"`
	AssignedJobName     string     `gorm:"column:assigned_job_name"`
	Error               string     `gorm:"column:error"`
	GitHubPayloadJSON   string     `gorm:"column:github_payload_json"`
	QueuedAt            time.Time  `gorm:"column:queued_at"`
	LastAttemptAt       *time.Time `gorm:"column:last_attempt_at"`
	NextRetryAt         *time.Time `gorm:"column:next_retry_at"`
	CreatingAt          *time.Time `gorm:"column:creating_at"`
	RunningAt           *time.Time `gorm:"column:running_at"`
	StoppingAt          *time.Time `gorm:"column:stopping_at"`
	CompletedAt         *time.Time `gorm:"column:completed_at"`
	FailedAt            *time.Time `gorm:"column:failed_at"`
	LeaseOwner          string     `gorm:"column:lease_owner"`
	LeaseExpiresAt      *time.Time `gorm:"column:lease_expires_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at"`
	Version             int64      `gorm:"column:version"`
}

func (runnerRequestRecord) TableName() string { return "runner_requests" }

type runnerEventRecord struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement"`
	RequestID   string    `gorm:"column:request_id"`
	EventType   string    `gorm:"column:event_type"`
	Stage       string    `gorm:"column:stage"`
	Message     string    `gorm:"column:message"`
	PayloadJSON string    `gorm:"column:payload_json"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

func (runnerEventRecord) TableName() string { return "runner_events" }

type runnerProfileRecord struct {
	Name             string    `gorm:"column:name;primaryKey"`
	LabelsJSON       string    `gorm:"column:labels_json"`
	TemplateID       string    `gorm:"column:template_id"`
	RunnerGroup      string    `gorm:"column:runner_group"`
	MaxConcurrency   int       `gorm:"column:max_concurrency"`
	MinIdle          int       `gorm:"column:min_idle"`
	Priority         int       `gorm:"column:priority"`
	Enabled          bool      `gorm:"column:enabled"`
	DefaultAvailable bool      `gorm:"column:default_available"`
	CreatedAt        time.Time `gorm:"column:created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at"`
}

func (runnerProfileRecord) TableName() string { return "runner_profiles" }

type runnerGroupRecord struct {
	Name        string    `gorm:"column:name;primaryKey"`
	Description string    `gorm:"column:description"`
	Enabled     bool      `gorm:"column:enabled"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (runnerGroupRecord) TableName() string { return "runner_groups" }

type runnerGroupSpecRecord struct {
	GroupName string    `gorm:"column:group_name;primaryKey"`
	SpecName  string    `gorm:"column:spec_name;primaryKey"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (runnerGroupSpecRecord) TableName() string { return "runner_group_specs" }

type repositoryPolicyRecord struct {
	ID                 int64     `gorm:"column:id;primaryKey;autoIncrement"`
	RepositoryFullName string    `gorm:"column:repository_full_name"`
	ProfileName        string    `gorm:"column:profile_name"`
	RunnerGroupName    string    `gorm:"column:runner_group_name"`
	Enabled            bool      `gorm:"column:enabled"`
	CreatedAt          time.Time `gorm:"column:created_at"`
}

func (repositoryPolicyRecord) TableName() string { return "repository_policies" }

type auditEventRecord struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement"`
	Actor        string    `gorm:"column:actor"`
	Action       string    `gorm:"column:action"`
	ResourceType string    `gorm:"column:resource_type"`
	ResourceID   string    `gorm:"column:resource_id"`
	PayloadJSON  string    `gorm:"column:payload_json"`
	CreatedAt    time.Time `gorm:"column:created_at"`
}

func (auditEventRecord) TableName() string { return "audit_events" }

type userRecord struct {
	ID            int64     `gorm:"column:id;primaryKey;autoIncrement"`
	OAuthProvider string    `gorm:"column:oauth_provider"`
	OAuthSubject  string    `gorm:"column:oauth_subject"`
	OAuthLogin    string    `gorm:"column:oauth_login"`
	Role          string    `gorm:"column:role"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (userRecord) TableName() string { return "users" }

func New(dir string) Store {
	return NewWithOptions(Options{
		Backend:        BackendSQLite,
		DatabaseURL:    filepath.Join(dir, "runnerd.db"),
		MigrateOnStart: true,
	})
}

func NewWithOptions(opts Options) Store {
	backend := strings.ToLower(strings.TrimSpace(opts.Backend))
	if backend == "" {
		backend = BackendSQLite
	}
	if opts.DatabaseURL == "" && backend == BackendSQLite {
		opts.DatabaseURL = filepath.Join(".", "var", "runnerd.db")
	}
	opts.Backend = backend
	return &DBStore{opts: opts}
}

func (s *DBStore) Ensure() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return nil
	}

	db, err := s.open()
	if err != nil {
		return err
	}
	if s.opts.MigrateOnStart && !s.migrated {
		if err := s.migrate(db); err != nil {
			return err
		}
		s.migrated = true
	}
	s.db = db
	return nil
}

func (s *DBStore) CreateRequest(req RunnerRequest, payload []byte) (bool, RunnerState, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return false, RunnerState{}, err
	}

	req.ID = sanitizeID(req.ID)
	if req.ID == "" {
		return false, RunnerState{}, fmt.Errorf("request id is empty")
	}
	req.RunnerName = sanitizeRunnerName(req.RunnerName)
	if req.RunnerName == "" {
		req.RunnerName = "e2b-" + req.ID
	}
	now := time.Now().UTC()
	if req.CreatedAt.IsZero() {
		req.CreatedAt = now
	}

	if len(req.RequestedLabels) == 0 {
		req.RequestedLabels = append([]string(nil), req.Labels...)
	}
	labelsJSON, err := json.Marshal(req.Labels)
	if err != nil {
		return false, RunnerState{}, err
	}
	requestedLabelsJSON, err := json.Marshal(req.RequestedLabels)
	if err != nil {
		return false, RunnerState{}, err
	}
	record := runnerRequestRecord{
		ID:                  req.ID,
		Source:              req.Source,
		RepositoryFullName:  req.RepositoryFullName,
		RequestedLabelsJSON: string(requestedLabelsJSON),
		LabelsJSON:          string(labelsJSON),
		ProfileName:         req.ProfileName,
		RunnerGroup:         req.RunnerGroup,
		RunnerName:          req.RunnerName,
		Status:              StatusQueued,
		GitHubPayloadJSON:   string(payload),
		QueuedAt:            req.CreatedAt,
		UpdatedAt:           now,
		Version:             0,
	}
	if req.JobID != 0 {
		record.WorkflowJobID = &req.JobID
	}
	result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
	if result.Error != nil {
		return false, RunnerState{}, result.Error
	}
	if result.RowsAffected == 0 {
		st, err := s.ReadState(req.ID)
		return false, st, err
	}
	return true, recordToState(record), nil
}

func (s *DBStore) ReadRequest(id string) (RunnerRequest, error) {
	record, err := s.readRecord(id)
	if err != nil {
		return RunnerRequest{}, err
	}
	return recordToRequest(record)
}

func (s *DBStore) ReadState(id string) (RunnerState, error) {
	record, err := s.readRecord(id)
	if err != nil {
		return RunnerState{}, err
	}
	return recordToState(record), nil
}

func (s *DBStore) WriteState(st RunnerState) error {
	db, err := s.dbOrEnsure()
	if err != nil {
		return err
	}
	st.ID = sanitizeID(st.ID)
	if st.ID == "" {
		return fmt.Errorf("request id is empty")
	}
	now := time.Now().UTC()
	st.UpdatedAt = now
	applyStateTimestamps(&st, now)

	updates := map[string]any{
		"status":               st.Status,
		"repository_full_name": st.RepositoryFullName,
		"profile_name":         st.ProfileName,
		"runner_group":         st.RunnerGroup,
		"runner_name":          sanitizeRunnerName(st.RunnerName),
		"sandbox_id":           st.SandboxID,
		"process_pid":          st.ProcessPID,
		"assigned_job_id":      st.AssignedJobID,
		"assigned_job_name":    st.AssignedJobName,
		"error":                st.Error,
		"failure_stage":        st.FailureStage,
		"failure_reason":       st.FailureReason,
		"last_error_code":      st.LastErrorCode,
		"last_error_message":   st.LastErrorMessage,
		"last_error_retryable": st.LastErrorRetryable,
		"retry_count":          st.RetryCount,
		"updated_at":           st.UpdatedAt,
		"version":              st.Version + 1,
		"last_attempt_at":      zeroTimeToPointer(st.LastAttemptAt),
		"next_retry_at":        zeroTimeToPointer(st.NextRetryAt),
		"creating_at":          zeroTimeToPointer(st.CreatingAt),
		"running_at":           zeroTimeToPointer(st.RunningAt),
		"stopping_at":          zeroTimeToPointer(st.StoppingAt),
		"completed_at":         zeroTimeToPointer(st.CompletedAt),
		"failed_at":            zeroTimeToPointer(st.FailedAt),
		"lease_owner":          st.LeaseOwner,
		"lease_expires_at":     zeroTimeToPointer(st.LeaseExpiresAt),
	}
	result := db.Model(&runnerRequestRecord{}).
		Where("id = ? AND version = ?", st.ID, st.Version).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrConflict
	}
	return nil
}

func (s *DBStore) ListStates() ([]RunnerState, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return nil, err
	}
	var records []runnerRequestRecord
	if err := db.Order("queued_at DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	states := make([]RunnerState, 0, len(records))
	for _, record := range records {
		states = append(states, recordToState(record))
	}
	return states, nil
}

func (s *DBStore) ListActiveStates() ([]RunnerState, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return nil, err
	}
	var records []runnerRequestRecord
	if err := db.
		Where("status IN ?", activeStatuses()).
		Order("queued_at DESC").
		Find(&records).Error; err != nil {
		return nil, err
	}
	states := make([]RunnerState, 0, len(records))
	for _, record := range records {
		states = append(states, recordToState(record))
	}
	return states, nil
}

func (s *DBStore) ListMismatchedCompletedStates(limit int) ([]RunnerState, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	var records []runnerRequestRecord
	if err := db.
		Where("status = ?", StatusCompleted).
		Where("workflow_job_id IS NOT NULL AND workflow_job_id != 0").
		Where("assigned_job_id != 0 AND assigned_job_id != workflow_job_id").
		Order("updated_at DESC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, err
	}
	states := make([]RunnerState, 0, len(records))
	for _, record := range records {
		states = append(states, recordToState(record))
	}
	return states, nil
}

func (s *DBStore) ListFailedWorkflowJobStates(limit int) ([]RunnerState, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	var records []runnerRequestRecord
	if err := db.
		Where("status = ?", StatusFailed).
		Where("workflow_job_id IS NOT NULL AND workflow_job_id != 0").
		Where("failure_stage IN ?", []string{"recovery", "cleanup"}).
		Order("updated_at DESC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, err
	}
	states := make([]RunnerState, 0, len(records))
	for _, record := range records {
		states = append(states, recordToState(record))
	}
	return states, nil
}

func (s *DBStore) ListStatesPage(limit, offset int) ([]RunnerState, int64, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	var total int64
	if err := db.Model(&runnerRequestRecord{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []runnerRequestRecord
	if err := db.Order("queued_at DESC").Limit(limit).Offset(offset).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	states := make([]RunnerState, 0, len(records))
	for _, record := range records {
		states = append(states, recordToState(record))
	}
	return states, total, nil
}

func activeStatuses() []string {
	return []string{StatusQueued, StatusCreating, StatusRunning, StatusStopping}
}

func (s *DBStore) ActiveCount() (int, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return 0, err
	}
	var count int64
	if err := db.Model(&runnerRequestRecord{}).
		Where("status IN ?", activeStatuses()).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func (s *DBStore) InFlightCount() (int, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return 0, err
	}
	var count int64
	if err := db.Model(&runnerRequestRecord{}).
		Where("status IN ?", []string{StatusCreating, StatusRunning, StatusStopping}).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func (s *DBStore) ActiveCountForProfile(name string) (int, error) {
	return s.countStatesForProfile(name, []string{StatusQueued, StatusCreating, StatusRunning, StatusStopping})
}

func (s *DBStore) InFlightCountForProfile(name string) (int, error) {
	return s.countStatesForProfile(name, []string{StatusCreating, StatusRunning, StatusStopping})
}

func (s *DBStore) ClaimNextRunnable(workerID string, now time.Time, leaseTTL time.Duration) (RunnerRequest, RunnerState, bool, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return RunnerRequest{}, RunnerState{}, false, err
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return RunnerRequest{}, RunnerState{}, false, fmt.Errorf("worker id is required")
	}
	leaseExpiresAt := now.UTC().Add(leaseTTL)
	var claimed runnerRequestRecord
	err = db.Transaction(func(tx *gorm.DB) error {
		var candidate runnerRequestRecord
		if err := tx.
			Where("status = ?", StatusQueued).
			Where("(next_retry_at IS NULL OR next_retry_at <= ?)", now.UTC()).
			Where("(lease_expires_at IS NULL OR lease_expires_at <= ?)", now.UTC()).
			Order("queued_at ASC").
			First(&candidate).Error; err != nil {
			return err
		}
		result := tx.Model(&runnerRequestRecord{}).
			Where("id = ?", candidate.ID).
			Where("status = ?", StatusQueued).
			Where("(lease_expires_at IS NULL OR lease_expires_at <= ?)", now.UTC()).
			Updates(map[string]any{
				"lease_owner":      workerID,
				"lease_expires_at": leaseExpiresAt,
				"last_attempt_at":  now.UTC(),
				"updated_at":       now.UTC(),
				"version":          candidate.Version + 1,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrConflict
		}
		if err := tx.First(&claimed, "id = ?", candidate.ID).Error; err != nil {
			return err
		}
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return RunnerRequest{}, RunnerState{}, false, nil
	}
	if errors.Is(err, ErrConflict) {
		return RunnerRequest{}, RunnerState{}, false, nil
	}
	if err != nil {
		return RunnerRequest{}, RunnerState{}, false, err
	}
	req, err := recordToRequest(claimed)
	if err != nil {
		return RunnerRequest{}, RunnerState{}, false, err
	}
	return req, recordToState(claimed), true, nil
}

func (s *DBStore) ReleaseLease(id, workerID string) error {
	db, err := s.dbOrEnsure()
	if err != nil {
		return err
	}
	return db.Model(&runnerRequestRecord{}).
		Where("id = ? AND lease_owner = ?", sanitizeID(id), strings.TrimSpace(workerID)).
		Updates(map[string]any{
			"lease_owner":      "",
			"lease_expires_at": nil,
		}).Error
}

func (s *DBStore) RetryRequest(id string, now time.Time) (RunnerState, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return RunnerState{}, err
	}
	id = sanitizeID(id)
	var saved runnerRequestRecord
	if err := db.Transaction(func(tx *gorm.DB) error {
		var record runnerRequestRecord
		if err := tx.First(&record, "id = ?", id).Error; err != nil {
			return err
		}
		switch record.Status {
		case StatusFailed:
		case StatusQueued:
			if record.NextRetryAt == nil || !record.LastErrorRetryable {
				return ErrRetryNotAllowed
			}
		default:
			return ErrRetryNotAllowed
		}
		record.Status = StatusQueued
		record.Error = ""
		record.FailureStage = ""
		record.FailureReason = ""
		record.LastErrorCode = ""
		record.LastErrorMessage = ""
		record.LastErrorRetryable = false
		record.SandboxID = ""
		record.ProcessPID = 0
		record.AssignedJobID = 0
		record.AssignedJobName = ""
		record.NextRetryAt = nil
		record.LeaseOwner = ""
		record.LeaseExpiresAt = nil
		record.CreatingAt = nil
		record.RunningAt = nil
		record.StoppingAt = nil
		record.CompletedAt = nil
		record.FailedAt = nil
		record.UpdatedAt = now.UTC()
		record.Version++
		if err := tx.Model(&runnerRequestRecord{}).
			Where("id = ? AND version = ?", record.ID, record.Version-1).
			Updates(map[string]any{
				"status":               record.Status,
				"error":                record.Error,
				"failure_stage":        record.FailureStage,
				"failure_reason":       record.FailureReason,
				"last_error_code":      record.LastErrorCode,
				"last_error_message":   record.LastErrorMessage,
				"last_error_retryable": record.LastErrorRetryable,
				"sandbox_id":           record.SandboxID,
				"process_pid":          record.ProcessPID,
				"assigned_job_id":      record.AssignedJobID,
				"assigned_job_name":    record.AssignedJobName,
				"next_retry_at":        nil,
				"lease_owner":          record.LeaseOwner,
				"lease_expires_at":     nil,
				"creating_at":          nil,
				"running_at":           nil,
				"stopping_at":          nil,
				"completed_at":         nil,
				"failed_at":            nil,
				"updated_at":           record.UpdatedAt,
				"version":              record.Version,
			}).Error; err != nil {
			return err
		}
		if err := tx.First(&saved, "id = ?", id).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return RunnerState{}, err
	}
	return recordToState(saved), nil
}

func (s *DBStore) AppendLog(id, name string, data []byte) {
	if len(data) == 0 {
		return
	}
	db, err := s.dbOrEnsure()
	if err != nil {
		slog.Default().Warn("append runner log failed", "id", id, "name", name, "error", err)
		return
	}
	eventType, err := logEventType(name)
	if err != nil {
		slog.Default().Warn("append runner log failed", "id", id, "name", name, "error", err)
		return
	}
	if err := db.Create(&runnerEventRecord{
		RequestID: id,
		EventType: eventType,
		Message:   string(data),
		CreatedAt: time.Now().UTC(),
	}).Error; err != nil {
		slog.Default().Warn("append runner log failed", "id", id, "name", name, "error", err)
	}
}

func (s *DBStore) ReadLog(id, name string, maxBytes int64) ([]byte, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return nil, err
	}
	eventType, err := logEventType(name)
	if err != nil {
		return nil, err
	}
	var events []runnerEventRecord
	if err := db.Where("request_id = ? AND event_type = ?", sanitizeID(id), eventType).
		Order("id ASC").
		Find(&events).Error; err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, os.ErrNotExist
	}
	var buf bytes.Buffer
	for _, event := range events {
		buf.WriteString(event.Message)
	}
	data := buf.Bytes()
	if maxBytes > 0 && int64(len(data)) > maxBytes {
		data = data[len(data)-int(maxBytes):]
	}
	return append([]byte(nil), data...), nil
}

func (s *DBStore) ListProfiles() ([]RunnerProfile, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return nil, err
	}
	var records []runnerProfileRecord
	if err := db.Order("priority DESC, name ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	profiles := make([]RunnerProfile, 0, len(records))
	for _, record := range records {
		profile, err := recordToProfile(record)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func (s *DBStore) GetProfile(name string) (RunnerProfile, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return RunnerProfile{}, err
	}
	var record runnerProfileRecord
	if err := db.First(&record, "name = ?", strings.TrimSpace(name)).Error; err != nil {
		return RunnerProfile{}, err
	}
	return recordToProfile(record)
}

func (s *DBStore) UpsertProfile(profile RunnerProfile) (RunnerProfile, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return RunnerProfile{}, err
	}
	profile.Name = strings.TrimSpace(profile.Name)
	if profile.Name == "" {
		return RunnerProfile{}, fmt.Errorf("profile name is required")
	}
	labelsJSON, err := json.Marshal(profile.Labels)
	if err != nil {
		return RunnerProfile{}, err
	}
	now := time.Now().UTC()
	record := runnerProfileRecord{
		Name:             profile.Name,
		LabelsJSON:       string(labelsJSON),
		TemplateID:       profile.TemplateID,
		RunnerGroup:      profile.RunnerGroup,
		MaxConcurrency:   profile.MaxConcurrency,
		MinIdle:          profile.MinIdle,
		Priority:         profile.Priority,
		Enabled:          profile.Enabled,
		DefaultAvailable: profile.DefaultAvailable,
		CreatedAt:        profile.CreatedAt,
		UpdatedAt:        now,
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoUpdates: clause.Assignments(map[string]any{"labels_json": record.LabelsJSON, "template_id": record.TemplateID, "runner_group": record.RunnerGroup, "max_concurrency": record.MaxConcurrency, "min_idle": record.MinIdle, "priority": record.Priority, "enabled": record.Enabled, "default_available": record.DefaultAvailable, "updated_at": record.UpdatedAt}),
	}).Create(&record).Error; err != nil {
		return RunnerProfile{}, err
	}
	return s.GetProfile(record.Name)
}

func (s *DBStore) DeleteProfile(name string) error {
	db, err := s.dbOrEnsure()
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&runnerGroupSpecRecord{}, "spec_name = ?", name).Error; err != nil {
			return err
		}
		return tx.Delete(&runnerProfileRecord{}, "name = ?", name).Error
	})
}

func (s *DBStore) ListRunnerGroups() ([]RunnerGroup, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return nil, err
	}
	var records []runnerGroupRecord
	if err := db.Order("name ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	groups := make([]RunnerGroup, 0, len(records))
	for _, record := range records {
		group, err := s.recordToRunnerGroup(db, record)
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, nil
}

func (s *DBStore) GetRunnerGroup(name string) (RunnerGroup, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return RunnerGroup{}, err
	}
	var record runnerGroupRecord
	if err := db.First(&record, "name = ?", strings.TrimSpace(name)).Error; err != nil {
		return RunnerGroup{}, err
	}
	return s.recordToRunnerGroup(db, record)
}

func (s *DBStore) UpsertRunnerGroup(group RunnerGroup) (RunnerGroup, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return RunnerGroup{}, err
	}
	group.Name = strings.TrimSpace(group.Name)
	if group.Name == "" {
		return RunnerGroup{}, fmt.Errorf("runner group name is required")
	}
	specNames := uniqueTrimmed(group.SpecNames)
	now := time.Now().UTC()
	record := runnerGroupRecord{
		Name:        group.Name,
		Description: strings.TrimSpace(group.Description),
		Enabled:     group.Enabled,
		CreatedAt:   group.CreatedAt,
		UpdatedAt:   now,
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		for _, specName := range specNames {
			var count int64
			if err := tx.Model(&runnerProfileRecord{}).Where("name = ?", specName).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return fmt.Errorf("runner spec %q does not exist", specName)
			}
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "name"}},
			DoUpdates: clause.Assignments(map[string]any{
				"description": record.Description,
				"enabled":     record.Enabled,
				"updated_at":  record.UpdatedAt,
			}),
		}).Create(&record).Error; err != nil {
			return err
		}
		if err := tx.Delete(&runnerGroupSpecRecord{}, "group_name = ?", record.Name).Error; err != nil {
			return err
		}
		for _, specName := range specNames {
			link := runnerGroupSpecRecord{GroupName: record.Name, SpecName: specName, CreatedAt: now}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&link).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return RunnerGroup{}, err
	}
	return s.GetRunnerGroup(record.Name)
}

func (s *DBStore) DeleteRunnerGroup(name string) error {
	db, err := s.dbOrEnsure()
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&runnerGroupSpecRecord{}, "group_name = ?", name).Error; err != nil {
			return err
		}
		if err := tx.Model(&repositoryPolicyRecord{}).
			Where("runner_group_name = ?", name).
			Update("enabled", false).Error; err != nil {
			return err
		}
		return tx.Delete(&runnerGroupRecord{}, "name = ?", name).Error
	})
}

func (s *DBStore) ListRepositoryPolicies() ([]RepositoryPolicy, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return nil, err
	}
	var records []repositoryPolicyRecord
	if err := db.Order("repository_full_name ASC, profile_name ASC, runner_group_name ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	policies := make([]RepositoryPolicy, 0, len(records))
	for _, record := range records {
		policies = append(policies, recordToRepositoryPolicy(record))
	}
	return policies, nil
}

func (s *DBStore) GetRepositoryPolicy(id int64) (RepositoryPolicy, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return RepositoryPolicy{}, err
	}
	var record repositoryPolicyRecord
	if err := db.First(&record, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return RepositoryPolicy{}, ErrNotFound
		}
		return RepositoryPolicy{}, err
	}
	return recordToRepositoryPolicy(record), nil
}

func (s *DBStore) UpsertRepositoryPolicy(policy RepositoryPolicy) (RepositoryPolicy, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return RepositoryPolicy{}, err
	}
	policy.RepositoryFullName = strings.TrimSpace(policy.RepositoryFullName)
	policy.ProfileName = strings.TrimSpace(policy.ProfileName)
	policy.RunnerGroupName = strings.TrimSpace(policy.RunnerGroupName)
	if policy.RepositoryFullName == "" {
		return RepositoryPolicy{}, fmt.Errorf("repository_full_name is required")
	}
	if (policy.ProfileName == "") == (policy.RunnerGroupName == "") {
		return RepositoryPolicy{}, fmt.Errorf("exactly one of runner_spec_name or runner_group_name is required")
	}
	now := time.Now().UTC()
	if policy.ID == 0 {
		record := repositoryPolicyRecord{
			RepositoryFullName: policy.RepositoryFullName,
			ProfileName:        policy.ProfileName,
			RunnerGroupName:    policy.RunnerGroupName,
			Enabled:            policy.Enabled,
			CreatedAt:          now,
		}
		if record.ProfileName != "" {
			var saved repositoryPolicyRecord
			err := db.First(&saved, "repository_full_name = ? AND profile_name = ?", record.RepositoryFullName, record.ProfileName).Error
			if err == nil {
				saved.Enabled = record.Enabled
				saved.RunnerGroupName = ""
				if err := db.Save(&saved).Error; err != nil {
					return RepositoryPolicy{}, err
				}
				return recordToRepositoryPolicy(saved), nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return RepositoryPolicy{}, err
			}
			if err := db.Create(&record).Error; err != nil {
				return RepositoryPolicy{}, err
			}
			if err := db.First(&saved, "repository_full_name = ? AND profile_name = ?", record.RepositoryFullName, record.ProfileName).Error; err != nil {
				return RepositoryPolicy{}, err
			}
			return recordToRepositoryPolicy(saved), nil
		}
		var saved repositoryPolicyRecord
		err := db.First(&saved, "repository_full_name = ? AND runner_group_name = ?", record.RepositoryFullName, record.RunnerGroupName).Error
		if err == nil {
			saved.Enabled = record.Enabled
			saved.ProfileName = ""
			if err := db.Save(&saved).Error; err != nil {
				return RepositoryPolicy{}, err
			}
			return recordToRepositoryPolicy(saved), nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return RepositoryPolicy{}, err
		}
		if err := db.Create(&record).Error; err != nil {
			return RepositoryPolicy{}, err
		}
		if err := db.First(&saved, "repository_full_name = ? AND runner_group_name = ?", record.RepositoryFullName, record.RunnerGroupName).Error; err != nil {
			return RepositoryPolicy{}, err
		}
		return recordToRepositoryPolicy(saved), nil
	}
	updates := map[string]any{
		"repository_full_name": policy.RepositoryFullName,
		"profile_name":         policy.ProfileName,
		"runner_group_name":    policy.RunnerGroupName,
		"enabled":              policy.Enabled,
	}
	if err := db.Model(&repositoryPolicyRecord{}).Where("id = ?", policy.ID).Updates(updates).Error; err != nil {
		return RepositoryPolicy{}, err
	}
	var saved repositoryPolicyRecord
	if err := db.First(&saved, "id = ?", policy.ID).Error; err != nil {
		return RepositoryPolicy{}, err
	}
	return recordToRepositoryPolicy(saved), nil
}

func (s *DBStore) DeleteRepositoryPolicy(id int64) error {
	db, err := s.dbOrEnsure()
	if err != nil {
		return err
	}
	return db.Delete(&repositoryPolicyRecord{}, "id = ?", id).Error
}

func (s *DBStore) MatchProfile(repositoryFullName string, labels []string) (ProfileMatch, error) {
	policies, err := s.ListRepositoryPolicies()
	if err != nil {
		return ProfileMatch{}, err
	}
	profiles, err := s.ListProfiles()
	if err != nil {
		return ProfileMatch{}, err
	}
	match := ProfileMatch{
		RepositoryFullName: repositoryFullName,
		Labels:             append([]string(nil), labels...),
	}
	allowed := map[string]bool{}
	for _, profile := range profiles {
		if profile.Enabled && profile.DefaultAvailable {
			allowed[profile.Name] = true
		}
	}
	for _, policy := range policies {
		if !policy.Enabled {
			continue
		}
		if repositoryMatches(policy.RepositoryFullName, repositoryFullName) {
			if policy.ProfileName != "" {
				allowed[policy.ProfileName] = true
			}
			if policy.RunnerGroupName != "" {
				group, err := s.GetRunnerGroup(policy.RunnerGroupName)
				if err != nil {
					continue
				}
				if !group.Enabled {
					continue
				}
				for _, specName := range group.SpecNames {
					allowed[specName] = true
				}
			}
		}
	}
	if len(allowed) == 0 {
		match.Reason = "profile_not_allowed"
		return match, nil
	}
	var candidates []RunnerProfile
	for _, profile := range profiles {
		if !profile.Enabled || !allowed[profile.Name] {
			continue
		}
		if labelsMatch(labels, profile.Labels) {
			candidates = append(candidates, profile)
		}
	}
	if len(candidates) == 0 {
		match.Reason = "profile_labels_not_matched"
		return match, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority > candidates[j].Priority
		}
		if len(candidates[i].Labels) != len(candidates[j].Labels) {
			return len(candidates[i].Labels) > len(candidates[j].Labels)
		}
		return candidates[i].Name < candidates[j].Name
	})
	match.Profile = &candidates[0]
	return match, nil
}

func (s *DBStore) GetUserByOAuthIdentity(provider, subject string) (User, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return User{}, err
	}
	provider = normalizeOAuthProvider(provider)
	subject = normalizeOAuthSubject(subject)
	if provider == "" || subject == "" {
		return User{}, ErrNotFound
	}
	var record userRecord
	if err := db.First(&record, "oauth_provider = ? AND oauth_subject = ?", provider, subject).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	return recordToUser(record), nil
}

func (s *DBStore) UpsertUser(user User) (User, error) {
	return s.saveUser(user, true)
}

func (s *DBStore) EnsureUser(user User) (User, error) {
	return s.saveUser(user, false)
}

func (s *DBStore) saveUser(user User, updateExisting bool) (User, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return User{}, err
	}
	provider := normalizeOAuthProvider(user.OAuthProvider)
	subject := normalizeOAuthSubject(user.OAuthSubject)
	login := normalizeOAuthLogin(user.OAuthLogin)
	if provider == "" {
		return User{}, fmt.Errorf("oauth_provider is required")
	}
	if subject == "" {
		return User{}, fmt.Errorf("oauth_subject is required")
	}
	if login == "" {
		return User{}, fmt.Errorf("oauth_login is required")
	}
	role := normalizePlatformRole(user.Role)
	if role == "" {
		return User{}, fmt.Errorf("role must be admin or user")
	}
	now := time.Now().UTC()
	record := userRecord{
		OAuthProvider: provider,
		OAuthSubject:  subject,
		OAuthLogin:    login,
		Role:          role,
		CreatedAt:     user.CreatedAt,
		UpdatedAt:     now,
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	onConflict := clause.OnConflict{
		Columns: []clause.Column{{Name: "oauth_provider"}, {Name: "oauth_subject"}},
	}
	if updateExisting {
		onConflict.DoUpdates = clause.Assignments(map[string]any{
			"oauth_login": record.OAuthLogin,
			"role":        record.Role,
			"updated_at":  record.UpdatedAt,
		})
	} else {
		onConflict.DoUpdates = clause.Assignments(map[string]any{
			"oauth_login": record.OAuthLogin,
			"updated_at":  record.UpdatedAt,
		})
	}
	if err := db.Clauses(onConflict).Create(&record).Error; err != nil {
		return User{}, err
	}
	return s.GetUserByOAuthIdentity(provider, subject)
}

func (s *DBStore) AppendAuditEvent(event AuditEvent) (AuditEvent, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return AuditEvent{}, err
	}
	record := auditEventRecord{
		Actor:        strings.TrimSpace(event.Actor),
		Action:       strings.TrimSpace(event.Action),
		ResourceType: strings.TrimSpace(event.ResourceType),
		ResourceID:   strings.TrimSpace(event.ResourceID),
		PayloadJSON:  event.PayloadJSON,
		CreatedAt:    event.CreatedAt.UTC(),
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if err := db.Create(&record).Error; err != nil {
		return AuditEvent{}, err
	}
	return AuditEvent(record), nil
}

func (s *DBStore) ListAuditEvents(limit int) ([]AuditEvent, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	var records []auditEventRecord
	if err := db.Order("created_at DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	events := make([]AuditEvent, 0, len(records))
	for _, record := range records {
		events = append(events, AuditEvent(record))
	}
	return events, nil
}

func (s *DBStore) dbOrEnsure() (*gorm.DB, error) {
	if err := s.Ensure(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db, nil
}

func (s *DBStore) readRecord(id string) (runnerRequestRecord, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return runnerRequestRecord{}, err
	}
	var record runnerRequestRecord
	if err := db.First(&record, "id = ?", sanitizeID(id)).Error; err != nil {
		return runnerRequestRecord{}, err
	}
	return record, nil
}

func (s *DBStore) countStatesForProfile(name string, statuses []string) (int, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return 0, err
	}
	var count int64
	if err := db.Model(&runnerRequestRecord{}).
		Where("profile_name = ?", strings.TrimSpace(name)).
		Where("status IN ?", statuses).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func (s *DBStore) open() (*gorm.DB, error) {
	cfg := &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)}
	switch s.opts.Backend {
	case BackendSQLite:
		dir := filepath.Dir(s.opts.DatabaseURL)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, err
			}
		}
		db, err := gorm.Open(sqlite.Open(s.opts.DatabaseURL), cfg)
		if err != nil {
			return nil, err
		}
		if err := configureSQLite(db); err != nil {
			return nil, err
		}
		return db, nil
	case BackendPostgres:
		return gorm.Open(postgres.Open(s.opts.DatabaseURL), cfg)
	default:
		return nil, fmt.Errorf("unsupported state backend: %s", s.opts.Backend)
	}
}

func configureSQLite(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 15000",
	}
	for _, pragma := range pragmas {
		if err := db.Exec(pragma).Error; err != nil {
			return fmt.Errorf("%s: %w", pragma, err)
		}
	}
	return nil
}

func (s *DBStore) migrate(db *gorm.DB) error {
	statements := sqliteMigrations
	if s.opts.Backend == BackendPostgres {
		statements = postgresMigrations
	}
	for _, stmt := range statements {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	for column, columnType := range map[string]string{
		"repository_full_name":  "TEXT",
		"profile_name":          "TEXT",
		"runner_group":          "TEXT",
		"requested_labels_json": "TEXT",
		"last_error_code":       "TEXT",
		"last_error_message":    "TEXT",
		"last_error_retryable":  "BOOLEAN NOT NULL DEFAULT FALSE",
		"retry_count":           "INTEGER NOT NULL DEFAULT 0",
		"last_attempt_at":       "TIMESTAMP",
		"next_retry_at":         "TIMESTAMP",
		"lease_owner":           "TEXT",
		"lease_expires_at":      "TIMESTAMP",
	} {
		if !db.Migrator().HasColumn(&runnerRequestRecord{}, column) {
			if err := db.Exec(fmt.Sprintf("ALTER TABLE runner_requests ADD COLUMN %s %s", column, columnType)).Error; err != nil {
				return err
			}
		}
	}
	for column, columnType := range map[string]string{
		"default_available": "BOOLEAN NOT NULL DEFAULT TRUE",
	} {
		if !db.Migrator().HasColumn(&runnerProfileRecord{}, column) {
			if err := db.Exec(fmt.Sprintf("ALTER TABLE runner_profiles ADD COLUMN %s %s", column, columnType)).Error; err != nil {
				return err
			}
		}
	}
	for column, columnType := range map[string]string{
		"runner_group_name": "TEXT NOT NULL DEFAULT ''",
	} {
		if !db.Migrator().HasColumn(&repositoryPolicyRecord{}, column) {
			if err := db.Exec(fmt.Sprintf("ALTER TABLE repository_policies ADD COLUMN %s %s", column, columnType)).Error; err != nil {
				return err
			}
		}
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_repository_policies_repository_group ON repository_policies(repository_full_name, runner_group_name) WHERE runner_group_name <> '';`).Error; err != nil {
		return err
	}
	if db.Migrator().HasTable(&userRecord{}) && !db.Migrator().HasColumn(&userRecord{}, "oauth_provider") {
		if err := db.Exec(`ALTER TABLE users ADD COLUMN oauth_provider TEXT NOT NULL DEFAULT 'github'`).Error; err != nil {
			return err
		}
	}
	if db.Migrator().HasTable(&userRecord{}) && !db.Migrator().HasColumn(&userRecord{}, "oauth_subject") {
		if err := db.Exec(`ALTER TABLE users ADD COLUMN oauth_subject TEXT NOT NULL DEFAULT ''`).Error; err != nil {
			return err
		}
		if err := db.Exec(`UPDATE users SET oauth_subject = oauth_login WHERE oauth_subject = ''`).Error; err != nil {
			return err
		}
	}
	if err := db.Exec(`DROP INDEX IF EXISTS idx_users_oauth_login;`).Error; err != nil {
		return err
	}
	if err := db.Exec(`DROP INDEX IF EXISTS idx_users_oauth_identity;`).Error; err != nil {
		return err
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_oauth_identity ON users(oauth_provider, oauth_subject);`).Error; err != nil {
		return err
	}
	return nil
}

func applyStateTimestamps(st *RunnerState, now time.Time) {
	switch st.Status {
	case StatusCreating:
		if st.CreatingAt.IsZero() {
			st.CreatingAt = now
		}
	case StatusRunning:
		if st.CreatingAt.IsZero() {
			st.CreatingAt = now
		}
		if st.RunningAt.IsZero() {
			st.RunningAt = now
		}
	case StatusStopping:
		if st.StoppingAt.IsZero() {
			st.StoppingAt = now
		}
	case StatusCompleted:
		if st.StoppingAt.IsZero() {
			st.StoppingAt = now
		}
		if st.CompletedAt.IsZero() {
			st.CompletedAt = now
		}
	case StatusFailed:
		if st.FailedAt.IsZero() {
			st.FailedAt = now
		}
	}
}

func recordToRequest(record runnerRequestRecord) (RunnerRequest, error) {
	var labels []string
	if err := json.Unmarshal([]byte(record.LabelsJSON), &labels); err != nil {
		return RunnerRequest{}, err
	}
	var requestedLabels []string
	if record.RequestedLabelsJSON != "" {
		if err := json.Unmarshal([]byte(record.RequestedLabelsJSON), &requestedLabels); err != nil {
			return RunnerRequest{}, err
		}
	}
	return RunnerRequest{
		ID:                 record.ID,
		Source:             record.Source,
		JobID:              pointerToInt64(record.WorkflowJobID),
		RepositoryFullName: record.RepositoryFullName,
		RequestedLabels:    requestedLabels,
		Labels:             labels,
		ProfileName:        record.ProfileName,
		RunnerGroup:        record.RunnerGroup,
		RunnerName:         record.RunnerName,
		CreatedAt:          record.QueuedAt,
	}, nil
}

func recordToState(record runnerRequestRecord) RunnerState {
	requestedLabels, _ := labelsFromJSON(record.RequestedLabelsJSON)
	githubLinks := githubLinksFromPayload(record)
	return RunnerState{
		ID:                 record.ID,
		Status:             record.Status,
		RepositoryFullName: record.RepositoryFullName,
		RequestedLabels:    requestedLabels,
		ProfileName:        record.ProfileName,
		RunnerGroup:        record.RunnerGroup,
		RunnerName:         record.RunnerName,
		SandboxID:          record.SandboxID,
		ProcessPID:         record.ProcessPID,
		WorkflowJobID:      pointerToInt64(record.WorkflowJobID),
		WorkflowRunID:      githubLinks.workflowRunID,
		GitHubJobURL:       githubLinks.jobURL,
		PullRequestNumber:  githubLinks.pullRequestNumber,
		AssignedJobID:      record.AssignedJobID,
		AssignedJobName:    record.AssignedJobName,
		Error:              record.Error,
		FailureStage:       record.FailureStage,
		FailureReason:      record.FailureReason,
		LastErrorCode:      record.LastErrorCode,
		LastErrorMessage:   record.LastErrorMessage,
		LastErrorRetryable: record.LastErrorRetryable,
		RetryCount:         record.RetryCount,
		UpdatedAt:          record.UpdatedAt,
		CreatedAt:          record.QueuedAt,
		LastAttemptAt:      pointerToTime(record.LastAttemptAt),
		NextRetryAt:        pointerToTime(record.NextRetryAt),
		CreatingAt:         pointerToTime(record.CreatingAt),
		RunningAt:          pointerToTime(record.RunningAt),
		StoppingAt:         pointerToTime(record.StoppingAt),
		CompletedAt:        pointerToTime(record.CompletedAt),
		FailedAt:           pointerToTime(record.FailedAt),
		LeaseOwner:         record.LeaseOwner,
		LeaseExpiresAt:     pointerToTime(record.LeaseExpiresAt),
		Version:            record.Version,
	}
}

func recordToUser(record userRecord) User {
	return User(record)
}

type githubPayloadLinks struct {
	workflowRunID     int64
	jobURL            string
	pullRequestNumber int64
}

type githubPayloadPullRequest struct {
	Number int64 `json:"number"`
}

func githubLinksFromPayload(record runnerRequestRecord) githubPayloadLinks {
	var payload struct {
		WorkflowJob struct {
			RunID        int64                      `json:"run_id"`
			HTMLURL      string                     `json:"html_url"`
			PullRequests []githubPayloadPullRequest `json:"pull_requests"`
			WorkflowRun  struct {
				ID int64 `json:"id"`
			} `json:"workflow_run"`
		} `json:"workflow_job"`
		WorkflowRun struct {
			ID           int64                      `json:"id"`
			HTMLURL      string                     `json:"html_url"`
			PullRequests []githubPayloadPullRequest `json:"pull_requests"`
		} `json:"workflow_run"`
		PullRequest struct {
			Number int64 `json:"number"`
		} `json:"pull_request"`
	}
	if record.GitHubPayloadJSON != "" {
		_ = json.Unmarshal([]byte(record.GitHubPayloadJSON), &payload)
	}

	runID := payload.WorkflowJob.RunID
	if runID == 0 {
		runID = payload.WorkflowJob.WorkflowRun.ID
	}
	if runID == 0 {
		runID = payload.WorkflowRun.ID
	}

	prNumber := firstPullRequestNumber(payload.WorkflowJob.PullRequests)
	if prNumber == 0 {
		prNumber = firstPullRequestNumber(payload.WorkflowRun.PullRequests)
	}
	if prNumber == 0 {
		prNumber = payload.PullRequest.Number
	}

	jobURL := payload.WorkflowJob.HTMLURL
	jobID := record.AssignedJobID
	if jobID == 0 {
		jobID = pointerToInt64(record.WorkflowJobID)
	}
	if jobURL == "" && record.RepositoryFullName != "" && runID > 0 && jobID > 0 {
		jobURL = fmt.Sprintf("https://github.com/%s/actions/runs/%d/job/%d", record.RepositoryFullName, runID, jobID)
	}
	jobURL = appendPullRequestQuery(jobURL, prNumber)

	return githubPayloadLinks{
		workflowRunID:     runID,
		jobURL:            jobURL,
		pullRequestNumber: prNumber,
	}
}

func firstPullRequestNumber(pulls []githubPayloadPullRequest) int64 {
	if len(pulls) == 0 {
		return 0
	}
	return pulls[0].Number
}

func appendPullRequestQuery(rawURL string, prNumber int64) string {
	if rawURL == "" || prNumber == 0 || strings.Contains(rawURL, "pr=") {
		return rawURL
	}
	separator := "?"
	if strings.Contains(rawURL, "?") {
		separator = "&"
	}
	return fmt.Sprintf("%s%spr=%d", rawURL, separator, prNumber)
}

func recordToProfile(record runnerProfileRecord) (RunnerProfile, error) {
	var labels []string
	if err := json.Unmarshal([]byte(record.LabelsJSON), &labels); err != nil {
		return RunnerProfile{}, err
	}
	return RunnerProfile{
		Name:             record.Name,
		Labels:           labels,
		TemplateID:       record.TemplateID,
		RunnerGroup:      record.RunnerGroup,
		MaxConcurrency:   record.MaxConcurrency,
		MinIdle:          record.MinIdle,
		Priority:         record.Priority,
		Enabled:          record.Enabled,
		DefaultAvailable: record.DefaultAvailable,
		CreatedAt:        record.CreatedAt,
		UpdatedAt:        record.UpdatedAt,
	}, nil
}

func (s *DBStore) recordToRunnerGroup(db *gorm.DB, record runnerGroupRecord) (RunnerGroup, error) {
	var links []runnerGroupSpecRecord
	if err := db.Where("group_name = ?", record.Name).Order("spec_name ASC").Find(&links).Error; err != nil {
		return RunnerGroup{}, err
	}
	specNames := make([]string, 0, len(links))
	for _, link := range links {
		specNames = append(specNames, link.SpecName)
	}
	return RunnerGroup{
		Name:        record.Name,
		Description: record.Description,
		SpecNames:   specNames,
		Enabled:     record.Enabled,
		CreatedAt:   record.CreatedAt,
		UpdatedAt:   record.UpdatedAt,
	}, nil
}

func recordToRepositoryPolicy(record repositoryPolicyRecord) RepositoryPolicy {
	return RepositoryPolicy(record)
}

func uniqueTrimmed(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func zeroTimeToPointer(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	tt := t.UTC()
	return &tt
}

func pointerToTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.UTC()
}

func pointerToInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func labelsFromJSON(data string) ([]string, error) {
	if strings.TrimSpace(data) == "" {
		return nil, nil
	}
	var labels []string
	if err := json.Unmarshal([]byte(data), &labels); err != nil {
		return nil, err
	}
	return labels, nil
}

func repositoryMatches(pattern, repository string) bool {
	pattern = strings.TrimSpace(pattern)
	repository = strings.TrimSpace(repository)
	if pattern == "" || repository == "" {
		return false
	}
	if pattern == repository {
		return true
	}
	if strings.HasSuffix(pattern, "/*") {
		return strings.HasPrefix(repository, strings.TrimSuffix(pattern, "*"))
	}
	return false
}

func labelsMatch(jobLabels, required []string) bool {
	return labelutil.Match(jobLabels, required)
}

func logEventType(name string) (string, error) {
	switch filepath.Base(name) {
	case "control.log":
		return "control_log", nil
	case "stdout.log":
		return "stdout_log", nil
	case "stderr.log":
		return "stderr_log", nil
	default:
		return "", fmt.Errorf("unsupported log name")
	}
}

func sanitizeID(id string) string {
	id = strings.TrimSpace(id)
	id = strings.ReplaceAll(id, "/", "-")
	id = strings.ReplaceAll(id, "\\", "-")
	id = strings.ReplaceAll(id, "..", "-")
	return id
}

func sanitizeRunnerName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	return name
}

func normalizeOAuthProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

func normalizeOAuthSubject(subject string) string {
	return strings.TrimSpace(subject)
}

func normalizeOAuthLogin(login string) string {
	return strings.ToLower(strings.TrimSpace(login))
}

func normalizePlatformRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	switch role {
	case "admin", "user":
		return role
	default:
		return ""
	}
}

var sqliteMigrations = []string{
	`CREATE TABLE IF NOT EXISTS runner_requests (
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
	);`,
	`CREATE TABLE IF NOT EXISTS runner_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		request_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		stage TEXT,
		message TEXT,
		payload_json TEXT,
		created_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS runner_profiles (
		name TEXT PRIMARY KEY,
		labels_json TEXT NOT NULL,
		template_id TEXT NOT NULL,
		runner_group TEXT,
		max_concurrency INTEGER NOT NULL,
		min_idle INTEGER NOT NULL DEFAULT 0,
		priority INTEGER NOT NULL DEFAULT 0,
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		default_available BOOLEAN NOT NULL DEFAULT TRUE,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS runner_groups (
		name TEXT PRIMARY KEY,
		description TEXT,
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS runner_group_specs (
		group_name TEXT NOT NULL,
		spec_name TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		PRIMARY KEY (group_name, spec_name)
	);`,
	`CREATE TABLE IF NOT EXISTS repository_policies (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		repository_full_name TEXT NOT NULL,
		profile_name TEXT NOT NULL,
		runner_group_name TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		created_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS audit_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		actor TEXT NOT NULL,
		action TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		payload_json TEXT,
		created_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		oauth_provider TEXT NOT NULL,
		oauth_subject TEXT NOT NULL,
		oauth_login TEXT NOT NULL,
		role TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_runner_requests_workflow_job_id ON runner_requests(workflow_job_id);`,
	`CREATE INDEX IF NOT EXISTS idx_runner_requests_status_updated ON runner_requests(status, updated_at, queued_at);`,
	`CREATE INDEX IF NOT EXISTS idx_runner_requests_status_retry_queue ON runner_requests(status, next_retry_at, queued_at);`,
	`CREATE INDEX IF NOT EXISTS idx_runner_requests_lease_expiry ON runner_requests(status, lease_expires_at);`,
	`CREATE INDEX IF NOT EXISTS idx_runner_events_request_created ON runner_events(request_id, created_at);`,
	`CREATE INDEX IF NOT EXISTS idx_runner_group_specs_spec ON runner_group_specs(spec_name);`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_repository_policies_repository_profile ON repository_policies(repository_full_name, profile_name) WHERE profile_name <> '';`,
	`CREATE INDEX IF NOT EXISTS idx_audit_events_created ON audit_events(created_at);`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_oauth_identity ON users(oauth_provider, oauth_subject);`,
}

var postgresMigrations = []string{
	`CREATE TABLE IF NOT EXISTS runner_requests (
		id TEXT PRIMARY KEY,
		source TEXT NOT NULL,
		workflow_job_id BIGINT,
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
		process_pid BIGINT,
		assigned_job_id BIGINT,
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
		version BIGINT NOT NULL DEFAULT 0
	);`,
	`CREATE TABLE IF NOT EXISTS runner_events (
		id BIGSERIAL PRIMARY KEY,
		request_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		stage TEXT,
		message TEXT,
		payload_json TEXT,
		created_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS runner_profiles (
		name TEXT PRIMARY KEY,
		labels_json TEXT NOT NULL,
		template_id TEXT NOT NULL,
		runner_group TEXT,
		max_concurrency INTEGER NOT NULL,
		min_idle INTEGER NOT NULL DEFAULT 0,
		priority INTEGER NOT NULL DEFAULT 0,
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		default_available BOOLEAN NOT NULL DEFAULT TRUE,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS runner_groups (
		name TEXT PRIMARY KEY,
		description TEXT,
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS runner_group_specs (
		group_name TEXT NOT NULL,
		spec_name TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		PRIMARY KEY (group_name, spec_name)
	);`,
	`CREATE TABLE IF NOT EXISTS repository_policies (
		id BIGSERIAL PRIMARY KEY,
		repository_full_name TEXT NOT NULL,
		profile_name TEXT NOT NULL,
		runner_group_name TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		created_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS audit_events (
		id BIGSERIAL PRIMARY KEY,
		actor TEXT NOT NULL,
		action TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		payload_json TEXT,
		created_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS users (
		id BIGSERIAL PRIMARY KEY,
		oauth_provider TEXT NOT NULL,
		oauth_subject TEXT NOT NULL,
		oauth_login TEXT NOT NULL,
		role TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_runner_requests_workflow_job_id ON runner_requests(workflow_job_id);`,
	`CREATE INDEX IF NOT EXISTS idx_runner_requests_status_updated ON runner_requests(status, updated_at, queued_at);`,
	`CREATE INDEX IF NOT EXISTS idx_runner_requests_status_retry_queue ON runner_requests(status, next_retry_at, queued_at);`,
	`CREATE INDEX IF NOT EXISTS idx_runner_requests_lease_expiry ON runner_requests(status, lease_expires_at);`,
	`CREATE INDEX IF NOT EXISTS idx_runner_events_request_created ON runner_events(request_id, created_at);`,
	`CREATE INDEX IF NOT EXISTS idx_runner_group_specs_spec ON runner_group_specs(spec_name);`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_repository_policies_repository_profile ON repository_policies(repository_full_name, profile_name) WHERE profile_name <> '';`,
	`CREATE INDEX IF NOT EXISTS idx_audit_events_created ON audit_events(created_at);`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_oauth_identity ON users(oauth_provider, oauth_subject);`,
}
