type SandboxAudienceIdentity = {
  github_account_id: number
  account_type: string
}

type SandboxRegion = {
  apiURL: string
  id?: string
}

function normalizeSandboxAPIURL(value: string) {
  return value.trim().replace(/\/+$/, "").toLowerCase()
}

export function sandboxServiceDefaultAPIURL(value: string, regions: readonly SandboxRegion[]) {
  const trimmed = value.trim()
  const normalized = normalizeSandboxAPIURL(trimmed)
  const exact = regions.find((item) => normalizeSandboxAPIURL(item.apiURL) === normalized)
  if (exact) return exact.apiURL
  try {
    const hostname = new URL(normalized).hostname.toLowerCase()
    const region = regions.find((item) => {
      const configuredHostname = new URL(item.apiURL).hostname.toLowerCase()
      return hostname === configuredHostname || (item.id && (hostname.startsWith(`${item.id}.`) || hostname.startsWith(`${item.id}-`)))
    })
    return region?.apiURL ?? trimmed
  } catch {
    return trimmed
  }
}

export function sandboxServiceDefaultStatus(config: { enabled: boolean; configured: boolean }) {
  if (!config.configured) return "Incomplete" as const
  return config.enabled ? ("Enabled" as const) : ("Disabled" as const)
}

export function sandboxAudienceIdentityKey(account: SandboxAudienceIdentity) {
  return `${account.account_type.toLowerCase()}:${account.github_account_id}`
}

export function normalizeSandboxAudienceLogin(value: string) {
  return value.trim().replace(/^@+/, "")
}

export function availableSandboxAudienceAccounts<T extends SandboxAudienceIdentity>(
  available: T[],
  selected: SandboxAudienceIdentity[],
) {
  const selectedKeys = new Set(selected.map(sandboxAudienceIdentityKey))
  return available.filter((account) => !selectedKeys.has(sandboxAudienceIdentityKey(account)))
}

export function sandboxAudienceSummary(mode: "all" | "selected", selectedCount: number) {
  if (mode === "all") return "Available to all GitHub accounts"
  if (selectedCount === 0) return "No GitHub accounts selected"
  return `Available to ${selectedCount} selected ${selectedCount === 1 ? "account" : "accounts"}`
}

export function sandboxConfigSourceLabel(source?: string) {
  switch (source) {
    case "installation":
      return "GitHub installation"
    case "account":
      return "Account"
    case "inherited_account":
      return "Inherited account"
    case "admin_default":
      return "Admin default"
    case "request_snapshot":
      return "Saved request snapshot"
    default:
      return "—"
  }
}
