package state

import "time"

type runnerRequestRecord struct {
	ID                   string     `gorm:"column:id;primaryKey"`
	Source               string     `gorm:"column:source;not null"`
	WorkflowJobID        *int64     `gorm:"column:workflow_job_id;uniqueIndex:idx_runner_requests_workflow_job_id"`
	GitHubInstallationID int64      `gorm:"column:github_installation_id;index:idx_runner_requests_github_installation_updated"`
	RepositoryFullName   string     `gorm:"column:repository_full_name"`
	RequestedLabelsJSON  string     `gorm:"column:requested_labels_json"`
	LabelsJSON           string     `gorm:"column:labels_json;not null"`
	ProfileName          string     `gorm:"column:profile_name"`
	RunnerGroup          string     `gorm:"column:runner_group"`
	RunnerName           string     `gorm:"column:runner_name;not null"`
	Status               string     `gorm:"column:status;not null;index:idx_runner_requests_status_updated;index:idx_runner_requests_status_retry_queue;index:idx_runner_requests_lease_expiry"`
	FailureStage         string     `gorm:"column:failure_stage"`
	FailureReason        string     `gorm:"column:failure_reason"`
	LastErrorCode        string     `gorm:"column:last_error_code"`
	LastErrorMessage     string     `gorm:"column:last_error_message"`
	LastErrorRetryable   bool       `gorm:"column:last_error_retryable;not null;default:false"`
	RetryCount           int        `gorm:"column:retry_count;not null;default:0"`
	SandboxID            string     `gorm:"column:sandbox_id"`
	ProcessPID           uint32     `gorm:"column:process_pid"`
	AssignedJobID        int64      `gorm:"column:assigned_job_id"`
	AssignedJobName      string     `gorm:"column:assigned_job_name"`
	Error                string     `gorm:"column:error"`
	GitHubPayloadJSON    string     `gorm:"column:github_payload_json;type:text"`
	QueuedAt             time.Time  `gorm:"column:queued_at;not null;index:idx_runner_requests_status_updated;index:idx_runner_requests_status_retry_queue"`
	LastAttemptAt        *time.Time `gorm:"column:last_attempt_at"`
	NextRetryAt          *time.Time `gorm:"column:next_retry_at;index:idx_runner_requests_status_retry_queue"`
	CreatingAt           *time.Time `gorm:"column:creating_at"`
	RunningAt            *time.Time `gorm:"column:running_at"`
	StoppingAt           *time.Time `gorm:"column:stopping_at"`
	CompletedAt          *time.Time `gorm:"column:completed_at"`
	FailedAt             *time.Time `gorm:"column:failed_at"`
	LeaseOwner           string     `gorm:"column:lease_owner"`
	LeaseExpiresAt       *time.Time `gorm:"column:lease_expires_at;index:idx_runner_requests_lease_expiry"`
	UpdatedAt            time.Time  `gorm:"column:updated_at;not null;index:idx_runner_requests_status_updated;index:idx_runner_requests_github_installation_updated"`
	Version              int64      `gorm:"column:version;not null;default:0"`
}

func (runnerRequestRecord) TableName() string { return "runner_requests" }

type runnerEventRecord struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement"`
	RequestID   string    `gorm:"column:request_id;not null;index:idx_runner_events_request_created"`
	EventType   string    `gorm:"column:event_type;not null"`
	Stage       string    `gorm:"column:stage"`
	Message     string    `gorm:"column:message;type:text"`
	PayloadJSON string    `gorm:"column:payload_json;type:text"`
	CreatedAt   time.Time `gorm:"column:created_at;not null;index:idx_runner_events_request_created"`
}

func (runnerEventRecord) TableName() string { return "runner_events" }

type runnerProfileRecord struct {
	Name             string    `gorm:"column:name;primaryKey"`
	LabelsJSON       string    `gorm:"column:labels_json;not null"`
	TemplateID       string    `gorm:"column:template_id;not null"`
	RunnerGroup      string    `gorm:"column:runner_group"`
	MaxConcurrency   int       `gorm:"column:max_concurrency;not null"`
	MinIdle          int       `gorm:"column:min_idle;not null;default:0"`
	Priority         int       `gorm:"column:priority;not null;default:0"`
	Enabled          bool      `gorm:"column:enabled;not null"`
	DefaultAvailable bool      `gorm:"column:default_available;not null"`
	CreatedAt        time.Time `gorm:"column:created_at;not null"`
	UpdatedAt        time.Time `gorm:"column:updated_at;not null"`
}

func (runnerProfileRecord) TableName() string { return "runner_profiles" }

