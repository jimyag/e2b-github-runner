import type { AdminSection } from "@/admin-types"

export const userRunnerInitialPageSize = 100
export const userRunnerHistoryWindow = 500

export type AdminDataResource =
  | "runner_requests"
  | "runner_specs"
  | "runner_groups"
  | "runner_policies"
  | "audit_events"

export type UserDataResource = "github_app" | "runner_requests" | "preferences"

const adminResourcesBySection: Record<AdminSection, readonly AdminDataResource[]> = {
  overview: ["runner_requests", "runner_specs", "runner_policies"],
  accounts: [],
  runner_requests: ["runner_requests", "runner_specs"],
  runner_specs: ["runner_specs", "runner_groups"],
  runner_groups: ["runner_groups", "runner_specs"],
  runner_policies: ["runner_policies", "runner_specs", "runner_groups"],
  sandbox_service: [],
  match: [],
  audit: ["audit_events"],
  diagnostics: [],
}

export function adminDataResources(section: AdminSection): AdminDataResource[] {
  return [...adminResourcesBySection[section]]
}

export function shouldPollAdminSection(section: AdminSection): boolean {
  return section === "overview" || section === "runner_requests"
}

export function adminPollingResources(section: AdminSection): AdminDataResource[] {
  return shouldPollAdminSection(section) ? ["runner_requests"] : []
}

export function userDataResources(path: string): UserDataResource[] {
  if (isUserJobsRoute(path)) return ["github_app", "runner_requests"]
  if (path === "/repositories") return ["github_app"]
  if (isAccountSettingsRoute(path)) return ["github_app", "preferences"]
  return []
}

export function shouldPollUserRoute(path: string): boolean {
  return isUserJobsRoute(path)
}

export function userPollingResources(path: string): UserDataResource[] {
  return shouldPollUserRoute(path) ? ["runner_requests"] : []
}

export function userRunnerRequestLimit(path: string, polling: boolean): number {
  if (polling || path === "/") return userRunnerInitialPageSize
  return isUserJobsRoute(path) ? userRunnerHistoryWindow : userRunnerInitialPageSize
}

export function userRunnerRequestsPath(limit: number, offset: number): string {
  return `/user/runner_requests?limit=${limit}&offset=${offset}`
}

function isUserJobsRoute(path: string): boolean {
  return (
    path === "/" ||
    /^\/github\/(pulls|runs)\/[^/]+\/[^/]+\/[^/]+\/jobs$/.test(path) ||
    /^\/github\/branches\/[^/]+\/[^/]+\/.+\/jobs$/.test(path) ||
    /^\/jobs\/(pulls|runs|branches|manual)\//.test(path)
  )
}

function isAccountSettingsRoute(path: string): boolean {
  return (
    path === "/settings" ||
    path === "/accounts" ||
    /^\/account\/(repositories|preferences|sandbox|sandbox-templates|sandbox-instances)$/.test(path) ||
    /^\/organizations\/[^/]+\/(repositories|preferences|sandbox|sandbox-templates|sandbox-instances)$/.test(path)
  )
}
