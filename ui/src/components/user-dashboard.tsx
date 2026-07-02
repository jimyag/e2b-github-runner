import {
  AlertCircle,
  BookOpen,
  CalendarDays,
  ChevronDown,
  Check,
  ExternalLink,
  Github,
  KeyRound,
  LogOut,
  Monitor,
  Moon,
  Play,
  Settings,
  ShieldCheck,
  Sun,
  Workflow,
  X,
} from "lucide-react"
import { useTheme } from "next-themes"
import { type FormEvent, type MouseEvent, type ReactNode, useEffect, useMemo, useState } from "react"

import type { AuthSession, GitHubAppConfig, RunnerState, UserPreferences } from "@/admin-types"
import { formatTime } from "@/admin-format"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { cn } from "@/lib/utils"

type BuildGroupKind = "pull_request" | "branch" | "workflow_run" | "manual"
type RunnerStatusSummary = "completed" | "active" | "failed"

type BuildGroup = {
  key: string
  kind: BuildGroupKind
  repository: string
  title: string
  subtitle: string
  updatedAt: string
  jobs: RunnerState[]
  workflowRunIDs: number[]
  headSHA?: string
  headBranch?: string
  pullRequestNumber?: number
}

type UserPage = "home" | "repositories" | "settings"
type AccountSettingsTab = "repositories" | "preferences"
type AccountSettingsRoute = {
  accountLogin?: string
  tab: AccountSettingsTab
}

export function UserDashboard({
  authSession,
  githubApp,
  userPreferences,
  runners,
  selectedKey,
  page,
  accountSettingsRoute,
  authorizedRepositories,
  loadingRepositoriesFor,
  onLoadAuthorizedRepositories,
  onSaveSandboxConfig,
  onDeleteSandboxAPIKey,
  onNavigate,
  onNavigateAccountSettings,
  onSelectKey,
  onSignOut,
}: {
  authSession: AuthSession
  githubApp: GitHubAppConfig | null
  userPreferences: UserPreferences | null
  runners: RunnerState[]
  selectedKey: string
  page: UserPage
  accountSettingsRoute: AccountSettingsRoute
  authorizedRepositories: Record<number, string[]>
  loadingRepositoriesFor: number | null
  onLoadAuthorizedRepositories: (id: number) => void
  onSaveSandboxConfig: (apiURL: string, apiKey: string, installationID?: number) => Promise<void>
  onDeleteSandboxAPIKey: (installationID?: number) => Promise<void>
  onNavigate: (page: UserPage) => void
  onNavigateAccountSettings: (accountLogin: string | undefined, tab: AccountSettingsTab) => void
  onSelectKey: (key: string) => void
  onSignOut: () => void
}) {
  const groups = useMemo(() => groupRunnersByBuildContext(runners), [runners])
  const selected = groups.find((group) => group.key === selectedKey) || groups[0]
  const installations = useMemo(
    () => orderInstallationsByCurrentAccount(githubApp?.installations ?? [], authSession.login),
    [authSession.login, githubApp?.installations]
  )
  const hasInstallations = installations.length > 0
  const navItemClass = (active: boolean) =>
    cn(
      "inline-flex h-9 items-center gap-2 rounded-md px-3 text-sm font-medium transition-colors",
      active ? "bg-accent text-accent-foreground" : "text-muted-foreground hover:bg-accent hover:text-accent-foreground"
    )
  const goToPage = (event: MouseEvent<HTMLAnchorElement>, next: UserPage) => {
    event.preventDefault()
    onNavigate(next)
  }

  return (
    <main className="flex min-h-screen flex-col bg-background text-foreground">
      <header className="flex h-14 shrink-0 items-center gap-3 border-b px-4 lg:px-6">
        <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-foreground text-background">
          <Github className="h-4 w-4" />
        </div>
        <div>
          <div className="text-sm font-semibold">Qiniu Runner</div>
        </div>
        <nav className="ml-3 hidden items-center gap-1 md:flex" aria-label="Workspace">
          <a href="/" className={navItemClass(page === "home")} onClick={(event) => goToPage(event, "home")}>
            <Workflow className="h-4 w-4" />
            Jobs
          </a>
          <a
            href="/repositories"
            className={navItemClass(page === "repositories")}
            onClick={(event) => goToPage(event, "repositories")}
          >
            <BookOpen className="h-4 w-4" />
            Repositories
          </a>
        </nav>
        <div className="ml-auto flex items-center gap-2">
          <UserMenu authSession={authSession} onSignOut={onSignOut} />
        </div>
      </header>

      <nav className="flex items-center gap-1 border-b px-4 py-2 md:hidden" aria-label="Workspace">
        <a href="/" className={navItemClass(page === "home")} onClick={(event) => goToPage(event, "home")}>
          <Workflow className="h-4 w-4" />
          Jobs
        </a>
        <a
          href="/repositories"
          className={navItemClass(page === "repositories")}
          onClick={(event) => goToPage(event, "repositories")}
        >
          <BookOpen className="h-4 w-4" />
          Repositories
        </a>
      </nav>

      {page === "repositories" ? (
        <ActivityRepositoriesPage
          installations={installations}
        />
      ) : page === "settings" ? (
        <AccountsPage
          githubApp={githubApp}
          userPreferences={userPreferences}
          installations={installations}
          authorizedRepositories={authorizedRepositories}
          loadingRepositoriesFor={loadingRepositoriesFor}
          route={accountSettingsRoute}
          onLoadAuthorizedRepositories={onLoadAuthorizedRepositories}
          onSaveSandboxConfig={onSaveSandboxConfig}
          onDeleteSandboxAPIKey={onDeleteSandboxAPIKey}
          currentLogin={authSession.login}
          onNavigateAccountSettings={onNavigateAccountSettings}
        />
      ) : (
        <PullRequestsPage
          groups={groups}
          hasInstallations={hasInstallations}
          selected={selected}
          onSelectKey={onSelectKey}
          onNavigate={onNavigate}
        />
      )}
    </main>
  )
}

