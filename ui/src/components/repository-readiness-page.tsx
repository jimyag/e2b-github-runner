import {
  BookOpen,
  Check,
  ChevronRight,
  CircleAlert,
  Github,
  KeyRound,
  Loader2,
  Search,
} from "lucide-react"
import {
  useEffect,
  useMemo,
  useState,
} from "react"

import type {
  GitHubAppConfig,
  GitHubInstallation,
  UserPreferences,
} from "@/admin-types"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"
import {
  repositoryRows,
  sandboxServiceReadiness,
  selectRepositoryInstallation,
} from "@/repository-readiness"

export function RepositoryReadinessPage({
  githubApp,
  currentLogin,
  selectedAccountLogin,
  preferences,
  preferencesLoading,
  authorizedRepositories,
  repositoryErrors,
  loadingRepositoriesFor,
  syncingGitHubInstallations,
  onLoadAuthorizedRepositories,
  onSyncGitHubInstallations,
  onSelectAccount,
}: {
  githubApp: GitHubAppConfig | null
  currentLogin?: string
  selectedAccountLogin?: string | null
  preferences: UserPreferences | null
  preferencesLoading: boolean
  authorizedRepositories: Record<number, string[]>
  repositoryErrors: Record<number, string>
  loadingRepositoriesFor: number | null
  syncingGitHubInstallations: boolean
  onLoadAuthorizedRepositories: (id: number) => void
  onSyncGitHubInstallations: () => void
  onSelectAccount: (accountLogin: string | undefined) => void
}) {
  const installations = githubApp?.installations ?? []
  const selected = selectRepositoryInstallation(
    installations,
    selectedAccountLogin,
    currentLogin,
  )
  const authorized = selected ? authorizedRepositories[selected.id] : undefined
  const repositoryError = selected ? repositoryErrors[selected.id] : undefined
  const [filter, setFilter] = useState("")
  const readiness = sandboxServiceReadiness(preferencesLoading ? null : preferences)
  const rows = useMemo(
    () => repositoryRows(authorized ?? [], selected?.repositories ?? []),
    [authorized, selected?.repositories],
  )
  const filteredRows = useMemo(() => {
    const query = filter.trim().toLowerCase()
    return query
      ? rows.filter((repository) => repository.name.toLowerCase().includes(query))
      : rows
  }, [filter, rows])
  const repositoriesWithJobs = rows.filter((repository) => repository.hasJobs).length
  const sandboxManageable = preferences?.sandbox.manageable !== false
  const repositoryOnlyAccess = !readiness.loading && !sandboxManageable
  const selectedLogin = selected?.account_login?.trim() ?? ""
  const personalScope =
    !selectedLogin ||
    selectedLogin.toLowerCase() === currentLogin?.trim().toLowerCase()
  const sandboxSettingsURL = personalScope
    ? "/account/preferences"
    : `/organizations/${encodeURIComponent(selectedLogin)}/preferences`
  const sandboxProvider = readiness.source === "admin_default"
    ? "Provided by the platform."
    : personalScope
      ? readiness.source === "inherited"
        ? "Inherited from a connected account."
        : "Provided by your account."
      : `Provided by the ${selectedLogin} organization.`
  const missingSandboxDescription = personalScope
    ? "Your account has not configured a Sandbox service."
    : `The ${selectedLogin} organization has not configured a Sandbox service.`
  const repositoryAccessURL = repositoryOnlyAccess
    ? `https://github.com/${encodeURIComponent(selected?.account_login ?? "")}`
    : `https://github.com/settings/installations/${selected?.installation_id ?? ""}`

  useEffect(() => {
    if (
      selected &&
      !authorizedRepositories[selected.id] &&
      !repositoryErrors[selected.id] &&
      loadingRepositoriesFor !== selected.id
    ) {
      onLoadAuthorizedRepositories(selected.id)
    }
  }, [
    authorizedRepositories,
    loadingRepositoriesFor,
    onLoadAuthorizedRepositories,
    repositoryErrors,
    selected,
  ])

  return (
    <>
      <section
        className="border-b bg-muted/35 px-4 py-4 lg:px-6"
        data-onboarding="repository-readiness"
      >
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h1 className="text-xl font-semibold">Repositories</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Connect GitHub repositories and confirm the Sandbox service they need to run.
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {githubApp?.install_url ? (
              <>
                <Button
                  type="button"
                  variant="outline"
                  disabled={syncingGitHubInstallations}
                  onClick={onSyncGitHubInstallations}
                >
                  {syncingGitHubInstallations ? (
                    <Loader2 className="animate-spin" />
                  ) : (
                    <Github />
                  )}
                  {syncingGitHubInstallations ? "Syncing..." : "Sync installations"}
                </Button>
                <Button type="button" asChild>
                  <a
                    href={githubApp.install_url}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    <Github />
                    Install GitHub App
                  </a>
                </Button>
              </>
            ) : (
              <Badge variant="outline">GitHub App installation is unavailable</Badge>
            )}
          </div>
        </div>
      </section>

      <div className="grid min-h-0 flex-1 lg:grid-cols-[320px_minmax(0,1fr)]">
        <aside className="min-h-0 border-r bg-muted/20">
          <div className="min-h-0 h-full overflow-y-auto">
            {installations.length ? (
              installations.map((installation) => (
                <button
                  key={installation.id}
                  type="button"
                  className={cn(
                    "flex h-14 w-full items-center gap-3 border-b px-4 text-left transition-colors hover:bg-accent",
                    selected?.id === installation.id ? "bg-accent" : "",
                  )}
                  onClick={() => onSelectAccount(installation.account_login)}
                >
                  <InstallationAvatar installation={installation} />
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-semibold">
                      {installation.account_name || installation.account_login || "GitHub App"}
                    </div>
                    {installation.account_name && installation.account_login ? (
                      <div className="truncate text-xs text-muted-foreground">
                        {installation.account_login}
                      </div>
                    ) : null}
                  </div>
                  <ChevronRight className="h-4 w-4 text-muted-foreground" />
                </button>
              ))
            ) : (
              <div className="space-y-3 p-4 text-sm text-muted-foreground">
                <p>Install the GitHub App or sync an existing installation to get started.</p>
                {githubApp?.install_url ? (
                  <Button type="button" size="sm" asChild>
                    <a
                      href={githubApp.install_url}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      <Github />
                      Install GitHub App
                    </a>
                  </Button>
                ) : null}
              </div>
            )}
          </div>
        </aside>

        <section className="min-h-0 overflow-y-auto p-4 lg:p-6">
          {selected ? (
            <div className="mx-auto max-w-6xl space-y-5">
              <div className="flex items-center gap-3">
                <InstallationAvatar installation={selected} size="lg" />
                <div className="min-w-0">
                  <h2 className="truncate text-2xl font-semibold">
                    {selected.account_name || selected.account_login || "GitHub App"}
                  </h2>
                  <div className="truncate text-sm text-muted-foreground">
                    {selected.account_login || currentLogin || "GitHub"}
                  </div>
                </div>
              </div>

              <div>
                <h3 className="text-sm font-semibold uppercase tracking-[0.12em] text-muted-foreground">
                  Runner readiness
                </h3>
                <div className="mt-3 grid gap-3 xl:grid-cols-2">
                  <Card className="gap-4 rounded-lg py-5">
                    <CardHeader className="gap-3 px-5">
                      <div className="flex items-start justify-between gap-3">
                        <div className="flex min-w-0 gap-3">
                          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border bg-muted/35">
                            <BookOpen className="h-4 w-4" />
                          </div>
                          <div className="min-w-0">
                            <CardTitle className="text-base">Repository access</CardTitle>
                            <CardDescription className="mt-1">
                              Repositories available to both you and this GitHub App installation.
                            </CardDescription>
                          </div>
                        </div>
                        {repositoryError ? (
                          <Badge variant="destructive">
                            <CircleAlert />
                            Access unavailable
                          </Badge>
                        ) : loadingRepositoriesFor === selected.id || authorized === undefined ? (
                          <Badge variant="outline">Checking access</Badge>
                        ) : authorized.length ? (
                          <Badge variant="success">
                            <Check />
                            Connected
                          </Badge>
                        ) : (
                          <Badge variant="warning">
                            <CircleAlert />
                            Choose repositories
                          </Badge>
                        )}
                      </div>
                    </CardHeader>
                    <CardContent className="flex flex-wrap items-center justify-between gap-3 px-5">
                      <div className="text-sm text-muted-foreground">
                        {repositoryError
                          ? "Unable to load repository authorization from GitHub."
                          : authorized === undefined
                          ? "Loading authorization from GitHub."
                          : `${authorized.length} authorized · ${repositoriesWithJobs} with jobs`}
                      </div>
                      <Button type="button" variant="outline" size="sm" asChild>
                        <a
                          href={repositoryAccessURL}
                          target="_blank"
                          rel="noopener noreferrer"
                        >
                          <Github />
                          {repositoryOnlyAccess ? "View on GitHub" : "Manage on GitHub"}
                        </a>
                      </Button>
                    </CardContent>
                  </Card>

                  <Card
                    className={cn(
                      "gap-4 rounded-lg py-5",
                      !readiness.loading && !readiness.ready
                        ? "border-yellow-300 bg-yellow-50/40 dark:border-yellow-900 dark:bg-yellow-950/10"
                        : "",
                    )}
                    data-onboarding="sandbox-service"
                  >
                    <CardHeader className="gap-3 px-5">
                      <div className="flex items-start justify-between gap-3">
                        <div className="flex min-w-0 gap-3">
                          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border bg-muted/35">
                            <KeyRound className="h-4 w-4" />
                          </div>
                          <div className="min-w-0">
                            <CardTitle className="text-base">Sandbox service</CardTitle>
                            <CardDescription className="mt-1">
                              {readiness.loading
                                ? "Checking the effective Sandbox service for this account."
                                : readiness.ready
                                  ? sandboxProvider
                                  : missingSandboxDescription}
                            </CardDescription>
                          </div>
                        </div>
                        <Badge
                          variant={
                            readiness.loading
                              ? "outline"
                              : readiness.ready
                                ? "success"
                                : "warning"
                          }
                        >
                          {readiness.ready
                            ? <Check />
                            : readiness.loading
                              ? <Loader2 className="animate-spin" />
                              : <CircleAlert />}
                          {readiness.loading
                            ? "Checking"
                            : readiness.ready
                              ? "Ready"
                              : sandboxManageable
                                ? "Setup required"
                                : "Unavailable"}
                        </Badge>
                      </div>
                    </CardHeader>
                    <CardContent className="flex flex-wrap items-center justify-between gap-3 px-5">
                      <div className="text-sm text-muted-foreground">
                        {readiness.ready
                          ? "Runner jobs can use this Sandbox service."
                          : readiness.loading
                            ? "Checking account and platform configuration."
                            : sandboxManageable
                              ? "Runner jobs cannot start until Sandbox is configured."
                              : "Ask an organization member to configure Sandbox."}
                      </div>
                      {!readiness.loading && sandboxManageable ? (
                        <Button
                          type="button"
                          variant={readiness.ready ? "outline" : "default"}
                          size="sm"
                          asChild
                        >
                          <a href={sandboxSettingsURL}>
                            <KeyRound />
                            {readiness.ready ? "Sandbox settings" : "Configure Sandbox"}
                          </a>
                        </Button>
                      ) : null}
                    </CardContent>
                  </Card>
                </div>
              </div>

              <Card className="gap-4 rounded-lg py-5">
                <CardHeader className="gap-3 px-5">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <CardTitle className="text-base">Authorized repositories</CardTitle>
                      <CardDescription className="mt-1">
                        Job activity is shown without hiding repositories that have not run yet.
                      </CardDescription>
                    </div>
                    <Badge variant="secondary">
                      {authorized === undefined ? "Loading" : `${authorized.length} repositories`}
                    </Badge>
                  </div>
                </CardHeader>
                <CardContent className="space-y-3 px-5">
                  <div className="relative">
                    <Search className="pointer-events-none absolute left-3 top-2.5 h-4 w-4 text-muted-foreground" />
                    <Input
                      className="pl-9"
                      placeholder="Filter repositories"
                      value={filter}
                      onChange={(event) => setFilter(event.target.value)}
                    />
                  </div>
                      {repositoryError ? (
                        <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-3">
                          <div className="min-w-0">
                            <div className="text-sm font-medium">Unable to load repositories</div>
                            <div className="mt-1 break-words text-sm text-muted-foreground">
                              {repositoryError}
                            </div>
                          </div>
                          <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            onClick={() => onLoadAuthorizedRepositories(selected.id)}
                          >
                            Retry
                          </Button>
                        </div>
                      ) : loadingRepositoriesFor === selected.id || authorized === undefined ? (
                    <div className="rounded-md border bg-muted/25 px-3 py-3 text-sm text-muted-foreground">
                      Loading repositories from GitHub...
                    </div>
                  ) : filteredRows.length ? (
                    <div className="divide-y rounded-md border">
                      {filteredRows.map((repository) => (
                        <div
                          key={repository.name}
                          className="flex min-h-12 items-center justify-between gap-3 px-3 py-2"
                        >
                          <div className="min-w-0 truncate text-sm font-medium">
                            {repository.name}
                          </div>
                          <Badge variant={repository.hasJobs ? "success" : "outline"}>
                            {repository.hasJobs ? "Has jobs" : "No jobs yet"}
                          </Badge>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <div className="rounded-md border bg-muted/25 px-3 py-3 text-sm text-muted-foreground">
                      {authorized.length
                        ? "No repositories match this filter."
                        : "No repositories are authorized. Choose repositories on GitHub to continue."}
                    </div>
                  )}
                </CardContent>
              </Card>
            </div>
          ) : (
            <div className="rounded-lg border bg-muted/30 p-6">
              <h2 className="text-base font-semibold">Connect a GitHub account or organization</h2>
              <p className="mt-1 text-sm text-muted-foreground">
                Install the GitHub App, then return here to finish repository and Sandbox setup.
              </p>
            </div>
          )}
        </section>
      </div>
    </>
  )
}

function InstallationAvatar({
  installation,
  size = "sm",
}: {
  installation: GitHubInstallation
  size?: "sm" | "lg"
}) {
  const dimensions = size === "lg" ? "h-12 w-12 rounded-xl" : "h-8 w-8 rounded-lg"
  const label = installation.account_name || installation.account_login || "GitHub"
  if (installation.account_avatar) {
    return (
      <img
        src={installation.account_avatar}
        alt=""
        className={cn(dimensions, "shrink-0 border bg-muted object-cover")}
        referrerPolicy="no-referrer"
      />
    )
  }
  return (
    <div
      className={cn(
        dimensions,
        "flex shrink-0 items-center justify-center border bg-muted text-xs font-semibold",
      )}
      aria-label={`${label} avatar`}
    >
      {label.slice(0, 2).toUpperCase()}
    </div>
  )
}