type legacyRunnerProfileDefaultAvailableColumn struct {
	DefaultAvailable bool `gorm:"column:default_available;not null;default:true"`
}

func (legacyRunnerProfileDefaultAvailableColumn) TableName() string { return "runner_profiles" }

type runnerGroupRecord struct {
	Name        string    `gorm:"column:name;primaryKey"`
	Description string    `gorm:"column:description"`
	Enabled     bool      `gorm:"column:enabled;not null"`
	CreatedAt   time.Time `gorm:"column:created_at;not null"`
	UpdatedAt   time.Time `gorm:"column:updated_at;not null"`
}

func (runnerGroupRecord) TableName() string { return "runner_groups" }

type runnerGroupSpecRecord struct {
	GroupName string    `gorm:"column:group_name;primaryKey"`
	SpecName  string    `gorm:"column:spec_name;primaryKey;index:idx_runner_group_specs_spec"`
	CreatedAt time.Time `gorm:"column:created_at;not null"`
}

func (runnerGroupSpecRecord) TableName() string { return "runner_group_specs" }

type repositoryPolicyRecord struct {
	ID                 int64     `gorm:"column:id;primaryKey;autoIncrement"`
	RepositoryFullName string    `gorm:"column:repository_full_name;not null;uniqueIndex:idx_repository_policies_unique"`
	ProfileName        string    `gorm:"column:profile_name;not null;uniqueIndex:idx_repository_policies_unique"`
	RunnerGroupName    string    `gorm:"column:runner_group_name;not null;default:'';uniqueIndex:idx_repository_policies_unique"`
	Enabled            bool      `gorm:"column:enabled;not null"`
	CreatedAt          time.Time `gorm:"column:created_at;not null"`
}

func (repositoryPolicyRecord) TableName() string { return "repository_policies" }

type legacyRepositoryPolicyRunnerGroupNameColumn struct {
	RunnerGroupName string `gorm:"column:runner_group_name;not null;default:''"`
}

func (legacyRepositoryPolicyRunnerGroupNameColumn) TableName() string { return "repository_policies" }

type auditEventRecord struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement"`
	Actor        string    `gorm:"column:actor;not null"`
	Action       string    `gorm:"column:action;not null"`
	ResourceType string    `gorm:"column:resource_type;not null"`
	ResourceID   string    `gorm:"column:resource_id;not null"`
	PayloadJSON  string    `gorm:"column:payload_json;type:text"`
	CreatedAt    time.Time `gorm:"column:created_at;not null;index:idx_audit_events_created"`
}

func (auditEventRecord) TableName() string { return "audit_events" }

type accountRecord struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement"`
	Role      string    `gorm:"column:role;not null"`
	CreatedAt time.Time `gorm:"column:created_at;not null"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null"`
}

func (accountRecord) TableName() string { return "accounts" }

type oauthIdentityRecord struct {
	ID            int64         `gorm:"column:id;primaryKey;autoIncrement"`
	AccountID     int64         `gorm:"column:account_id;not null;index:idx_oauth_identities_account"`
	Account       accountRecord `gorm:"foreignKey:AccountID"`
	OAuthProvider string        `gorm:"column:oauth_provider;not null;uniqueIndex:idx_oauth_identities_provider_subject"`
	OAuthSubject  string        `gorm:"column:oauth_subject;not null;uniqueIndex:idx_oauth_identities_provider_subject"`
	OAuthLogin    string        `gorm:"column:oauth_login;not null"`
	CreatedAt     time.Time     `gorm:"column:created_at;not null"`
	UpdatedAt     time.Time     `gorm:"column:updated_at;not null"`
}

func (oauthIdentityRecord) TableName() string { return "oauth_identities" }

type githubInstallationRecord struct {
	ID             int64         `gorm:"column:id;primaryKey;autoIncrement"`
	AccountID      int64         `gorm:"column:account_id;not null;uniqueIndex:idx_github_installations_account_installation;index:idx_github_installations_account"`
	Account        accountRecord `gorm:"foreignKey:AccountID"`
	InstallationID int64         `gorm:"column:installation_id;not null;uniqueIndex:idx_github_installations_account_installation"`
	AccountLogin   string        `gorm:"column:account_login;not null"`
	AccountName    string        `gorm:"column:account_name"`
	AccountAvatar  string        `gorm:"column:account_avatar"`
	CreatedAt      time.Time     `gorm:"column:created_at;not null"`
	UpdatedAt      time.Time     `gorm:"column:updated_at;not null"`
}

func (githubInstallationRecord) TableName() string { return "github_installations" }
