import type { GitHubInstallation, UserPreferences } from "@/admin-types"

export type SandboxServiceReadiness = {
  ready: boolean
  loading: boolean
  source: UserPreferences["sandbox"]["resolved_source"]
  label: string
  description: string
}

export function sandboxServiceReadiness(
  preferences: UserPreferences | null,
): SandboxServiceReadiness {
  if (!preferences) {
    return {
      ready: false,
      loading: true,
      source: "none",
      label: "Checking configuration",
      description: "Resolving the effective Sandbox service for this account.",
    }
  }

  switch (preferences.sandbox.resolved_source) {
    case "custom":
      return {
        ready: true,
        loading: false,
        source: "custom",
        label: "Own credentials",
        description: "Using Sandbox service credentials configured for this account.",
      }
    case "inherited":
      return {
        ready: true,
        loading: false,
        source: "inherited",
        label: "Inherited credentials",
        description: "Using Sandbox service credentials inherited from a connected account.",
      }
    case "admin_default":
      return {
        ready: true,
        loading: false,
        source: "admin_default",
        label: "Platform provided",
        description: "This account is eligible for the platform Sandbox service.",
      }
    default:
      return {
        ready: false,
        loading: false,
        source: "none",
        label: "Configuration required",
        description: "Add a Sandbox service region and API Key before runner jobs can start.",
      }
  }
}

export function repositoryPath(accountLogin: string | undefined, currentLogin: string | undefined) {
  const normalizedAccount = accountLogin?.trim()
  const normalizedCurrent = currentLogin?.trim()
  if (!normalizedAccount || normalizedAccount.toLowerCase() === normalizedCurrent?.toLowerCase()) {
    return "/repositories"
  }
  return `/organizations/${encodeURIComponent(normalizedAccount)}/repositories`
}

export function repositoryAccountLogin(path: string, currentLogin: string | undefined): string | null {
  if (path === "/repositories" || path === "/account/repositories") {
    return currentLogin?.trim() || null
  }
  const match = path.match(/^\/organizations\/([^/]+)\/repositories$/)
  if (!match) return null
  try {
    return decodeURIComponent(match[1]) || null
  } catch {
    return null
  }
}

export function selectRepositoryInstallation<
  T extends Pick<GitHubInstallation, "id" | "account_login">,
>(
  installations: T[],
  selectedAccountLogin: string | undefined | null,
  currentLogin: string | undefined,
): T | undefined {
  const selectedLogin = selectedAccountLogin?.trim().toLowerCase()
  const current = currentLogin?.trim().toLowerCase()
  const selected = selectedLogin
    ? installations.find(
        (installation) => installation.account_login?.trim().toLowerCase() === selectedLogin,
      )
    : undefined
  if (selected) return selected
  if (!selectedLogin || selectedLogin === current) return installations[0]
  return undefined
}

export function repositoryPreferenceScope(
  installation: Pick<GitHubInstallation, "id" | "account_login"> | undefined,
  currentLogin: string | undefined,
): string {
  const installationLogin = installation?.account_login?.trim().toLowerCase()
  const current = currentLogin?.trim().toLowerCase()
  return installation && installationLogin && installationLogin !== current
    ? `github_installation:${installation.id}`
    : "account"
}

export function repositoryRows(
  authorizedRepositories: string[],
  repositoriesWithJobs: string[],
): Array<{ name: string; hasJobs: boolean }> {
  const active = new Set(repositoriesWithJobs.map((repository) => repository.trim().toLowerCase()))
  return [...authorizedRepositories]
    .sort((left, right) => left.localeCompare(right, undefined, { sensitivity: "base" }))
    .map((name) => ({ name, hasJobs: active.has(name.trim().toLowerCase()) }))
}
