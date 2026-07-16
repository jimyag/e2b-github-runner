export type AccountRoleFilter = "all" | "admin" | "user"

export type AccountAvatarIdentity = {
  oauth_provider: string
  oauth_login: string
}

type AccountAvatarIdentities = AccountAvatarIdentity[] | null | undefined

export type AccountListQuery = {
  query: string
  role: AccountRoleFilter
  limit: number
  offset: number
}

export type AccountPageMeta = {
  page: number
  pages: number
  canPrevious: boolean
  canNext: boolean
}

export function accountAvatarURL(identities: AccountAvatarIdentities) {
  const githubIdentity = identities?.find(
    (identity) => identity.oauth_provider.trim().toLowerCase() === "github",
  )
  const login = githubIdentity?.oauth_login.trim()
  if (!login) return ""
  return `https://github.com/${encodeURIComponent(login)}.png?size=96`
}

export function accountAvatarImageURL(
  identities: AccountAvatarIdentities,
  failedAvatarURL: string,
) {
  const avatarURL = accountAvatarURL(identities)
  return avatarURL === failedAvatarURL ? "" : avatarURL
}

export function accountListQuery(input: AccountListQuery) {
  const params = new URLSearchParams()
  const query = input.query.trim()
  if (query) params.set("q", query)
  if (input.role !== "all") params.set("role", input.role)
  params.set("limit", String(Math.max(1, Math.trunc(input.limit))))
  params.set("offset", String(Math.max(0, Math.trunc(input.offset))))
  return params.toString()
}

export function accountPageMeta(total: number, limit: number, offset: number): AccountPageMeta {
  const safeTotal = Math.max(0, Math.trunc(total))
  const safeLimit = Math.max(1, Math.trunc(limit))
  const safeOffset = Math.max(0, Math.trunc(offset))
  const pages = Math.max(1, Math.ceil(safeTotal / safeLimit))
  const page = Math.min(pages, Math.floor(safeOffset / safeLimit) + 1)
  return {
    page,
    pages,
    canPrevious: page > 1,
    canNext: safeOffset + safeLimit < safeTotal,
  }
}
