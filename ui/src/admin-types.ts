export type RunnerStatus = "queued" | "creating" | "running" | "stopping" | "completed" | "failed"

export type RunnerState = {
  id: string
  status: RunnerStatus
  repository_full_name?: string
  requested_labels?: string[]
  runner_spec_name?: string
  runner_group?: string
  runner_name: string
  sandbox_id?: string
  sandbox_config_source?: string
  process_pid?: number
  workflow_job_id?: number
  workflow_run_id?: number
  workflow_name?: string
  workflow_run_attempt?: number
  head_branch?: string
  head_sha?: string
  github_job_url?: string
  pull_request_number?: number
  assigned_job_id?: number
  assigned_job_name?: string
  error?: string
  failure_stage?: string
  failure_reason?: string
  last_error_code?: string
  last_error_message?: string
  last_error_retryable?: boolean
  retry_count?: number
  updated_at: string
  created_at: string
  running_at?: string
  next_retry_at?: string
  completed_at?: string
  failed_at?: string
}

export type RunnerJobGroup = {
  key: string
  group: "pull_request" | "branch" | "workflow_run" | "manual" | "repository"
  repository: string
  title: string
  subtitle: string
  updated_at: string
  jobs: RunnerState[]
  current_jobs: RunnerState[]
  previous_jobs: RunnerState[]
  workflow_run_ids: number[]
  head_sha?: string
  head_branch?: string
  pull_request_number?: number
  pull_request_title?: string
  pull_request_title_error?: string
}

export type RunnerSpec = {
  name: string
  labels: string[]
  required_labels: string[]
  template_id: string
  default_template_name?: string
  managed_by?: string
  catalog_revision?: number
  runner_group?: string
  max_concurrency: number
  min_idle: number
  priority: number
  enabled: boolean
  default_available: boolean
  created_at: string
  updated_at: string
}

export type RunnerSpecMatch = {
  repository_full_name: string
  labels: string[]
  runner_spec?: RunnerSpec
  reason?: string
}

export type DiagnosticsSummary = {
  pprof: Array<{ address: string; address_file: string; dump_script: string }>
  state: { backend: string; database: string }
  github: { auth_mode: string; installation_id?: number; api_base_url: string }
  recent_failures: RunnerState[]
}

export type CatalogMigrationGateCode =
  | "window_at_least_72_hours"
  | "catalog_unchanged"
  | "matcher_parity"
  | "all_enabled_specs_full_lifecycle"

export type CatalogMigrationManualRequirement =
  | "backup_restore_verified"
  | "continuous_service_observation"
  | "workflow_labels_unchanged"

export type CatalogMatchReplaySample = {
  repository_full_name: string
  labels?: string[]
  request_count: number
  first_seen_at: string
  last_seen_at: string
  result: "legacy_only" | "enabled_only" | "different_profile" | "error"
  legacy_profile?: string
  legacy_reason?: string
  enabled_profile?: string
  enabled_reason?: string
  error?: string
}

export type RunnerSpecLifecycleExample = {
  request_id: string
  repository_full_name: string
  workflow_job_id: number
  github_job_url?: string
  requested_labels: string[]
  registered_at: string
  completed_at: string
  cleanup_finalized_at: string
}

export type RunnerSpecLifecycleAttempt = {
  request_id: string
  repository_full_name: string
  status: RunnerStatus
  workflow_job_id?: number
  github_job_url?: string
  requested_labels: string[]
  failure_stage?: string
  failure_reason?: string
  queued_at: string
  registered_at?: string
  completed_at?: string
}

export type RunnerSpecLifecycleEvidence = {
  name: string
  workflow_labels: string[]
  request_count: number
  registered_requests: number
  completed_requests: number
  cleanup_finalized_requests: number
  latest?: RunnerSpecLifecycleExample
  recent_attempts: RunnerSpecLifecycleAttempt[]
}

export type CatalogMigrationReadiness = {
  window_start: string
  window_end: string
  replay: {
    request_count: number
    distinct_input_count: number
    same: number
    legacy_only: number
    enabled_only: number
    different_profile: number
    errors: number
    truncated: boolean
  }
  replay_samples: CatalogMatchReplaySample[]
  specs: RunnerSpecLifecycleEvidence[]
  catalog_changes: AuditEvent[]
  catalog_changes_truncated: boolean
  automated_gates_passed: boolean
  gates: Array<{ code: CatalogMigrationGateCode; passed: boolean }>
  manual_requirements: CatalogMigrationManualRequirement[]
  current_process: {
    started_at: string
    catalog_match_counts: {
      same: number
      legacy_only: number
      enabled_only: number
      different_profile: number
    }
  }
}