function ActivityRepositoriesPage({
  installations,
}: {
  installations: NonNullable<GitHubAppConfig["installations"]>
}) {
  const [selectedID, setSelectedID] = useState<number | null>(null)
  const selected = installations.find((installation) => installation.id === selectedID) || installations[0]

  return (
    <>
      <section className="border-b bg-muted/35 px-4 py-4 lg:px-6">
        <div>
          <h1 className="text-xl font-semibold">Repositories</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Local repositories appear here after runnerd observes jobs from them.
          </p>
        </div>
      </section>

      <div className="grid min-h-0 flex-1 lg:grid-cols-[320px_minmax(0,1fr)]">
        <aside className="min-h-0 border-r bg-muted/20">
          <div className="flex h-full flex-col">
            <div className="min-h-0 flex-1 overflow-y-auto">
              {installations.length ? (
                installations.map((installation) => (
                  <button
                    key={installation.id}
                    type="button"
                    className={cn(
                      "flex h-14 w-full items-center gap-3 border-b px-4 text-left transition-colors hover:bg-accent",
                      selected?.id === installation.id ? "bg-accent" : ""
                    )}
                    onClick={() => setSelectedID(installation.id)}
                  >
                    <AccountAvatar installation={installation} size="sm" />
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm font-semibold">{installation.account_login || "GitHub App"}</div>
                    </div>
                  </button>
                ))
              ) : (
                <div className="p-4 text-sm text-muted-foreground">
                  Connect a GitHub App account, then trigger a workflow job to show active repositories here.
                </div>
              )}
            </div>
          </div>
        </aside>

        <section className="min-h-0 overflow-y-auto p-4 lg:p-6">
          {selected ? (
            <div className="space-y-4">
              <div className="flex items-center gap-3">
                <AccountAvatar installation={selected} size="lg" />
                <div className="min-w-0">
                  <h2 className="truncate text-2xl font-semibold">{accountDisplayName(selected)}</h2>
                  <div className="truncate text-sm text-muted-foreground">{selected.account_login || "GitHub"}</div>
                </div>
              </div>

              <Card className="rounded-lg">
                <CardHeader className="gap-3 pb-3">
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div>
                      <CardTitle className="text-base">Active repositories</CardTitle>
                      <CardDescription>Local repositories with observed runner jobs.</CardDescription>
                    </div>
                    <Badge variant="secondary">{selected.repositories.length} repositories</Badge>
                  </div>
                </CardHeader>
                <CardContent>
                  {selected.repositories.length ? (
                    <div className="grid gap-2 md:grid-cols-2 xl:grid-cols-3">
                      {selected.repositories.map((repository) => (
                        <div key={repository} className="rounded-md border bg-muted/25 px-3 py-2">
                          <div className="truncate text-sm font-medium">{repository}</div>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <div className="rounded-md border bg-muted/25 px-3 py-2 text-sm text-muted-foreground">
                      No repositories have runner jobs for this account yet.
                    </div>
                  )}
                </CardContent>
              </Card>
            </div>
          ) : (
            <div className="rounded-lg border bg-muted/30 p-6 text-sm text-muted-foreground">
              Connect a GitHub App account, then trigger a workflow job to show active repositories here.
            </div>
          )}
        </section>
      </div>
    </>
  )
}

function AccountsPage({
  githubApp,
  userPreferences,
  installations,
  authorizedRepositories,
  loadingRepositoriesFor,
  route,
  onLoadAuthorizedRepositories,
  onSaveSandboxConfig,
  onDeleteSandboxAPIKey,
  currentLogin,
  onNavigateAccountSettings,
}: {
  githubApp: GitHubAppConfig | null
  userPreferences: UserPreferences | null
  installations: NonNullable<GitHubAppConfig["installations"]>
  authorizedRepositories: Record<number, string[]>
  loadingRepositoriesFor: number | null
  route: AccountSettingsRoute
  onLoadAuthorizedRepositories: (id: number) => void
  onSaveSandboxConfig: (apiURL: string, apiKey: string, installationID?: number) => Promise<void>
  onDeleteSandboxAPIKey: (installationID?: number) => Promise<void>
  currentLogin?: string
  onNavigateAccountSettings: (accountLogin: string | undefined, tab: AccountSettingsTab) => void
}) {
  const [filter, setFilter] = useState("")
  const selected = installations.find((installation) => installation.account_login === route.accountLogin)
  const authorized = selected ? authorizedRepositories[selected.id] : undefined
  const preferenceInstallationID =
    selected && selected.account_login && selected.account_login !== currentLogin ? selected.id : undefined
  const filteredRepositories = useMemo(() => {
    const query = filter.trim().toLowerCase()
    const repositories = authorized || []
    if (!query) return repositories
    return repositories.filter((repository) => repository.toLowerCase().includes(query))
  }, [authorized, filter])

  useEffect(() => {
    if (!selected) return
    if (!authorizedRepositories[selected.id]) {
      onLoadAuthorizedRepositories(selected.id)
    }
  }, [authorizedRepositories, onLoadAuthorizedRepositories, selected])

  return (
    <>
      <section className="border-b bg-muted/35 px-4 py-4 lg:px-6">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h1 className="text-xl font-semibold">Settings</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Configure repository access and runner preferences for GitHub users and organizations.
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {githubApp?.install_url ? (
              <Button type="button" asChild>
                <a href={githubApp.install_url}>
                  <Github className="h-4 w-4" />
                  Install GitHub App
                </a>
              </Button>
            ) : (
              <Badge variant="outline">Set github.app.slug to enable the install link</Badge>
            )}
          </div>
        </div>
      </section>

      <div className="grid min-h-0 flex-1 lg:grid-cols-[320px_minmax(0,1fr)]">
        <aside className="min-h-0 border-r bg-muted/20">
          <div className="flex h-full flex-col">
            <div className="min-h-0 flex-1 overflow-y-auto">
              {installations.length ? (
                installations.map((installation) => (
                  <button
                    key={installation.id}
                    type="button"
                    className={cn(
                      "flex h-14 w-full items-center gap-3 border-b px-4 text-left transition-colors hover:bg-accent",
                      selected?.id === installation.id ? "bg-accent" : ""
                    )}
                    onClick={() => {
                      onNavigateAccountSettings(installation.account_login, route.tab)
                      setFilter("")
                      if (!authorizedRepositories[installation.id]) onLoadAuthorizedRepositories(installation.id)
                    }}
                  >
                    <AccountAvatar installation={installation} size="sm" />
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm font-semibold">{installation.account_login || "GitHub App"}</div>
                    </div>
                  </button>
                ))
              ) : (
                <div className="p-4 text-sm text-muted-foreground">
                  Install the GitHub App to connect a user or organization.
                </div>
              )}
            </div>
          </div>
        </aside>

        <section className="min-h-0 overflow-y-auto p-4 lg:p-6">
          {selected ? (
            <div className="space-y-4">
              <div className="flex items-center gap-3">
                <AccountAvatar installation={selected} size="lg" />
                <div className="min-w-0">
                  <h2 className="truncate text-2xl font-semibold">{accountDisplayName(selected)}</h2>
                  <div className="truncate text-sm text-muted-foreground">{selected.account_login || "GitHub"}</div>
                </div>
              </div>

              <Tabs
                value={route.tab}
                onValueChange={(value) => onNavigateAccountSettings(selected.account_login, value as AccountSettingsTab)}
                className="gap-4"
              >
                <TabsList className="h-auto w-full justify-start rounded-none border-b bg-transparent p-0">
                  <TabsTrigger
                    value="repositories"
                    className="mr-8 h-10 flex-none rounded-none border-0 border-b-2 border-transparent bg-transparent px-0 py-2 shadow-none data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:text-primary data-[state=active]:shadow-none"
                  >
                    Repositories
                  </TabsTrigger>
                  <TabsTrigger
                    value="preferences"
                    className="h-10 flex-none rounded-none border-0 border-b-2 border-transparent bg-transparent px-0 py-2 shadow-none data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:text-primary data-[state=active]:shadow-none"
                  >
                    Preferences
                  </TabsTrigger>
                </TabsList>

                <TabsContent value="repositories">
                  <Card className="rounded-lg">
                    <CardHeader className="gap-3 pb-3">
                      <div className="flex flex-wrap items-center justify-between gap-3">
                        <div>
                          <CardTitle className="text-base">Authorized repositories</CardTitle>
                          <CardDescription>Repositories currently authorized for this GitHub App installation.</CardDescription>
                        </div>
                        <div className="flex flex-wrap items-center gap-2">
                          <Badge variant="secondary">
                            {authorized ? `${authorized.length} repositories` : "Not loaded"}
                          </Badge>
                          <Button type="button" variant="outline" size="sm" asChild>
                            <a href={`https://github.com/settings/installations/${selected.installation_id}`}>
                              <Github className="h-4 w-4" />
                              Manage on GitHub
                            </a>
                          </Button>
                        </div>
                      </div>
                    </CardHeader>
                    <CardContent className="space-y-3">
                      <input
                        className="h-9 w-full rounded-md border bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        placeholder="Filter GitHub repositories"
                        value={filter}
                        onChange={(event) => setFilter(event.target.value)}
                      />
                      {loadingRepositoriesFor === selected.id ? (
                        <div className="rounded-md border bg-muted/25 px-3 py-2 text-sm text-muted-foreground">
                          Loading repositories from GitHub...
                        </div>
                      ) : filteredRepositories.length ? (
                        <div className="grid gap-2 md:grid-cols-2 xl:grid-cols-3">
                          {filteredRepositories.map((repository) => (
                            <div key={repository} className="rounded-md border bg-muted/25 px-3 py-2">
                              <div className="truncate text-sm font-medium">{repository}</div>
                            </div>
                          ))}
                        </div>
                      ) : (
                        <div className="rounded-md border bg-muted/25 px-3 py-2 text-sm text-muted-foreground">
                          {authorized ? "No repositories match the current filter." : "Select this account to load repositories."}
                        </div>
                      )}
                    </CardContent>
                  </Card>
                </TabsContent>

                <TabsContent value="preferences">
                  <SandboxAPIKeyCard
                    preferences={userPreferences}
                    onSave={(apiURL, apiKey) => onSaveSandboxConfig(apiURL, apiKey, preferenceInstallationID)}
                    onDelete={() => onDeleteSandboxAPIKey(preferenceInstallationID)}
                  />
                </TabsContent>
              </Tabs>
            </div>
          ) : (
            route.tab === "preferences" ? (
              <div className="space-y-4">
                <div>
                  <h2 className="text-2xl font-semibold">Account preferences</h2>
                  <div className="mt-1 text-sm text-muted-foreground">Settings for the signed-in account.</div>
                </div>
                <SandboxAPIKeyCard
                  preferences={userPreferences}
                  onSave={onSaveSandboxConfig}
                  onDelete={onDeleteSandboxAPIKey}
                />
              </div>
            ) : (
              <div className="rounded-lg border bg-muted/30 p-6 text-sm text-muted-foreground">
                Install the GitHub App to connect a user or organization.
              </div>
            )
          )}
        </section>
      </div>
    </>
  )
}

function SandboxAPIKeyCard({
  preferences,
  onSave,
  onDelete,
}: {
  preferences: UserPreferences | null
  onSave: (apiURL: string, apiKey: string) => Promise<void>
  onDelete: () => Promise<void>
}) {
  const [apiURL, setAPIURL] = useState("")
  const [apiKey, setAPIKey] = useState("")
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState("")
  const configured = preferences?.sandbox?.api_key?.configured ?? false
  const updatedAt = preferences?.sandbox?.api_key?.updated_at

  useEffect(() => {
    setAPIURL(preferences?.sandbox?.api_url ?? "")
  }, [preferences?.sandbox?.api_url])

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const nextAPIURL = apiURL.trim()
    const nextAPIKey = apiKey.trim()
    if (!nextAPIURL) {
      setError("Sandbox service API URL is required.")
      return
    }
    if (!configured && !nextAPIKey) {
      setError("Sandbox API Key is required.")
      return
    }
    setSaving(true)
    setError("")
    try {
      await onSave(nextAPIURL, nextAPIKey)
      setAPIKey("")
    } catch (error) {
      setError(error instanceof Error ? error.message : "Failed to save Sandbox service settings.")
    } finally {
      setSaving(false)
    }
  }

  const remove = async () => {
    setDeleting(true)
    setError("")
    try {
      await onDelete()
      setAPIKey("")
    } catch (error) {
      setError(error instanceof Error ? error.message : "Failed to remove Sandbox API Key.")
    } finally {
      setDeleting(false)
    }
  }

  return (
    <Card className="gap-0 rounded-lg py-0">
      <form className="p-3" onSubmit={submit}>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex items-center gap-2 text-base font-semibold leading-none">
              <KeyRound className="h-4 w-4 shrink-0" />
              <span>Sandbox service</span>
            </div>
            <div className="mt-1 text-sm text-muted-foreground">
              Configure the account Sandbox service endpoint and encrypted API Key.
            </div>
          </div>
          <Badge variant={configured ? "secondary" : "outline"}>{configured ? "Configured" : "Not configured"}</Badge>
        </div>

        <div className="mt-2.5 grid gap-2 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] lg:items-end">
          <div className="grid min-w-0 gap-1.5">
            <Label htmlFor="sandbox-api-url">API URL</Label>
            <Input
              id="sandbox-api-url"
              value={apiURL}
              onChange={(event) => setAPIURL(event.target.value)}
              autoComplete="off"
              placeholder="https://api.e2b.dev"
            />
          </div>

          <div className="grid min-w-0 gap-1.5">
            <Label htmlFor="sandbox-api-key">API Key</Label>
            <Input
              id="sandbox-api-key"
              type="password"
              value={apiKey}
              onChange={(event) => setAPIKey(event.target.value)}
              autoComplete="off"
              placeholder={configured ? "••••••••••••••••" : "Enter Sandbox API Key"}
            />
          </div>

          <div className="flex flex-wrap items-center gap-2">
            {configured ? (
              <Button type="button" variant="outline" size="sm" onClick={remove} disabled={deleting || saving}>
                <X className="h-4 w-4" />
                {deleting ? "Removing" : "Remove"}
              </Button>
            ) : null}
            <Button type="submit" size="sm" disabled={saving || deleting || !apiURL.trim() || (!configured && !apiKey.trim())}>
              <ShieldCheck className="h-4 w-4" />
              {saving ? "Saving" : configured ? "Save changes" : "Save settings"}
            </Button>
          </div>
        </div>

        <div className="mt-1.5 text-sm text-muted-foreground">
          {configured && updatedAt ? `Last updated ${formatTime(updatedAt)}` : "No Sandbox API Key is saved."}
        </div>

        {error ? <div className="mt-2 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</div> : null}
      </form>
    </Card>
  )
}

function PullRequestsPage({
  groups,
  hasInstallations,
  selected,
  onSelectKey,
  onNavigate,
}: {
  groups: BuildGroup[]
  hasInstallations: boolean
  selected: BuildGroup | undefined
  onSelectKey: (key: string) => void
  onNavigate: (page: UserPage) => void
}) {
  const currentJobs = selected ? currentBuildJobs(selected) : []
  const previousJobs = selected ? previousBuildJobs(selected, currentJobs) : []
  const selectedTitleClass = selected ? userBuildGroupTitleClass(selected) : ""
  const shouldCollapsePreviousJobs = previousJobs.length > 2

  return (
    <>
      <section className="border-b bg-muted/35 px-4 py-4 lg:px-6">
        <div>
          <h1 className="text-xl font-semibold">Jobs</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Review runner jobs grouped by repository, pull request, or workflow run.
          </p>
        </div>
      </section>

      <div className="grid min-h-0 flex-1 xl:grid-cols-[360px_minmax(0,1fr)]">
        <aside className="min-h-0 border-r bg-muted/20">
          <div className="flex h-full flex-col">
            <div className="min-h-0 flex-1 overflow-y-auto">
              {groups.length ? (
                groups.map((group) => (
                  <BuildGroupListItem
                    key={group.key}
                    group={group}
                    selected={selected?.key === group.key}
                    onSelect={() => onSelectKey(group.key)}
                  />
                ))
              ) : (
                <div className="p-4 text-sm text-muted-foreground">
                  {hasInstallations ? (
                    "No jobs yet. Trigger a workflow in an installed repository, then refresh."
                  ) : (
                    <button
                      type="button"
                      className="text-left text-primary hover:underline"
                      onClick={() => onNavigate("settings")}
                    >
                      Connect a GitHub App account to start tracking jobs.
                    </button>
                  )}
                </div>
              )}
            </div>
          </div>
        </aside>

        <section className="min-h-0 overflow-y-auto p-4 lg:p-6">
          {selected ? (
            <div className="space-y-4">
              <div>
                <h2 className="flex flex-wrap items-baseline gap-x-3 gap-y-1 text-2xl font-semibold">
                  <span className={selectedTitleClass}>{selected.repository}</span>
                  <span className={selectedTitleClass}>{selected.title}</span>
                </h2>
                <div className="mt-3 grid gap-3 text-sm sm:grid-cols-2 xl:grid-cols-4">
                  <JobField label="Branch" value={selected.headBranch || selected.subtitle || "unknown"} />
                  <JobField label="Commit" value={shortSHA(selected.headSHA) || "unknown"} />
                  <JobField label="Workflow runs" value={String(selected.workflowRunIDs.length || 1)} />
                  <JobField label="Last updated" value={formatTime(selected.updatedAt)} />
                </div>
              </div>
              <div className="grid gap-3">
                {currentJobs.map((job) => (
                  <RunnerJobCard key={job.id} job={job} />
                ))}
                {shouldCollapsePreviousJobs ? (
                  <Collapsible>
                    <CollapsibleTrigger asChild>
                      <Button type="button" variant="outline" className="w-full justify-between">
                        Previous jobs
                        <span className="inline-flex items-center gap-2 text-muted-foreground">
                          {previousJobs.length} jobs
                          <ChevronDown className="h-4 w-4" />
                        </span>
                      </Button>
                    </CollapsibleTrigger>
                    <CollapsibleContent className="mt-3 grid gap-3">
                      {previousJobs.map((job) => (
                        <RunnerJobCard key={job.id} job={job} />
                      ))}
                    </CollapsibleContent>
                  </Collapsible>
                ) : previousJobs.length ? (
                  <div className="grid gap-3">
                    <div className="text-sm font-medium text-muted-foreground">Previous jobs</div>
                    {previousJobs.map((job) => (
                      <RunnerJobCard key={job.id} job={job} />
                    ))}
                  </div>
                ) : null}
              </div>
            </div>
          ) : (
            <div className="rounded-lg border bg-muted/30 p-6 text-sm text-muted-foreground">
              {hasInstallations ? (
                "No runner jobs are available yet. Trigger a workflow in an installed repository to see jobs here."
              ) : (
                <button
                  type="button"
                  className="text-left text-primary hover:underline"
                  onClick={() => onNavigate("settings")}
                >
                  Connect a GitHub App account to start tracking jobs.
                </button>
              )}
            </div>
          )}
        </section>
      </div>
    </>
  )
}

function UserMenu({ authSession, onSignOut }: { authSession: AuthSession; onSignOut: () => void }) {
  const { setTheme, theme } = useTheme()
  const avatarURL = userAvatarURL(authSession)
  const login = authSession.login || "github"

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button type="button" variant="ghost" size="icon" className="rounded-full" aria-label="Account menu">
          {avatarURL ? (
            <img
              src={avatarURL}
              alt=""
              className="h-8 w-8 rounded-full border bg-muted object-cover"
              referrerPolicy="no-referrer"
            />
          ) : (
            <span className="flex h-8 w-8 items-center justify-center rounded-full border bg-muted text-xs font-semibold">
              {userInitials(login)}
            </span>
          )}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <DropdownMenuLabel className="truncate">{login}</DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild>
          <a href="/account/repositories">
            <Settings className="h-4 w-4" />
            Settings
          </a>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuLabel className="text-xs font-medium text-muted-foreground">Theme</DropdownMenuLabel>
        <DropdownMenuRadioGroup value={theme || "system"} onValueChange={setTheme}>
          <DropdownMenuRadioItem value="light">
            <Sun className="h-4 w-4" />
            Light
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="dark">
            <Moon className="h-4 w-4" />
            Dark
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="system">
            <Monitor className="h-4 w-4" />
            System
          </DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
        <DropdownMenuSeparator />
        {authSession.role === "admin" ? (
          <>
            <DropdownMenuItem asChild>
              <a href="/admin/">
                <ShieldCheck className="h-4 w-4" />
                Admin
              </a>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
          </>
        ) : null}
        <DropdownMenuItem onClick={onSignOut}>
          <LogOut className="h-4 w-4" />
          Sign out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function JobField({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div>
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 break-words font-medium">{value}</div>
    </div>
  )
}

function RunnerJobCard({ job }: { job: RunnerState }) {
  return (
    <Card className="rounded-lg">
      <CardHeader className="gap-1 pb-0">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle className="text-base">{runnerJobTitle(job)}</CardTitle>
          </div>
          <Badge className={userStatusClass(job.status)}>{job.status}</Badge>
        </div>
      </CardHeader>
      <CardContent className="grid gap-3 pt-0 text-sm md:grid-cols-3">
        <JobField label="Runner spec" value={job.runner_spec_name || "matched by labels"} />
        <JobField label="Workflow" value={job.workflow_name || "unknown"} />
        <JobField label="Workflow run" value={workflowRunValue(job)} />
        <JobField label="Queued" value={formatTime(job.created_at)} />
        <JobField label="Started" value={job.running_at ? formatTime(job.running_at) : "-"} />
        <JobField label="Finished" value={job.completed_at || job.failed_at ? formatTime(job.completed_at || job.failed_at) : "-"} />
      </CardContent>
    </Card>
  )
}

function workflowRunValue(job: RunnerState) {
  const runID = job.workflow_run_id ? String(job.workflow_run_id) : "unknown"
  const jobID = job.workflow_job_id ? String(job.workflow_job_id) : job.id
  const runURL = workflowRunURL(job)
  const runValue = runURL ? (
    <a className="text-primary hover:underline" href={runURL} target="_blank" rel="noreferrer">
      {runID}
    </a>
  ) : (
    runID
  )
  if (!job.github_job_url || !jobID) return runValue
  return (
    <span className="inline-flex items-center gap-1">
      {runValue}
      <span className="text-muted-foreground">/</span>
      <a className="inline-flex items-center gap-1 text-primary hover:underline" href={job.github_job_url} target="_blank" rel="noreferrer">
        {jobID}
        <ExternalLink className="h-3.5 w-3.5" />
      </a>
    </span>
  )
}

function workflowRunURL(job: RunnerState) {
  if (!job.github_job_url || !job.workflow_run_id) return ""
  const marker = `/actions/runs/${job.workflow_run_id}`
  const index = job.github_job_url.indexOf(marker)
  if (index < 0) return ""
  return job.github_job_url.slice(0, index + marker.length)
}

function BuildGroupListItem({
  group,
  selected,
  onSelect,
}: {
  group: BuildGroup
  selected: boolean
  onSelect: () => void
}) {
  const status = buildGroupStatus(group)
  const statusClasses = buildGroupStatusClasses(status)
  const reference = buildGroupReference(group)

  return (
    <button
      type="button"
      onClick={onSelect}
      className={cn(
        "group relative flex w-full gap-2 border-b bg-background/60 py-4 pl-3 pr-4 text-left transition-colors hover:bg-accent/70",
        selected ? "bg-accent" : ""
      )}
    >
      <span className={cn("absolute inset-y-0 left-0 w-1", statusClasses.bar)} aria-hidden="true" />
      <span className={cn("mt-1 flex h-5 w-5 shrink-0 items-center justify-center", statusClasses.icon)}>
        <BuildGroupStatusIcon status={status} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="flex items-start justify-between gap-3">
          <span className="min-w-0 flex-1">
            <span className={cn("block truncate text-sm font-semibold leading-5", statusClasses.title)}>
              {group.repository}
            </span>
          </span>
          <span className="flex shrink-0 items-baseline gap-1 font-mono text-sm leading-5">
            <span className={statusClasses.title}>#</span>
            <span className={cn("text-sm font-semibold", statusClasses.title)}>{reference}</span>
          </span>
        </span>
        <span className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
          <span className="inline-flex items-center gap-1">
            <Workflow className="h-3.5 w-3.5" />
            {group.jobs.length} jobs
          </span>
          <span className="inline-flex items-center gap-1">
            <Play className="h-3.5 w-3.5" />
            {group.workflowRunIDs.length || 1} runs
          </span>
          <span className="inline-flex items-center gap-1">
            <CalendarDays className="h-3.5 w-3.5" />
            {formatTime(group.updatedAt)}
          </span>
        </span>
      </span>
    </button>
  )
}

function userBuildGroupTitleClass(group: BuildGroup) {
  return buildGroupStatusClasses(buildGroupStatus(group)).title
}

function BuildGroupStatusIcon({ status }: { status: RunnerStatusSummary }) {
  const className = "h-4 w-4"
  if (status === "failed") return <X className={className} />
  if (status === "active") return <AlertCircle className={className} />
  return <Check className={className} />
}

function buildGroupStatus(group: BuildGroup): RunnerStatusSummary {
  const jobs = currentBuildJobs(group)
  if (jobs.some((job) => job.status === "failed")) return "failed"
  if (jobs.some((job) => job.status === "queued" || job.status === "creating" || job.status === "running" || job.status === "stopping")) {
    return "active"
  }
  return "completed"
}

function buildGroupStatusClasses(status: RunnerStatusSummary) {
  switch (status) {
    case "failed":
      return {
        bar: "bg-destructive",
        icon: "text-destructive",
        title: "text-destructive",
      }
    case "active":
      return {
        bar: "bg-blue-500",
        icon: "text-blue-500",
        title: "text-blue-500",
      }
    default:
      return {
        bar: "bg-emerald-500",
        icon: "text-emerald-500",
        title: "text-emerald-500",
      }
  }
}

function buildGroupReference(group: BuildGroup) {
  if (group.pullRequestNumber) return String(group.pullRequestNumber)
  if (group.headSHA) return shortSHA(group.headSHA)
  if (group.workflowRunIDs[0]) return String(group.workflowRunIDs[0])
  return String(group.jobs.length)
}

function runnerJobTitle(job: RunnerState) {
  if (job.assigned_job_name && job.assigned_job_name !== "__runner_job_started__") {
    return job.assigned_job_name
  }
  return job.workflow_name || job.runner_name
}

function groupRunnersByBuildContext(runners: RunnerState[]): BuildGroup[] {
  const visibleRunners = runners.filter(isUserVisibleRunnerJob)
  const prByRepositoryAndSHA = new Map<string, number>()
  for (const runner of runners) {
    if (runner.repository_full_name && runner.head_sha && runner.pull_request_number) {
      prByRepositoryAndSHA.set(`${runner.repository_full_name}:${runner.head_sha}`, runner.pull_request_number)
    }
  }

  const groups = new Map<string, BuildGroup>()
  for (const runner of visibleRunners) {
    const repository = runner.repository_full_name || "unknown/repository"
    const inferredPR = runner.pull_request_number || (runner.head_sha ? prByRepositoryAndSHA.get(`${repository}:${runner.head_sha}`) : undefined)
    const group = buildGroupSeed(runner, repository, inferredPR)
    const key = group.key
    const current = groups.get(key)
    if (current) {
      current.jobs.push(runner)
      if (runner.updated_at > current.updatedAt) {
        current.updatedAt = runner.updated_at
        current.subtitle = group.subtitle
        current.headSHA = runner.head_sha || current.headSHA
        current.headBranch = runner.head_branch || current.headBranch
      }
      if (runner.head_sha && !current.headSHA) current.headSHA = runner.head_sha
      if (runner.head_branch && !current.headBranch) current.headBranch = runner.head_branch
      if (runner.workflow_run_id && !current.workflowRunIDs.includes(runner.workflow_run_id)) {
        current.workflowRunIDs.push(runner.workflow_run_id)
        current.workflowRunIDs.sort((a, b) => b - a)
      }
      continue
    }
    groups.set(key, group)
  }
  return Array.from(groups.values())
    .map((group) => ({ ...group, jobs: orderJobs(group.jobs) }))
    .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
}

function isUserVisibleRunnerJob(job: RunnerState) {
  return !(job.failure_stage === "admission" && job.failure_reason === "profile_labels_not_matched")
}

function buildGroupSeed(runner: RunnerState, repository: string, pullRequestNumber?: number): BuildGroup {
  const workflowRunIDs = runner.workflow_run_id ? [runner.workflow_run_id] : []
  if (pullRequestNumber) {
    return {
      key: `pr:${repository}:${pullRequestNumber}`,
      kind: "pull_request",
      repository,
      title: `PR #${pullRequestNumber}`,
      subtitle: runner.head_branch || shortSHA(runner.head_sha) || runner.workflow_name || "Pull request checks",
      updatedAt: runner.updated_at,
      jobs: [runner],
      workflowRunIDs,
      headSHA: runner.head_sha,
      headBranch: runner.head_branch,
      pullRequestNumber,
    }
  }
  if (runner.head_sha) {
    const branch = runner.head_branch || "detached"
    return {
      key: `branch:${repository}:${branch}:${runner.head_sha}`,
      kind: "branch",
      repository,
      title: runner.head_branch || `Commit ${shortSHA(runner.head_sha)}`,
      subtitle: shortSHA(runner.head_sha) || runner.workflow_name || "Branch checks",
      updatedAt: runner.updated_at,
      jobs: [runner],
      workflowRunIDs,
      headSHA: runner.head_sha,
      headBranch: runner.head_branch,
    }
  }
  if (runner.workflow_run_id) {
    return {
      key: `run:${repository}:${runner.workflow_run_id}`,
      kind: "workflow_run",
      repository,
      title: runner.workflow_name || "Workflow run",
      subtitle: `Run ${runner.workflow_run_id}`,
      updatedAt: runner.updated_at,
      jobs: [runner],
      workflowRunIDs,
    }
  }
  return {
    key: `manual:${repository}:${runner.id}`,
    kind: "manual",
    repository,
    title: "Manual runner",
    subtitle: runner.runner_spec_name || runner.runner_name || runner.id,
    updatedAt: runner.updated_at,
    jobs: [runner],
    workflowRunIDs,
  }
}

function orderJobs(jobs: RunnerState[]) {
  return [...jobs].sort((a, b) => {
    const runOrder = (b.workflow_run_id || 0) - (a.workflow_run_id || 0)
    if (runOrder !== 0) return runOrder
    return b.updated_at.localeCompare(a.updated_at)
  })
}

function currentBuildJobs(group: BuildGroup) {
  if (group.headSHA) {
    const current = group.jobs.filter((job) => job.head_sha === group.headSHA)
    if (current.length > 0) return current
  }
  const latestRunID = group.workflowRunIDs[0]
  if (latestRunID) {
    const current = group.jobs.filter((job) => job.workflow_run_id === latestRunID)
    if (current.length > 0) return current
  }
  return group.jobs
}

function previousBuildJobs(group: BuildGroup, currentJobs: RunnerState[]) {
  const currentIDs = new Set(currentJobs.map((job) => job.id))
  return group.jobs.filter((job) => !currentIDs.has(job.id))
}

function shortSHA(value?: string) {
  if (!value) return ""
  return value.length > 7 ? value.slice(0, 7) : value
}

function orderInstallationsByCurrentAccount(
  installations: NonNullable<GitHubAppConfig["installations"]>,
  currentLogin?: string
) {
  const login = (currentLogin || "").toLowerCase()
  return [...installations].sort((a, b) => {
    const aLogin = a.account_login || ""
    const bLogin = b.account_login || ""
    const aIsCurrent = aLogin.toLowerCase() === login
    const bIsCurrent = bLogin.toLowerCase() === login
    if (aIsCurrent !== bIsCurrent) return aIsCurrent ? -1 : 1
    return aLogin.localeCompare(bLogin)
  })
}

function AccountAvatar({
  installation,
  size,
}: {
  installation: NonNullable<GitHubAppConfig["installations"]>[number]
  size: "sm" | "lg"
}) {
  const className =
    size === "lg"
      ? "flex h-12 w-12 shrink-0 items-center justify-center overflow-hidden rounded-md bg-foreground text-sm font-semibold text-background"
      : "flex h-8 w-8 shrink-0 items-center justify-center overflow-hidden rounded-md bg-foreground text-xs font-semibold text-background"
  const label = accountInitials(installation)
  const avatarURL = accountAvatarURL(installation)
  if (avatarURL) {
    return (
      <img
        className={className}
        src={avatarURL}
        alt={`${installation.account_login || "GitHub"} avatar`}
      />
    )
  }
  return <div className={className}>{label}</div>
}

function accountDisplayName(installation: NonNullable<GitHubAppConfig["installations"]>[number]) {
  return installation.account_name || installation.account_login || "GitHub App"
}

function accountInitials(installation: NonNullable<GitHubAppConfig["installations"]>[number]) {
  return accountDisplayName(installation).slice(0, 2).toUpperCase()
}

function accountAvatarURL(installation: NonNullable<GitHubAppConfig["installations"]>[number]) {
  if (installation.account_avatar) return installation.account_avatar
  if (!installation.account_login) return ""
  return `https://github.com/${encodeURIComponent(installation.account_login)}.png?size=96`
}

function userAvatarURL(authSession: AuthSession) {
  if (authSession.avatar_url) return authSession.avatar_url
  if (!authSession.login) return ""
  return `https://github.com/${encodeURIComponent(authSession.login)}.png?size=96`
}

function userInitials(login: string) {
  return (
    login
      .split(/[-_\s]+/)
      .filter(Boolean)
      .slice(0, 2)
      .map((part) => part[0]?.toUpperCase())
      .join("") || "GH"
  )
}

function userStatusClass(status: RunnerState["status"]) {
  switch (status) {
    case "running":
      return "bg-emerald-500 text-white"
    case "queued":
    case "creating":
    case "stopping":
      return "bg-blue-500 text-white"
    case "completed":
      return "bg-muted text-foreground"
    case "failed":
      return "bg-destructive text-white"
    default:
      return ""
  }
}
