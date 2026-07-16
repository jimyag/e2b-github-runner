import { type FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react"
import {
  AtSign,
  ChevronLeft,
  ChevronRight,
  KeyRound,
  Loader2,
  RefreshCw,
  Search,
  ShieldCheck,
  UserRoundCog,
} from "lucide-react"
import { toast } from "sonner"

import {
  type AccountRole,
  type AdminAccount,
  type AdminAccountStats,
  type AdminAccountsResponse,
} from "@/admin-types"
import { formatTime } from "@/admin-format"
import {
  accountAvatarImageURL,
  accountListQuery,
  accountPageMeta,
  type AccountAvatarIdentity,
  type AccountRoleFilter,
} from "@/components/accounts-section-utils"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

type RequestFunction = (url: string, options?: RequestInit) => Promise<unknown>

export type PendingRoleChange = {
  account: AdminAccount
  role: AccountRole
}

const emptyAccountStats: AdminAccountStats = {
  total_accounts: 0,
  admin_accounts: 0,
  user_accounts: 0,
  oauth_identities: 0,
}

export function AccountAvatar({
  identities,
  displayLogin,
}: {
  identities: AccountAvatarIdentity[] | null | undefined
  displayLogin: string
}) {
  const [failedAvatarURL, setFailedAvatarURL] = useState("")
  const avatarURL = accountAvatarImageURL(identities, failedAvatarURL)
  return (
    <div
      className="relative grid size-9 shrink-0 place-items-center overflow-hidden rounded-full border bg-primary/8 font-mono text-sm font-semibold text-primary"
      aria-hidden="true"
    >
      <span>{displayLogin.slice(0, 1).toUpperCase()}</span>
      {avatarURL ? (
        <img
          src={avatarURL}
          alt=""
          className="absolute inset-0 size-full object-cover"
          loading="lazy"
          decoding="async"
          referrerPolicy="no-referrer"
          onError={() => setFailedAvatarURL(avatarURL)}
        />
      ) : null}
    </div>
  )
}

export function AccountRoleChangeDialog({
  pendingRoleChange,
  pendingLogin,
  saving,
  error,
  onClose,
  onConfirm,
}: {
  pendingRoleChange: PendingRoleChange | null
  pendingLogin: string
  saving: boolean
  error: string
  onClose: () => void
  onConfirm: () => void
}) {
  return (
    <Dialog
      open={pendingRoleChange !== null}
      onOpenChange={(open) => {
        if (!open && !saving) onClose()
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Change account role?</DialogTitle>
          <DialogDescription>
            @{pendingLogin} will {pendingRoleChange?.role === "admin" ? "gain access to all runner management APIs" : "lose access to the admin console"}. This change takes effect immediately and is recorded in the audit log.
          </DialogDescription>
        </DialogHeader>
        <div className="flex items-center justify-between rounded-lg border bg-muted/30 p-3 text-sm">
          <span className="text-muted-foreground">Role change</span>
          <span className="inline-flex items-center gap-2 font-medium">
            {pendingRoleChange?.account.role}
            <ChevronRight className="size-3.5 text-muted-foreground" />
            {pendingRoleChange?.role}
          </span>
        </div>
        {error ? (
          <div className="text-sm text-destructive" role="alert">
            {error}
          </div>
        ) : null}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose} disabled={saving}>
            Cancel
          </Button>
          <Button type="button" onClick={onConfirm} disabled={saving}>
            {saving ? <Loader2 className="animate-spin" /> : <ShieldCheck />}
            Confirm role change
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function AccountsSection({ request }: { request: RequestFunction }) {
  const [accounts, setAccounts] = useState<AdminAccount[]>([])
  const [currentAccountID, setCurrentAccountID] = useState(0)
  const [stats, setStats] = useState<AdminAccountStats>(emptyAccountStats)
  const [total, setTotal] = useState(0)
  const [draftQuery, setDraftQuery] = useState("")
  const [query, setQuery] = useState("")
  const [role, setRole] = useState<AccountRoleFilter>("all")
  const [limit, setLimit] = useState(20)
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(true)
  const [savingAccountID, setSavingAccountID] = useState(0)
  const [error, setError] = useState("")
  const [roleError, setRoleError] = useState("")
  const [pendingRoleChange, setPendingRoleChange] = useState<PendingRoleChange | null>(null)
  const loadVersion = useRef(0)

  const load = useCallback(async () => {
    const version = ++loadVersion.current
    setLoading(true)
    setError("")
    try {
      const params = accountListQuery({ query, role, limit, offset })
      const data = (await request(`/admin/api/accounts?${params}`)) as AdminAccountsResponse
      if (version !== loadVersion.current) return
      if (!data || !Array.isArray(data.accounts) || !data.stats) {
        throw new Error("Invalid account list response")
      }
      if (data.total > 0 && offset >= data.total) {
        setOffset(Math.floor((data.total - 1) / limit) * limit)
        return
      }
      setAccounts(data.accounts)
      setCurrentAccountID(data.current_account_id)
      setStats(data.stats)
      setTotal(data.total)
    } catch (cause) {
      if (version !== loadVersion.current) return
      setError(cause instanceof Error ? cause.message : "Failed to load accounts")
    } finally {
      if (version === loadVersion.current) setLoading(false)
    }
  }, [limit, offset, query, request, role])

  useEffect(() => {
    void load()
    return () => {
      loadVersion.current += 1
    }
  }, [load])

  const page = useMemo(() => accountPageMeta(total, limit, offset), [limit, offset, total])
  const resultStart = total === 0 ? 0 : offset + 1
  const resultEnd = Math.min(offset + accounts.length, total)
  const pendingLogin = pendingRoleChange?.account.oauth_identities?.[0]?.oauth_login || `account #${pendingRoleChange?.account.id || ""}`

  const applySearch = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const nextQuery = draftQuery.trim()
    if (nextQuery === query && offset === 0) {
      void load()
      return
    }
    setQuery(nextQuery)
    setOffset(0)
  }

  const clearFilters = () => {
    setDraftQuery("")
    setQuery("")
    setRole("all")
    setOffset(0)
  }

  const closeRoleChangeDialog = () => {
    setPendingRoleChange(null)
    setRoleError("")
  }

  const updateRole = async () => {
    if (!pendingRoleChange) return
    const { account, role: nextRole } = pendingRoleChange
    setSavingAccountID(account.id)
    setRoleError("")
    try {
      await request(`/admin/api/accounts/${encodeURIComponent(String(account.id))}/role`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ role: nextRole }),
      })
      closeRoleChangeDialog()
      toast.success(`@${pendingLogin} is now ${nextRole}`)
      await load()
    } catch (cause) {
      setRoleError(cause instanceof Error ? cause.message : "Failed to update account role")
    } finally {
      setSavingAccountID(0)
    }
  }

  return (
    <>
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        {[
          { label: "Accounts", value: stats.total_accounts, description: "local access principals" },
          { label: "Administrators", value: stats.admin_accounts, description: "full management access" },
          { label: "Users", value: stats.user_accounts, description: "standard account access" },
          { label: "Linked identities", value: stats.oauth_identities, description: "OAuth provider bindings" },
        ].map((metric) => (
          <Card key={metric.label} className="gap-3 py-5">
            <CardHeader className="px-5">
              <CardDescription>{metric.label}</CardDescription>
              <CardTitle className="text-3xl tabular-nums">{metric.value}</CardTitle>
            </CardHeader>
            <CardContent className="px-5 text-xs text-muted-foreground">
              {metric.description}
            </CardContent>
          </Card>
        ))}
      </div>

      <Card className="min-w-0 gap-0 overflow-hidden py-0">
        <CardHeader className="border-b px-5 py-4">
          <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
            <div className="min-w-0">
              <CardTitle className="flex items-center gap-2 text-base">
                <UserRoundCog className="size-4 text-primary" />
                Accounts
              </CardTitle>
              <CardDescription className="mt-1">
                Search linked identities and control access to the runner management plane.
              </CardDescription>
            </div>
            <div className="flex items-center gap-2">
              <Badge variant="outline" className="font-mono tabular-nums">
                {total} {total === 1 ? "account" : "accounts"}
              </Badge>
              <Button
                type="button"
                variant="outline"
                size="icon"
                onClick={() => void load()}
                disabled={loading}
                aria-label="Refresh accounts"
                title="Refresh accounts"
              >
                <RefreshCw className={loading ? "animate-spin" : ""} />
              </Button>
            </div>
          </div>
        </CardHeader>

        <CardContent className="grid gap-0 p-0">
          <div className="flex flex-col gap-3 border-b bg-muted/20 p-4 lg:flex-row lg:items-center">
            <form className="flex min-w-0 flex-1 gap-2" onSubmit={applySearch}>
              <div className="relative min-w-0 flex-1 lg:max-w-md">
                <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={draftQuery}
                  onChange={(event) => setDraftQuery(event.target.value)}
                  className="pl-9"
                  placeholder="Login, provider, or stable subject"
                  aria-label="Search accounts"
                />
              </div>
              <Button type="submit" variant="secondary">
                Search
              </Button>
            </form>
            <div className="flex flex-wrap items-center gap-2">
              <Select
                value={role}
                onValueChange={(value) => {
                  setRole(value as AccountRoleFilter)
                  setOffset(0)
                }}
              >
                <SelectTrigger className="w-[150px]" aria-label="Filter by role">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All roles</SelectItem>
                  <SelectItem value="admin">Admins</SelectItem>
                  <SelectItem value="user">Users</SelectItem>
                </SelectContent>
              </Select>
              {query || role !== "all" ? (
                <Button type="button" variant="ghost" onClick={clearFilters}>
                  Clear
                </Button>
              ) : null}
            </div>
          </div>

          {error ? (
            <div className="border-b border-destructive/20 bg-destructive/5 px-5 py-3 text-sm text-destructive" role="alert">
              {error}
            </div>
          ) : null}

          <Table>
            <TableHeader>
              <TableRow className="bg-muted/10">
                <TableHead className="pl-5">Account</TableHead>
                <TableHead>Linked identities</TableHead>
                <TableHead>Created</TableHead>
                <TableHead>Updated</TableHead>
                <TableHead className="pr-5 text-right">Role</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading && accounts.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="h-36 text-center text-muted-foreground">
                    <span className="inline-flex items-center gap-2">
                      <Loader2 className="size-4 animate-spin" />
                      Loading accounts…
                    </span>
                  </TableCell>
                </TableRow>
              ) : accounts.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="h-36 text-center">
                    <div className="mx-auto grid max-w-sm gap-2">
                      <div className="font-medium">No accounts found</div>
                      <div className="text-sm text-muted-foreground">
                        Try another identity search or remove the role filter.
                      </div>
                    </div>
                  </TableCell>
                </TableRow>
              ) : (
                accounts.map((account) => {
                  const identities = account.oauth_identities ?? []
                  const primaryIdentity = identities[0]
                  const displayLogin = primaryIdentity?.oauth_login || `account-${account.id}`
                  const isCurrent = account.id === currentAccountID
                  const isSaving = savingAccountID === account.id
                  return (
                    <TableRow key={account.id}>
                      <TableCell className="pl-5">
                        <div className="flex items-center gap-3">
                          <AccountAvatar identities={identities} displayLogin={displayLogin} />
                          <div className="min-w-0">
                            <div className="flex items-center gap-2">
                              <span className="max-w-48 truncate font-medium">@{displayLogin}</span>
                              {isCurrent ? <Badge variant="secondary">You</Badge> : null}
                            </div>
                            <div className="font-mono text-xs text-muted-foreground">account #{account.id}</div>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell>
                        <div className="flex max-w-xl flex-wrap gap-1.5">
                          {identities.map((identity) => (
                            <span
                              key={identity.id}
                              className="inline-flex items-center gap-1 rounded-md border bg-background px-2 py-1 text-xs shadow-xs"
                              title={`${identity.oauth_provider} subject ${identity.oauth_subject}`}
                            >
                              <AtSign className="size-3 text-muted-foreground" />
                              <span className="font-medium">{identity.oauth_login}</span>
                              <span className="font-mono text-[10px] text-muted-foreground uppercase">
                                {identity.oauth_provider}
                              </span>
                            </span>
                          ))}
                        </div>
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">{formatTime(account.created_at)}</TableCell>
                      <TableCell className="text-xs text-muted-foreground">{formatTime(account.updated_at)}</TableCell>
                      <TableCell className="pr-5">
                        <div className="flex justify-end">
                          <Select
                            value={account.role}
                            disabled={isCurrent || isSaving}
                            onValueChange={(value) => {
                              if (value !== account.role) {
                                setRoleError("")
                                setPendingRoleChange({ account, role: value as AccountRole })
                              }
                            }}
                          >
                            <SelectTrigger
                              size="sm"
                              className="w-[116px] font-medium"
                              aria-label={`Role for ${displayLogin}`}
                              title={isCurrent ? "You cannot change your own role" : "Change account role"}
                            >
                              {isSaving ? <Loader2 className="animate-spin" /> : account.role === "admin" ? <ShieldCheck /> : <KeyRound />}
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent align="end">
                              <SelectItem value="admin">Admin</SelectItem>
                              <SelectItem value="user">User</SelectItem>
                            </SelectContent>
                          </Select>
                        </div>
                      </TableCell>
                    </TableRow>
                  )
                })
              )}
            </TableBody>
          </Table>

          <div className="flex flex-col gap-3 border-t bg-muted/10 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="text-xs text-muted-foreground tabular-nums">
              {total === 0 ? "No matching accounts" : `${resultStart}–${resultEnd} of ${total}`}
            </div>
            <div className="flex flex-wrap items-center gap-2 sm:justify-end">
              <Select
                value={String(limit)}
                onValueChange={(value) => {
                  setLimit(Number(value))
                  setOffset(0)
                }}
              >
                <SelectTrigger size="sm" className="min-w-32 shrink-0" aria-label="Accounts per page">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent align="end">
                  <SelectItem value="10">10 / page</SelectItem>
                  <SelectItem value="20">20 / page</SelectItem>
                  <SelectItem value="50">50 / page</SelectItem>
                </SelectContent>
              </Select>
              <span className="min-w-24 text-center text-xs font-medium tabular-nums">
                Page {page.page} of {page.pages}
              </span>
              <Button
                type="button"
                variant="outline"
                size="icon"
                disabled={!page.canPrevious || loading}
                onClick={() => setOffset(Math.max(0, offset - limit))}
                aria-label="Previous account page"
              >
                <ChevronLeft />
              </Button>
              <Button
                type="button"
                variant="outline"
                size="icon"
                disabled={!page.canNext || loading}
                onClick={() => setOffset(offset + limit)}
                aria-label="Next account page"
              >
                <ChevronRight />
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      <AccountRoleChangeDialog
        pendingRoleChange={pendingRoleChange}
        pendingLogin={pendingLogin}
        saving={savingAccountID !== 0}
        error={roleError}
        onClose={closeRoleChangeDialog}
        onConfirm={() => void updateRole()}
      />
    </>
  )
}