export type AuditEvent = {
  id: number
  actor: string
  action: string
  resource_type: string
  resource_id: string
  payload_json?: string
  created_at: string
}

export type AuthSession = {
  authenticated: boolean
  oauth_enabled: boolean
  login?: string
  role?: string
  avatar_url?: string
  expires_at?: string
}

export type AccountRole = "admin" | "user"

export type AdminOAuthIdentity = {
  id: number
  account_id: number
  oauth_provider: string
  oauth_subject: string
  oauth_login: string
  created_at: string
  updated_at: string
}

export type AdminAccount = {
  id: number
  role: AccountRole
  created_at: string
  updated_at: string
  oauth_identities: AdminOAuthIdentity[]
}

export type AdminAccountStats = {
  total_accounts: number
  admin_accounts: number
  user_accounts: number
  oauth_identities: number
}

export type AdminAccountsResponse = {
  accounts: AdminAccount[]
  current_account_id: number
  stats: AdminAccountStats
  total: number
  limit: number
  offset: number
}

export type GitHubInstallation = {
  id: number
  account_id: number
  installation_id: number
  account_login?: string
  account_name?: string
  account_avatar?: string
  manageable?: boolean
  repositories: string[]
  created_at: string
  updated_at: string
}

export type GitHubAppConfig = {
  app_slug?: string
  install_url?: string
  setup_url: string
  settings_manageability?: boolean
  installations: GitHubInstallation[]
}

export type UserPreferences = {
  sandbox: {
    mode: "custom" | "inherit"
    resolved_source: "custom" | "inherited" | "admin_default" | "none"
    api_url: string
    manageable?: boolean
    inherited?: boolean
    source_account_id?: number
    source_account_login?: string
    source_is_current_account?: boolean
    source_available?: boolean
    api_key: {
      configured: boolean
      updated_at?: string
    }
  }
}

export type ProductTourOnboarding = {
  version: 1
  status: "pending" | "completed" | "skipped"
  tour_seen: boolean
}

export type SandboxServiceDefault = {
  enabled: boolean
  configured: boolean
  audience_mode: "all" | "selected"
  audiences: SandboxServiceDefaultAudience[]
  available_accounts: SandboxAudienceAccount[]
  api_url: string
  api_key: {
    configured: boolean
    updated_at?: string
  }
}

export type SandboxAudienceAccount = {
  github_account_id: number
  account_type: "user" | "organization"
  account_login: string
  account_name?: string
  account_avatar?: string
}

export type SandboxServiceDefaultAudience = SandboxAudienceAccount & {
  id: number
  created_at?: string
  updated_at?: string
}

export type AuthorizedRepositories = {
  installation_id: number
  repositories: string[]
}

export type SyncedGitHubInstallations = {
  installations: GitHubInstallation[]
}

export type Metric = {
  id: string
  label: string
  value: number
  description: string
}

export type SandboxTemplate = {
  template_id: string
  aliases: string[]
  build_status: string
  cpu_count: number
  memory_mb: number
  disk_size_mb: number
  public: boolean
  spawn_count: number
  updated_at: string
}

export type PublicRunnerTemplate = {
  default_template_name: string
  runner_spec_names: string[]
  workflow_labels: string[][]
}

export type SandboxInstance = {
  sandbox_id: string
  template_id: string
  alias?: string
  state: string
  cpu_count: number
  memory_mb: number
  disk_size_mb: number
  started_at: string
  expires_at: string
}

export const activeStatuses = new Set<RunnerStatus>(["queued", "creating", "running", "stopping"])
export const logNames = ["control.log", "stdout.log", "stderr.log"] as const
export const adminSections = [
  "overview",
  "accounts",
  "runner_requests",
  "runner_specs",
  "sandbox_service",
  "match",
  "audit",
  "diagnostics",
] as const

export type AdminSection = (typeof adminSections)[number]

export function sectionFromPath(): AdminSection {
  const slug = window.location.pathname.replace(/^\/admin\/?/, "") || "overview"
  return adminSections.includes(slug as AdminSection) ? (slug as AdminSection) : "overview"
}
