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
  template_id: string
  runner_group?: string
  max_concurrency: number
  min_idle: number
  priority: number
  enabled: boolean
  default_available: boolean
  created_at: string
  updated_at: string
}

export type RunnerPolicy = {
  id: number
  repository_full_name: string
  runner_spec_name?: string
  runner_group_name?: string
  enabled: boolean
  created_at: string
}

export type RunnerGroup = {
  name: string
  description?: string
  spec_names: string[]
  enabled: boolean
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

export type GitHubInstallation = {
  id: number
  account_id: number
  installation_id: number
  account_login?: string
  account_name?: string
  account_avatar?: string
  repositories: string[]
  created_at: string
  updated_at: string
}

export type GitHubAppConfig = {
  app_slug?: string
  install_url?: string
  setup_url: string
  installations: GitHubInstallation[]
}

export type UserPreferences = {
  sandbox: {
    mode: "custom" | "inherit"
    api_url: string
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

export type AuthorizedRepositories = {
  installation_id: number
  repositories: string[]
}

export type Metric = {
  label: string
  value: number
  description: string
}

export const activeStatuses = new Set<RunnerStatus>(["queued", "creating", "running", "stopping"])
export const logNames = ["control.log", "stdout.log", "stderr.log"] as const
export const adminSections = [
  "overview",
  "runner_requests",
  "runner_specs",
  "runner_groups",
  "runner_policies",
  "match",
  "audit",
  "diagnostics",
] as const

export type AdminSection = (typeof adminSections)[number]

export function sectionFromPath(): AdminSection {
  const slug = window.location.pathname.replace(/^\/admin\/?/, "") || "overview"
  return adminSections.includes(slug as AdminSection) ? (slug as AdminSection) : "overview"
}
