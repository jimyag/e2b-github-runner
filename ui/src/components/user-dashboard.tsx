import {
  AlertCircle,
  BookOpen,
  CalendarDays,
  Check,
  ExternalLink,
  Github,
  KeyRound,
  Loader2,
  LogOut,
  Monitor,
  Moon,
  Play,
  Pencil,
  RefreshCw,
  Settings,
  ShieldCheck,
  SquareTerminal,
  Sun,
  Workflow,
  X,
} from "lucide-react"
import { useTheme } from "next-themes"
import { type CSSProperties, type FormEvent, type MouseEvent, type ReactNode, useEffect, useMemo, useRef, useState } from "react"

import type { AuthSession, GitHubAppConfig, RunnerJobGroup, RunnerState, UserPreferences } from "@/admin-types"
import { logNames } from "@/admin-types"
import { formatRunnerDuration, formatTime } from "@/admin-format"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { SandboxesSection, SandboxTemplatesSection } from "@/components/sandbox-catalog-sections"
import { sandboxRegions } from "@/components/sandbox-catalog-utils"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useSandboxTerminal } from "@/hooks/use-sandbox-terminal"
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
type AccountSettingsTab = "repositories" | "preferences" | "sandbox-templates" | "sandbox-instances"
type AccountSettingsRoute = {
  accountLogin?: string
  tab: AccountSettingsTab
}

type GitHubLogState =
  | { kind: "log"; text: string }
  | { kind: "unavailable"; detail: string }

const jobLogTabsListClassName = "h-auto w-full justify-start gap-6 rounded-none border-b bg-transparent p-0 text-muted-foreground"
const jobLogTabsTriggerClassName = "h-10 flex-none rounded-none border-x-0 border-t-0 border-b-2 border-transparent bg-transparent px-0 py-2 text-sm font-medium shadow-none hover:text-foreground data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:text-foreground data-[state=active]:shadow-none dark:data-[state=active]:bg-transparent"

function normalizeSandboxAPIURL(value: string) {
  return value.trim().replace(/\/+$/, "")
}

function findSandboxRegionByAPIURL(value: string) {
  const normalized = normalizeSandboxAPIURL(value)
  return sandboxRegions.find((region) => normalizeSandboxAPIURL(region.apiURL) === normalized)
}

function resolveSandboxRegionAPIURL(value: string) {
  const matchedRegion = findSandboxRegionByAPIURL(value)
  return matchedRegion?.apiURL ?? value
}

export function UserDashboard({
  authSession,
  githubApp,
  userPreferences,
  runners,
  selectedKey,
  selectedJobID,
  page,
  accountSettingsRoute,
  authorizedRepositories,
  loadingRepositoriesFor,
  syncingGitHubInstallations,
  onLoadAuthorizedRepositories,
  onSyncGitHubInstallations,
  onSaveSandboxConfig,
  onDeleteSandboxAPIKey,
  onNavigate,
  onNavigateAccountSettings,
  onOpenJob,
  onLoadJobGroup,
  request,
  onSelectKey,
  onSignOut,
}: {
  authSession: AuthSession
  githubApp: GitHubAppConfig | null
  userPreferences: UserPreferences | null
  runners: RunnerState[]
  selectedKey: string
  selectedJobID: string
  page: UserPage
  accountSettingsRoute: AccountSettingsRoute
  authorizedRepositories: Record<number, string[]>
  loadingRepositoriesFor: number | null
  syncingGitHubInstallations: boolean
  onLoadAuthorizedRepositories: (id: number) => void
  onSyncGitHubInstallations: () => void
  onSaveSandboxConfig: (apiURL: string, apiKey: string, installationID?: number, mode?: "custom" | "inherit", replaceInheritedSource?: boolean) => Promise<void>
  onDeleteSandboxAPIKey: (installationID?: number) => Promise<void>
  onNavigate: (page: UserPage) => void
  onNavigateAccountSettings: (accountLogin: string | undefined, tab: AccountSettingsTab) => void
  onOpenJob: (id: string) => void
  onLoadJobGroup: (key: string) => Promise<unknown>
  request: (url: string, options?: RequestInit) => Promise<unknown>
  onSelectKey: (key: string) => void
  onSignOut: () => void
}) {
  const groups = useMemo(() => groupRunnersByBuildContext(runners), [runners])
  const selected = groups.find((group) => group.key === selectedKey) || (selectedKey ? undefined : groups[0])
  const [loadedJobGroup, setLoadedJobGroup] = useState<{ key: string; group: RunnerJobGroup } | null>(null)
  const selectedJobGroup = loadedJobGroup && selected && loadedJobGroup.key === selected.key ? loadedJobGroup.group : null
  const installations = useMemo(
    () => orderInstallationsByCurrentAccount(githubApp?.installations ?? [], authSession.login),
    [authSession.login, githubApp?.installations]
  )
  const canSyncGitHubInstallations = Boolean(githubApp?.install_url || githubApp?.app_slug)
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

  useEffect(() => {
    let cancelled = false
    if (!selected?.key) return
    void onLoadJobGroup(selected.key)
      .then((group) => {
        if (!cancelled) {
          setLoadedJobGroup(isRunnerJobGroup(group) ? { key: selected.key, group } : null)
        }
      })
      .catch(() => {
        if (!cancelled) setLoadedJobGroup(null)
      })
    return () => {
      cancelled = true
    }
  }, [onLoadJobGroup, selected?.jobs.length, selected?.key, selected?.updatedAt])

  return (
    <main className="flex min-h-screen flex-col bg-background text-foreground">
      <header className="flex h-14 shrink-0 items-center gap-3 border-b px-4 lg:px-6">
        <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-foreground text-background">
          <Play className="h-5 w-5" />
        </div>
        <div>
          <div className="text-sm font-semibold tracking-wide">Qiniu Runner</div>
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
          canSyncGitHubInstallations={canSyncGitHubInstallations}
          syncingGitHubInstallations={syncingGitHubInstallations}
          onSyncGitHubInstallations={onSyncGitHubInstallations}
        />
      ) : page === "settings" ? (
        <AccountsPage
          githubApp={githubApp}
          userPreferences={userPreferences}
          installations={installations}
          authorizedRepositories={authorizedRepositories}
          loadingRepositoriesFor={loadingRepositoriesFor}
          route={accountSettingsRoute}
          syncingGitHubInstallations={syncingGitHubInstallations}
          onLoadAuthorizedRepositories={onLoadAuthorizedRepositories}
          onSyncGitHubInstallations={onSyncGitHubInstallations}
          onSaveSandboxConfig={onSaveSandboxConfig}
          onDeleteSandboxAPIKey={onDeleteSandboxAPIKey}
          currentLogin={authSession.login}
          onNavigateAccountSettings={onNavigateAccountSettings}
          request={request}
        />
      ) : (
        <PullRequestsPage
          groups={groups}
          hasInstallations={hasInstallations}
          selected={selected}
          selectedJobGroup={selectedJobGroup}
          selectedJobID={selectedJobID}
          onSelectKey={onSelectKey}
          canSyncGitHubInstallations={canSyncGitHubInstallations}
          syncingGitHubInstallations={syncingGitHubInstallations}
          onSyncGitHubInstallations={onSyncGitHubInstallations}
          onOpenJob={onOpenJob}
          request={request}
        />
      )}
    </main>
  )
}

function ActivityRepositoriesPage({
  installations,
  canSyncGitHubInstallations,
  syncingGitHubInstallations,
  onSyncGitHubInstallations,
}: {
  installations: NonNullable<GitHubAppConfig["installations"]>
  canSyncGitHubInstallations: boolean
  syncingGitHubInstallations: boolean
  onSyncGitHubInstallations: () => void
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
                  Sync existing GitHub App accounts, then trigger a workflow job to show active repositories here.
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
            <div className="rounded-lg border bg-muted/30 p-6">
              <div className="flex flex-wrap items-center justify-between gap-4">
                <div className="min-w-0">
                  <h2 className="text-base font-semibold">Sync existing GitHub App accounts</h2>
                  <p className="mt-1 text-sm text-muted-foreground">
                    {canSyncGitHubInstallations
                      ? "Use this if the GitHub App is already installed but this runnerd instance has no local account record yet."
                      : "Set up GitHub App auth before syncing local account records."}
                  </p>
                </div>
                {canSyncGitHubInstallations ? (
                  <SyncGitHubInstallationsButton
                    isSyncing={syncingGitHubInstallations}
                    label="Sync accounts"
                    loadingLabel="Syncing..."
                    onSync={onSyncGitHubInstallations}
                  />
                ) : null}
              </div>
            </div>
          )}
        </section>
      </div>
    </>
  )
}

function SyncGitHubInstallationsButton({
  isSyncing,
  label,
  loadingLabel,
  onSync,
  variant,
}: {
  isSyncing: boolean
  label: string
  loadingLabel: string
  onSync: () => void
  variant?: "default" | "outline"
}) {
  return (
    <Button
      type="button"
      variant={variant}
      disabled={isSyncing}
      className={cn(label.length > 16 ? "min-w-[13.5rem]" : "min-w-[8.5rem]")}
      onClick={onSync}
    >
      {isSyncing ? (
        <Loader2 className="h-4 w-4 animate-spin" />
      ) : (
        <Github className="h-4 w-4" />
      )}
      {isSyncing ? loadingLabel : label}
    </Button>
  )
}

function AccountsPage({
  githubApp,
  userPreferences,
  installations,
  authorizedRepositories,
  loadingRepositoriesFor,
  route,
  syncingGitHubInstallations,
  onLoadAuthorizedRepositories,
  onSyncGitHubInstallations,
  onSaveSandboxConfig,
  onDeleteSandboxAPIKey,
  currentLogin,
  onNavigateAccountSettings,
  request,
}: {
  githubApp: GitHubAppConfig | null
  userPreferences: UserPreferences | null
  installations: NonNullable<GitHubAppConfig["installations"]>
  authorizedRepositories: Record<number, string[]>
  loadingRepositoriesFor: number | null
  route: AccountSettingsRoute
  syncingGitHubInstallations: boolean
  onLoadAuthorizedRepositories: (id: number) => void
  onSyncGitHubInstallations: () => void
  onSaveSandboxConfig: (apiURL: string, apiKey: string, installationID?: number, mode?: "custom" | "inherit", replaceInheritedSource?: boolean) => Promise<void>
  onDeleteSandboxAPIKey: (installationID?: number) => Promise<void>
  currentLogin?: string
  onNavigateAccountSettings: (accountLogin: string | undefined, tab: AccountSettingsTab) => void
  request: (url: string, options?: RequestInit) => Promise<unknown>
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
              <>
                <SyncGitHubInstallationsButton
                  isSyncing={syncingGitHubInstallations}
                  label="Sync existing installations"
                  loadingLabel="Syncing installations..."
                  onSync={onSyncGitHubInstallations}
                  variant="outline"
                />
                <Button type="button" asChild>
                  <a href={githubApp.install_url}>
                    <Github className="h-4 w-4" />
                    Install GitHub App
                  </a>
                </Button>
              </>
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
                  Install the GitHub App or sync existing installations to link a user or organization.
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
                  <TabsTrigger
                    value="sandbox-templates"
                    className="ml-8 h-10 flex-none rounded-none border-0 border-b-2 border-transparent bg-transparent px-0 py-2 shadow-none data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:text-primary data-[state=active]:shadow-none"
                  >
                    Sandbox Templates
                  </TabsTrigger>
                  <TabsTrigger
                    value="sandbox-instances"
                    className="ml-8 h-10 flex-none rounded-none border-0 border-b-2 border-transparent bg-transparent px-0 py-2 shadow-none data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:text-primary data-[state=active]:shadow-none"
                  >
                    Sandbox Instances
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
                    allowInheritance={Boolean(preferenceInstallationID)}
                    onSave={(apiURL, apiKey, mode, replaceInheritedSource) => onSaveSandboxConfig(apiURL, apiKey, preferenceInstallationID, mode, replaceInheritedSource)}
                    onDelete={() => onDeleteSandboxAPIKey(preferenceInstallationID)}
                  />
                </TabsContent>
                <TabsContent value="sandbox-templates">
                  <SandboxTemplatesSection
                    key={`templates-${preferenceInstallationID ?? "account"}`}
                    request={request}
                    installationID={preferenceInstallationID}
                  />
                </TabsContent>
                <TabsContent value="sandbox-instances">
                  <SandboxesSection
                    key={`instances-${preferenceInstallationID ?? "account"}`}
                    request={request}
                    installationID={preferenceInstallationID}
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
                  onSave={(apiURL, apiKey, mode) => onSaveSandboxConfig(apiURL, apiKey, undefined, mode)}
                  onDelete={onDeleteSandboxAPIKey}
                />
              </div>
            ) : route.tab === "sandbox-templates" ? (
              <div className="space-y-4">
                <div>
                  <h2 className="text-2xl font-semibold">Sandbox templates</h2>
                  <div className="mt-1 text-sm text-muted-foreground">Templates available to the signed-in account.</div>
                </div>
                <SandboxTemplatesSection request={request} />
              </div>
            ) : route.tab === "sandbox-instances" ? (
              <div className="space-y-4">
                <div>
                  <h2 className="text-2xl font-semibold">Sandbox instances</h2>
                  <div className="mt-1 text-sm text-muted-foreground">Runner-created instances for the signed-in account.</div>
                </div>
                <SandboxesSection request={request} />
              </div>
            ) : (
              <div className="rounded-lg border bg-muted/30 p-6">
                <div className="flex flex-wrap items-center justify-between gap-4">
                  <div className="min-w-0">
                    <h2 className="text-base font-semibold">No local GitHub App accounts</h2>
                    <p className="mt-1 text-sm text-muted-foreground">
                      {githubApp
                        ? "Sync existing GitHub App installations to create the local account links for this runnerd instance."
                        : "Set up GitHub App auth before syncing local account records."}
                    </p>
                  </div>
                  {githubApp?.install_url || githubApp?.app_slug ? (
                    <SyncGitHubInstallationsButton
                      isSyncing={syncingGitHubInstallations}
                      label="Sync existing installations"
                      loadingLabel="Syncing installations..."
                      onSync={onSyncGitHubInstallations}
                    />
                  ) : null}
                </div>
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
  allowInheritance = false,
  onSave,
  onDelete,
}: {
  preferences: UserPreferences | null
  allowInheritance?: boolean
  onSave: (apiURL: string, apiKey: string, mode?: "custom" | "inherit", replaceInheritedSource?: boolean) => Promise<void>
  onDelete: () => Promise<void>
}) {
  const [apiURL, setAPIURL] = useState("")
  const [apiKey, setAPIKey] = useState("")
  const [credentialMode, setCredentialMode] = useState<"custom" | "inherit">("custom")
  const [customAPIURLOpen, setCustomAPIURLOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [removeConfirmOpen, setRemoveConfirmOpen] = useState(false)
  const [replaceSourceConfirmOpen, setReplaceSourceConfirmOpen] = useState(false)
  const [error, setError] = useState("")
  const configured = preferences?.sandbox?.api_key?.configured ?? false
  const customConfigured = configured && !preferences?.sandbox?.inherited
  const inherited = credentialMode === "inherit" && Boolean(preferences?.sandbox?.inherited)
  const sourceIsCurrentAccount = Boolean(preferences?.sandbox?.source_is_current_account)
  const sourceAccountLogin = preferences?.sandbox?.source_account_login?.trim()
  const sourceAvailable = Boolean(preferences?.sandbox?.source_available)
  const updatedAt = preferences?.sandbox?.api_key?.updated_at
  const savedAPIURL = preferences?.sandbox?.api_url ?? ""
  const canUseSavedAPIURL = !customAPIURLOpen && !(allowInheritance && preferences?.sandbox?.inherited && credentialMode === "custom")
  const effectiveAPIURL = apiURL || (canUseSavedAPIURL ? savedAPIURL : "")
  const selectedRegion = findSandboxRegionByAPIURL(effectiveAPIURL)
  const showsCustomAPIURL = customAPIURLOpen || (Boolean(effectiveAPIURL.trim()) && !selectedRegion)

  useEffect(() => {
    const resolvedAPIURL = resolveSandboxRegionAPIURL(savedAPIURL)
    setAPIURL(resolvedAPIURL)
    setCustomAPIURLOpen(Boolean(savedAPIURL.trim()) && !findSandboxRegionByAPIURL(resolvedAPIURL))
  }, [savedAPIURL])

  useEffect(() => {
    setCredentialMode(allowInheritance && preferences?.sandbox?.mode === "inherit" ? "inherit" : "custom")
  }, [allowInheritance, preferences?.sandbox?.mode])

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const nextAPIURL = effectiveAPIURL.trim()
    const nextAPIKey = apiKey.trim()
    if (credentialMode === "inherit") {
      setSaving(true)
      setError("")
      try {
        await onSave("", "", "inherit")
        setAPIKey("")
      } catch (error) {
        setError(error instanceof Error ? error.message : "Failed to save Sandbox service settings.")
      } finally {
        setSaving(false)
      }
      return
    }
    if (!nextAPIURL) {
      setError("Sandbox service region is required.")
      return
    }
    if (!customConfigured && !nextAPIKey) {
      setError("Sandbox API Key is required.")
      return
    }
    setSaving(true)
    setError("")
    try {
      await onSave(nextAPIURL, nextAPIKey, "custom")
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
      setRemoveConfirmOpen(false)
    } catch (error) {
      setError(error instanceof Error ? error.message : "Failed to remove Sandbox API Key.")
    } finally {
      setDeleting(false)
    }
  }

  const replaceInheritedSource = async () => {
    setSaving(true)
    setError("")
    try {
      await onSave("", "", "inherit", true)
      setAPIKey("")
      setReplaceSourceConfirmOpen(false)
    } catch (error) {
      setError(error instanceof Error ? error.message : "Failed to use your account credentials.")
    } finally {
      setSaving(false)
    }
  }

  return (
    <Card className="rounded-lg">
      <form onSubmit={submit}>
        <CardHeader className="gap-3 pb-3">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0">
              <CardTitle className="flex items-center gap-2 text-base">
                <KeyRound className="h-4 w-4 shrink-0" />
                <span>Sandbox service</span>
              </CardTitle>
              <CardDescription className="mt-1">
                Configure the account Sandbox service endpoint and encrypted API Key.
              </CardDescription>
            </div>
            <Badge variant={configured ? "success" : "outline"}>{configured ? "Configured" : "Not configured"}</Badge>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {allowInheritance ? (
            <div className="flex flex-wrap items-center gap-3 rounded-md border bg-muted/20 px-3 py-3">
              <Switch
                id="sandbox-use-account-default"
                checked={credentialMode === "inherit"}
                onCheckedChange={(checked) => {
                  setCredentialMode(checked ? "inherit" : "custom")
                  if (!checked && preferences?.sandbox?.inherited) {
                    setAPIURL("")
                    setAPIKey("")
                  }
                  setError("")
                }}
                disabled={saving || deleting}
              />
              <Label htmlFor="sandbox-use-account-default" className="cursor-pointer text-sm font-medium">
                Use account default credentials
              </Label>
              <span className="text-sm text-muted-foreground">
                {credentialMode !== "inherit"
                  ? "This organization uses its own Sandbox service settings."
                  : !preferences?.sandbox?.inherited
                    ? "Your account default credentials will be used after saving."
                  : !sourceAvailable
                    ? sourceAccountLogin
                      ? `Credentials provided by @${sourceAccountLogin} are unavailable because that account is no longer connected to this organization.`
                      : "The inherited credentials are unavailable because the source account is no longer connected to this organization."
                  : sourceIsCurrentAccount
                    ? "Using Sandbox credentials provided by your account."
                    : sourceAccountLogin
                      ? `Using Sandbox credentials provided by @${sourceAccountLogin}.`
                      : "Using Sandbox credentials provided by another connected owner."}
              </span>
            </div>
          ) : null}

          <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] xl:items-end">
            <div className="grid min-w-0 gap-2">
              <Label htmlFor="sandbox-api-region">Region</Label>
              <Select
                value={selectedRegion?.id ?? ""}
                onValueChange={(regionID) => {
                  const region = sandboxRegions.find((region) => region.id === regionID)
                  setAPIURL(region?.apiURL ?? "")
                  setCustomAPIURLOpen(false)
                }}
                disabled={saving || deleting || credentialMode === "inherit"}
              >
                <SelectTrigger id="sandbox-api-region" className="w-full">
                  {selectedRegion ? (
                    <span className="truncate">{selectedRegion.label}</span>
                  ) : (
                    <SelectValue placeholder="Select Sandbox region" />
                  )}
                </SelectTrigger>
                <SelectContent>
                  {sandboxRegions.map((region) => (
                    <SelectItem key={region.id} value={region.id} textValue={region.label}>
                      <span>{region.label}</span>
                      <span className="text-muted-foreground">{region.id}</span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {!showsCustomAPIURL && credentialMode === "custom" ? (
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => {
                    setAPIURL("")
                    setCustomAPIURLOpen(true)
                  }}
                  disabled={saving || deleting}
                >
                  <Pencil className="h-4 w-4" />
                  Custom endpoint
                </Button>
              ) : null}
              {showsCustomAPIURL && credentialMode === "custom" ? (
                <Input
                  value={apiURL}
                  onChange={(event) => setAPIURL(event.target.value)}
                  placeholder="https://sandbox.example.test"
                  disabled={saving || deleting}
                  autoComplete="off"
                />
              ) : null}
            </div>

            <div className="grid min-w-0 gap-2">
              <Label htmlFor="sandbox-api-key">API Key</Label>
              <Input
                id="sandbox-api-key"
                type="password"
                value={apiKey}
                onChange={(event) => setAPIKey(event.target.value)}
                autoComplete="off"
                disabled={saving || deleting || credentialMode === "inherit"}
                placeholder={customConfigured ? "Enter a new API Key to replace the saved one" : "Enter Sandbox API Key"}
              />
            </div>

            <div className="flex flex-wrap items-center gap-2">
              {!inherited ? (
                <Button type="submit" disabled={saving || deleting || (credentialMode === "custom" && (!effectiveAPIURL.trim() || (!customConfigured && !apiKey.trim())))}>
                  <ShieldCheck className="h-4 w-4" />
                  {saving ? "Saving" : configured ? "Save changes" : "Save settings"}
                </Button>
              ) : null}
              {inherited && !sourceIsCurrentAccount ? (
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => {
                    setError("")
                    setReplaceSourceConfirmOpen(true)
                  }}
                  disabled={saving || deleting}
                >
                  <KeyRound className="h-4 w-4" />
                  Use my account credentials
                </Button>
              ) : null}
              {customConfigured && credentialMode === "custom" ? (
                <Button type="button" variant="outline" onClick={() => setRemoveConfirmOpen(true)} disabled={deleting || saving}>
                  <X className="h-4 w-4" />
                  {deleting ? "Removing" : "Remove"}
                </Button>
              ) : null}
            </div>
          </div>

          <div className="text-sm text-muted-foreground">
            {configured && updatedAt ? `Last updated ${formatTime(updatedAt)}` : "No Sandbox API Key is saved."}
          </div>

          {error ? <div className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</div> : null}
        </CardContent>
      </form>
      <Dialog open={removeConfirmOpen} onOpenChange={(open) => !deleting && setRemoveConfirmOpen(open)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Remove Sandbox API Key?</DialogTitle>
            <DialogDescription>
              This removes the saved Sandbox API Key for this account. Runner jobs cannot start new Sandbox instances until a key is saved again.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline" disabled={deleting}>
                Cancel
              </Button>
            </DialogClose>
            <Button type="button" variant="destructive" onClick={remove} disabled={deleting}>
              <X className="h-4 w-4" />
              {deleting ? "Removing" : "Remove API Key"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={replaceSourceConfirmOpen} onOpenChange={(open) => !saving && setReplaceSourceConfirmOpen(open)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Use your account credentials?</DialogTitle>
            <DialogDescription>
              {sourceAccountLogin ? `This replaces the Sandbox credentials provided by @${sourceAccountLogin}. ` : "This replaces the current Sandbox credentials. "}
              The change applies to all Runner jobs in this organization, and your account must have a complete Sandbox service configuration.
            </DialogDescription>
          </DialogHeader>
          {error ? <div className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</div> : null}
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline" disabled={saving}>
                Cancel
              </Button>
            </DialogClose>
            <Button type="button" onClick={replaceInheritedSource} disabled={saving}>
              <KeyRound className="h-4 w-4" />
              {saving ? "Switching" : "Use my credentials"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}

function PullRequestsPage({
  groups,
  hasInstallations,
  selected,
  selectedJobGroup,
  selectedJobID,
  onSelectKey,
  canSyncGitHubInstallations,
  syncingGitHubInstallations,
  onSyncGitHubInstallations,
  onOpenJob,
  request,
}: {
  groups: BuildGroup[]
  hasInstallations: boolean
  selected: BuildGroup | undefined
  selectedJobGroup: RunnerJobGroup | null
  selectedJobID: string
  onSelectKey: (key: string) => void
  canSyncGitHubInstallations: boolean
  syncingGitHubInstallations: boolean
  onSyncGitHubInstallations: () => void
  onOpenJob: (id: string) => void
  request: (url: string, options?: RequestInit) => Promise<unknown>
}) {
  const currentJobs = selectedJobGroup?.current_jobs || (selected ? currentBuildJobs(selected) : [])
  const previousJobs = selectedJobGroup?.previous_jobs || (selected ? previousBuildJobs(selected, currentJobs) : [])
  const allJobs = [...currentJobs, ...previousJobs]
  const selectedJob = allJobs.find((job) => job.id === selectedJobID) || allJobs[0] || null
  const effectiveSelectedJobID = selectedJob?.id || ""
  const workflows = workflowGroups(allJobs)
  const selectedStatus = selected ? buildGroupStatus(selected) : null

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
                groups.map((group) => {
                  const isSelected = selected?.key === group.key
                  const showSubmenu = isSelected && allJobs.length > 1
                  return (
                    <div key={group.key} className="border-b">
                      <BuildGroupListItem
                        group={group}
                        selected={isSelected}
                        onSelect={() => onSelectKey(group.key)}
                      />
                      {showSubmenu ? (
                        <div className="border-t border-border/40 bg-background/70 py-1">
                          <WorkflowJobExplorer
                            workflows={workflows}
                            selectedJobID={effectiveSelectedJobID}
                            onOpenJob={onOpenJob}
                          />
                        </div>
                      ) : null}
                    </div>
                  )
                })
              ) : (
                <div className="p-4 text-sm text-muted-foreground">
                  {hasInstallations ? (
                    "No jobs yet. Trigger a workflow in an installed repository, then refresh."
                  ) : (
                    "Sync existing GitHub App accounts to start tracking jobs."
                  )}
                </div>
              )}
            </div>
          </div>
        </aside>

        <section className="min-h-0 overflow-y-auto">
          {selected ? (
            <div className="flex min-h-full flex-col">
              <div className="border-b px-4 py-4 lg:px-6">
                <h2 className="truncate text-2xl font-semibold">{selected.repository}</h2>
              </div>
              <div className="flex min-h-0 flex-1 flex-col gap-4 p-4 lg:p-6">
                <div className="shrink-0 border-b pb-4">
                  <h3 className="flex flex-wrap items-center gap-x-3 gap-y-1 text-2xl font-semibold">
                    <span>{pullRequestHeading(selected, selectedJobGroup)}</span>
                    {selectedStatus ? <BuildGroupStatusBadge group={selected} status={selectedStatus} /> : null}
                  </h3>
                  <div className="mt-3 space-y-4 text-sm">
                    <section>
                      <div className="grid gap-3 sm:grid-cols-3">
                        <JobField label="Branch" value={selectedJobGroup?.head_branch || selected.headBranch || selected.subtitle || "unknown"} />
                        <JobField label="Commit" value={shortSHA(selectedJobGroup?.head_sha || selected.headSHA) || "unknown"} />
                        <JobField label="Last updated" value={formatTime(selectedJobGroup?.updated_at || selected.updatedAt)} />
                      </div>
                    </section>
                    <section>
                      <div className="grid gap-3 sm:grid-cols-3">
                        {selectedJob ? (
                          <>
                            <JobField
                              label="Job Name"
                              value={selectedJob.github_job_url ? (
                                <a className={cn("inline-flex max-w-full min-w-0 items-center gap-1 hover:underline", jobStatusTextClass(selectedJob.status))} href={selectedJob.github_job_url} target="_blank" rel="noreferrer">
                                  <span className="truncate">{runnerJobTitle(selectedJob)}</span>
                                  <ExternalLink className="h-3.5 w-3.5 shrink-0" />
                                </a>
                              ) : (
                                <span className={cn("block truncate", jobStatusTextClass(selectedJob.status))}>{runnerJobTitle(selectedJob)}</span>
                              )}
                            />
                            <JobField
                              label="Workflow"
                              value={workflowRunURL(selectedJob) ? (
                                <a className="inline-flex max-w-full min-w-0 items-center gap-1 text-primary hover:underline" href={workflowRunURL(selectedJob)} target="_blank" rel="noreferrer">
                                  <span className="truncate">{selectedJob.workflow_name || "Workflow"}</span>
                                  <ExternalLink className="h-3.5 w-3.5 shrink-0" />
                                </a>
                              ) : (
                                <span className="block truncate">{selectedJob.workflow_name || "Workflow"}</span>
                              )}
                            />
                            <JobField label="Duration" value={formatRunnerDuration(selectedJob) || "-"} />
                          </>
                        ) : (
                          <div className="text-muted-foreground">Select a job to inspect its logs.</div>
                        )}
                      </div>
                    </section>
                  </div>
                </div>
                {selectedJob ? <RunnerJobLogPanel job={selectedJob} request={request} /> : (
                  <div className="rounded-lg border bg-muted/30 p-6 text-sm text-muted-foreground">
                    Select a job to inspect its logs.
                  </div>
                )}
              </div>
            </div>
          ) : (
            <div className="p-4 lg:p-6">
              {groups.length ? (
                <div className="rounded-lg border bg-muted/30 p-6 text-sm text-muted-foreground">
                  This job group was not found. It may have aged out of the local runner history or belongs to an account that is not connected.
                </div>
              ) : hasInstallations ? (
                <div className="rounded-lg border bg-muted/30 p-6 text-sm text-muted-foreground">
                  No runner jobs are available yet. Trigger a workflow in an installed repository to see jobs here.
                </div>
              ) : (
                <div className="rounded-lg border bg-muted/30 p-6">
                  <div className="flex flex-wrap items-center justify-between gap-4">
                    <div className="min-w-0">
                      <h2 className="text-base font-semibold">Sync existing GitHub App accounts</h2>
                      <p className="mt-1 text-sm text-muted-foreground">
                        {canSyncGitHubInstallations
                          ? "Use this if the GitHub App is already installed but this runnerd instance has no local account record yet."
                          : "Set up GitHub App auth before syncing local account records."}
                      </p>
                    </div>
                    {canSyncGitHubInstallations ? (
                      <SyncGitHubInstallationsButton
                        isSyncing={syncingGitHubInstallations}
                        label="Sync accounts"
                        loadingLabel="Syncing..."
                        onSync={onSyncGitHubInstallations}
                      />
                    ) : null}
                  </div>
                </div>
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

function JobField({ label, value, onOpen }: { label: string; value: ReactNode; onOpen?: () => void }) {
  return (
    <div className="grid grid-cols-[88px_minmax(0,1fr)] items-baseline gap-2">
      <div className="text-xs text-muted-foreground">{label}</div>
      {onOpen ? (
        <button type="button" className="min-w-0 break-words text-left font-medium hover:text-primary hover:underline" onClick={onOpen}>
          {value}
        </button>
      ) : (
        <div className="min-w-0 break-words font-medium">{value}</div>
      )}
    </div>
  )
}

function WorkflowJobExplorer({
  workflows,
  selectedJobID,
  onOpenJob,
}: {
  workflows: ReturnType<typeof workflowGroups>
  selectedJobID: string
  onOpenJob: (id: string) => void
}) {
  return (
    <div className="grid gap-0">
      {workflows.map((workflow) => (
        workflow.jobs.length === 1 ? (
          <WorkflowRunListItem
            key={workflow.id}
            workflow={workflow}
            selectedJobID={selectedJobID}
            onOpenJob={onOpenJob}
          />
        ) : (
          <section key={workflow.id} className="grid gap-0">
            <div className="grid gap-0">
              {workflow.jobs.map((job) => (
                <RunnerJobListItem key={job.id} job={job} selected={job.id === selectedJobID} onOpen={() => onOpenJob(job.id)} />
              ))}
            </div>
          </section>
        )
      ))}
    </div>
  )
}

function WorkflowRunListItem({
  workflow,
  selectedJobID,
  onOpenJob,
}: {
  workflow: ReturnType<typeof workflowGroups>[number]
  selectedJobID: string
  onOpenJob: (id: string) => void
}) {
  const job = workflow.jobs[0]
  const selected = job.id === selectedJobID
  const status = workflowStatus(workflow.jobs)

  return (
    <button
      type="button"
      onClick={() => onOpenJob(job.id)}
      className={cn(
        "grid w-full grid-cols-[32px_minmax(0,1fr)_auto] items-center gap-2 px-4 py-1.5 text-left text-sm transition-colors",
        selected ? "bg-primary/10 text-primary shadow-[inset_3px_0_0_hsl(var(--primary))]" : "hover:bg-muted/80"
      )}
    >
      <span className={cn("flex justify-center", buildGroupStatusClasses(status).icon)}>{jobStatusMark(job.status)}</span>
      <span className="min-w-0">
        <span className="block truncate font-medium">{workflow.name}</span>
      </span>
      <span className="shrink-0 text-xs text-muted-foreground">{formatRunnerDuration(job)}</span>
    </button>
  )
}

function RunnerJobListItem({ job, selected, onOpen }: { job: RunnerState; selected: boolean; onOpen: () => void }) {
  return (
    <button
      type="button"
      onClick={onOpen}
      className={cn(
        "grid w-full grid-cols-[32px_minmax(0,1fr)_auto] items-center gap-2 px-4 py-1.5 text-left text-sm transition-colors",
        selected ? "bg-primary/10 text-primary shadow-[inset_3px_0_0_hsl(var(--primary))]" : "hover:bg-muted/80"
      )}
    >
      <span className={cn("flex justify-center", buildGroupStatusClasses(jobStatusSummary(job.status)).icon)}>{jobStatusMark(job.status)}</span>
      <span className="min-w-0">
        <span className="block truncate font-medium">{runnerJobTitle(job)}</span>
      </span>
      <span className="shrink-0 text-xs text-muted-foreground">{formatRunnerDuration(job)}</span>
    </button>
  )
}

function RunnerJobLogPanel({
  job,
  request,
}: {
  job: RunnerState
  request: (url: string, options?: RequestInit) => Promise<unknown>
}) {
  const [selectedLog, setSelectedLog] = useState<(typeof logNames)[number]>("control.log")
  const [runnerLogText, setRunnerLogText] = useState("Loading runner log...")
  const [githubLog, setGithubLog] = useState<GitHubLogState>({ kind: "log", text: "Loading GitHub log..." })
  const [githubLogLoading, setGithubLogLoading] = useState(false)
  const endpoint = `/user/runner_requests/${encodeURIComponent(job.id)}`
  const endpointRef = useRef(endpoint)
  const terminalAvailable = isTerminalAvailable(job)
  const { terminalEl, terminalSession, terminalError, terminalConnecting, connectTerminal } = useSandboxTerminal({
    endpoint,
    available: terminalAvailable,
    request,
    connectingMessage: "Connecting to sandbox web console...",
    streamDisconnectedMessage: "Web console stream disconnected",
    connectErrorMessage: "Failed to connect web console",
  })

  useEffect(() => {
    endpointRef.current = endpoint
  }, [endpoint])

  useEffect(() => {
    let active = true
    queueMicrotask(() => {
      if (active) {
        setRunnerLogText("Loading runner log...")
      }
    })
    void request(`${endpoint}/logs/${encodeURIComponent(selectedLog)}`)
      .then((text) => {
        if (active) {
          setRunnerLogText(logResponseText(text, "Log is empty"))
        }
      })
      .catch((error) => {
        if (active) {
          setRunnerLogText(error instanceof Error ? error.message : "Failed to load runner log")
        }
      })
    return () => {
      active = false
    }
  }, [endpoint, request, selectedLog])

  useEffect(() => {
    let active = true
    queueMicrotask(() => {
      if (active) {
        setGithubLogLoading(true)
        setGithubLog({ kind: "log", text: "Loading GitHub log..." })
      }
    })
    void request(`${endpoint}/github-log`)
      .then((text) => {
        if (active) {
          setGithubLog(githubLogResponseState(text, "GitHub log is empty"))
        }
      })
      .catch((error) => {
        if (active) {
          setGithubLog(githubLogErrorState(error))
        }
      })
      .finally(() => {
        if (active) {
          setGithubLogLoading(false)
        }
      })
    return () => {
      active = false
    }
  }, [endpoint, request])

  const refreshGithubLog = () => {
    const refreshEndpoint = endpoint
    setGithubLogLoading(true)
    setGithubLog({ kind: "log", text: "Loading GitHub log..." })
    void request(`${refreshEndpoint}/github-log`)
      .then((text) => {
        if (endpointRef.current === refreshEndpoint) {
          setGithubLog(githubLogResponseState(text, "GitHub log is empty"))
        }
      })
      .catch((error) => {
        if (endpointRef.current === refreshEndpoint) {
          setGithubLog(githubLogErrorState(error))
        }
      })
      .finally(() => {
        if (endpointRef.current === refreshEndpoint) {
          setGithubLogLoading(false)
        }
      })
  }

  const githubLogActions = (
    <Button
      type="button"
      variant="outline"
      size="sm"
      className="border-white/15 bg-white/5 text-slate-100 hover:bg-white/10 hover:text-white"
      onClick={refreshGithubLog}
      disabled={githubLogLoading}
    >
      <RefreshCw className={cn(githubLogLoading && "animate-spin")} />
      Refresh
    </Button>
  )

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <Tabs defaultValue="github-logs" className="flex min-h-0 flex-1 flex-col gap-0">
        <TabsList className={jobLogTabsListClassName}>
          <TabsTrigger className={jobLogTabsTriggerClassName} value="github-logs">GitHub logs</TabsTrigger>
          <TabsTrigger className={jobLogTabsTriggerClassName} value="runner-logs">Runner logs</TabsTrigger>
          <TabsTrigger className={jobLogTabsTriggerClassName} value="web-console">Web Console</TabsTrigger>
          <TabsTrigger className={jobLogTabsTriggerClassName} value="details">Details</TabsTrigger>
        </TabsList>
        <TabsContent value="github-logs" className="m-0 pt-2">
          {githubLog.kind === "unavailable" ? (
            <GitHubLogsUnavailable detail={githubLog.detail} actions={githubLogActions} />
          ) : (
            <LogOutput
              text={githubLog.text}
              description="Workflow job output downloaded from GitHub Actions."
              actions={githubLogActions}
            />
          )}
        </TabsContent>
        <TabsContent value="runner-logs" className="m-0 pt-2">
          <LogOutput
            text={runnerLogText}
            description={`Runner ${selectedLog.replace(".log", "")} output captured by runnerd.`}
            leading={(
              <div className="flex items-center gap-1 rounded-md border border-white/10 bg-white/5 p-1" aria-label="Runner log stream">
                {logNames.map((name) => {
                  const value = name.replace(".log", "")
                  return (
                    <button
                      key={name}
                      type="button"
                      className={cn(
                        "rounded px-2 py-1 text-xs font-medium text-slate-300 transition-colors hover:bg-white/10 hover:text-white",
                        selectedLog === name && "bg-emerald-400/15 text-emerald-100"
                      )}
                      aria-pressed={selectedLog === name}
                      onClick={() => setSelectedLog(name)}
                    >
                      {value}
                    </button>
                  )
                })}
              </div>
            )}
          />
        </TabsContent>
        <TabsContent value="web-console" forceMount className="m-0 flex min-h-0 flex-1 flex-col overflow-hidden pt-2 data-[state=inactive]:hidden">
          <div className="flex min-h-0 flex-1 flex-col overflow-hidden border-y border-emerald-500/15 bg-[#111318] text-slate-100 shadow-[inset_3px_0_0_theme(colors.emerald.500/0.35)]">
            <div className="flex flex-wrap items-center justify-between gap-3 border-b border-white/10 bg-slate-900/95 px-4 py-3">
              <div className="min-w-0">
                <div className="truncate text-xs text-slate-400">{job.sandbox_id || "No active sandbox"}</div>
              </div>
              <Button
                type="button"
                size="sm"
                className="border-white/15 bg-white/5 text-slate-100 hover:bg-white/10 hover:text-white disabled:opacity-60"
                variant="outline"
                onClick={() => void connectTerminal()}
                disabled={!terminalAvailable || terminalConnecting || Boolean(terminalSession)}
              >
                <SquareTerminal />
                {terminalSession ? "Connected" : terminalConnecting ? "Connecting" : "Connect"}
              </Button>
            </div>
            {terminalError ? <WebConsoleError message={terminalError} /> : null}
            {terminalAvailable ? (
              <div className="relative min-h-0 flex-1 p-2">
                <div ref={terminalEl} className="h-full min-h-0 overflow-hidden rounded-md" />
                {!terminalSession ? (
                  <div className="absolute inset-2 flex items-center justify-center rounded-md bg-[#111318] text-sm text-slate-300">
                    Connect when you need an interactive shell.
                  </div>
                ) : null}
              </div>
            ) : (
              <WebConsoleUnavailable job={job} />
            )}
          </div>
        </TabsContent>
        <TabsContent value="details" className="m-0 py-5">
          <div className="grid gap-2 text-sm">
            <JobField label="Status" value={job.status} />
            <JobField label="Runner spec" value={job.runner_spec_name || "matched by labels"} />
            <JobField label="Workflow run" value={workflowRunValue(job)} />
            <JobField label="Queued" value={formatTime(job.created_at)} />
            <JobField label="Started" value={job.running_at ? formatTime(job.running_at) : "-"} />
            <JobField label="Finished" value={job.completed_at || job.failed_at ? formatTime(job.completed_at || job.failed_at) : "-"} />
            <JobField label="Commit" value={job.head_sha || "-"} />
          </div>
        </TabsContent>
      </Tabs>
    </div>
  )
}

function logResponseText(text: unknown, emptyMessage: string) {
  return typeof text === "string" ? text || emptyMessage : JSON.stringify(text, null, 2)
}

function githubLogResponseState(text: unknown, emptyMessage: string): GitHubLogState {
  const raw = logResponseText(text, emptyMessage)
  return { kind: "log", text: raw }
}

function githubLogErrorState(error: unknown): GitHubLogState {
  const raw = error instanceof Error ? error.message : "Failed to load GitHub log"
  return isGitHubLogUnavailable(raw) ? { kind: "unavailable", detail: raw } : { kind: "log", text: raw }
}

function isGitHubLogUnavailable(text: string) {
  const value = text.toLowerCase()
  return (
    value.includes("status 404") ||
    value.includes("blobnotfound") ||
    value.includes("the specified blob does not exist")
  )
}

function GitHubLogsUnavailable({ detail, actions }: { detail: string; actions: ReactNode }) {
  return (
    <div className="overflow-hidden border-y border-emerald-500/15 bg-slate-950 text-slate-100 shadow-[inset_3px_0_0_theme(colors.emerald.500/0.35)]">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-white/10 bg-slate-900/95 px-4 py-3">
        <div className="text-xs text-slate-400">Workflow job output downloaded from GitHub Actions.</div>
        <div className="flex items-center gap-2">{actions}</div>
      </div>
      <div className="flex min-h-[260px] items-center justify-center px-4 py-12">
        <div className="max-w-xl text-center">
          <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-md border border-white/10 bg-white/5 text-amber-200">
            <AlertCircle className="h-5 w-5" />
          </div>
          <h3 className="mt-4 text-sm font-semibold text-slate-100">GitHub logs are not available yet</h3>
          <p className="mt-2 text-sm leading-6 text-slate-400">
            GitHub may not have generated downloadable logs for this job. This can happen when a job never reached a runner, was cancelled before producing logs, or GitHub has not published the log archive yet.
          </p>
          <details className="mt-5 rounded-md border border-white/10 bg-white/[0.03] text-left">
            <summary className="cursor-pointer px-3 py-2 text-xs font-medium text-slate-300 hover:text-slate-100">
              Show technical details
            </summary>
            <pre className="max-h-48 overflow-auto border-t border-white/10 px-3 py-3 font-mono text-xs leading-relaxed whitespace-pre-wrap break-words text-slate-400">
              {detail}
            </pre>
          </details>
        </div>
      </div>
    </div>
  )
}

function WebConsoleError({ message }: { message: string }) {
  return (
    <div className="border-b border-red-400/15 bg-red-500/10 px-4 py-3 text-sm text-red-100">
      <div className="flex min-w-0 items-start gap-2">
        <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-red-200" />
        <div className="min-w-0">
          <div className="font-medium">Web Console connection failed</div>
          <div className="mt-1 break-words font-mono text-xs leading-relaxed text-red-100/80">{message}</div>
        </div>
      </div>
    </div>
  )
}

function WebConsoleUnavailable({ job }: { job: RunnerState }) {
  const reason = job.sandbox_id
    ? "The sandbox is no longer accepting web console sessions for this job state."
    : "The sandbox has already been cleaned up, so a web console session cannot be opened."
  return (
    <div className="flex min-h-[320px] items-center justify-center px-4 py-12">
      <div className="max-w-md text-center">
        <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-md border border-white/10 bg-white/5 text-emerald-200">
          <SquareTerminal className="h-5 w-5" />
        </div>
        <h3 className="mt-4 text-sm font-semibold text-slate-100">Web Console unavailable</h3>
        <p className="mt-2 text-sm leading-6 text-slate-400">
          Web Console is available while a sandbox job is creating, running, or stopping. {reason}
        </p>
        <div className="mt-4 inline-flex items-center gap-2 rounded-md border border-white/10 bg-white/5 px-3 py-1.5 text-xs text-slate-300">
          <span className="text-slate-500">Status</span>
          <span className="font-medium text-slate-100">{job.status}</span>
        </div>
      </div>
    </div>
  )
}

function LogOutput({
  text,
  description,
  actions,
  leading,
}: {
  text: string
  description: string
  actions?: ReactNode
  leading?: ReactNode
}) {
  const logRef = useRef<HTMLDivElement | null>(null)
  const [collapseState, setCollapseState] = useState<{ text: string; groups: Set<number> }>(() => ({ text, groups: new Set() }))
  const collapsedGroups = useMemo(() => (collapseState.text === text ? collapseState.groups : new Set<number>()), [collapseState, text])
  const lines = useMemo(() => text.split(/\r?\n/), [text])
  const largeLog = lines.length > 20000
  const logLines = useMemo(() => (largeLog ? [] : parseLogLines(lines, collapsedGroups)), [lines, collapsedGroups, largeLog])
  const numberWidth = `${Math.max(2, String(lines.length).length)}ch`

  const scrollToBottom = () => {
    logRef.current?.scrollIntoView({ behavior: "smooth", block: "end" })
  }

  const toggleGroup = (groupID: number) => {
    setCollapseState((current) => {
      const next = new Set(current.text === text ? current.groups : [])
      if (next.has(groupID)) {
        next.delete(groupID)
      } else {
        next.add(groupID)
      }
      return { text, groups: next }
    })
  }

  return (
    <div className="overflow-hidden border-y border-emerald-500/15 bg-slate-950 text-slate-100 shadow-[inset_3px_0_0_theme(colors.emerald.500/0.35)]">
      <div className="sticky top-0 z-10 flex flex-wrap items-center justify-between gap-3 border-b border-white/10 bg-slate-900/95 px-4 py-3 backdrop-blur">
        <div className="flex min-w-0 flex-wrap items-center gap-3">
          {leading}
          <div className="min-w-0 text-xs text-slate-400">{description}</div>
        </div>
        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="border-white/15 bg-white/5 text-slate-100 hover:bg-white/10 hover:text-white"
            onClick={scrollToBottom}
          >
            Scroll to Bottom
          </Button>
          {actions}
        </div>
      </div>
      <div ref={logRef} className="py-3 font-mono text-xs leading-relaxed">
        {largeLog ? (
          <pre className="max-h-[70vh] overflow-auto whitespace-pre-wrap break-words px-4 text-slate-200">{text}</pre>
        ) : logLines.map((logLine) => {
          const rowStyle = { "--line-number-width": numberWidth } as CSSProperties
          const rowClassName = "grid grid-cols-[12px_var(--line-number-width)_minmax(0,1fr)] gap-1 px-4"
          if (logLine.groupID !== undefined && logLine.kind === "group-start") {
            return (
              <button
                key={`${logLine.index}-${logLine.text.slice(0, 16)}`}
                type="button"
                className={cn(rowClassName, "group text-left")}
                style={rowStyle}
                onClick={() => toggleGroup(logLine.groupID ?? logLine.index)}
                aria-expanded={!collapsedGroups.has(logLine.groupID ?? logLine.index)}
              >
                <span className="flex h-[1.625em] select-none items-center justify-center text-slate-300 group-hover:text-emerald-200">
                  <Play
                    className={cn(
                      "h-3 w-3 max-w-none fill-current stroke-current",
                      !collapsedGroups.has(logLine.groupID ?? logLine.index) && "rotate-90"
                    )}
                  />
                </span>
                <span className="select-none text-right text-slate-500">{logLine.index + 1}</span>
                <span className={cn("min-w-0 whitespace-pre-wrap break-words text-left text-slate-200 group-hover:text-emerald-200", logLineClass(logLine.text))}>{logLine.displayText || " "}</span>
              </button>
            )}
          return (
            <div key={`${logLine.index}-${logLine.text.slice(0, 16)}`} className={rowClassName} style={rowStyle}>
              <span />
              <span className="select-none text-right text-slate-500">{logLine.index + 1}</span>
              <span className={cn("min-w-0 whitespace-pre-wrap break-words text-slate-200", logLineClass(logLine.text))}>{logLine.displayText || " "}</span>
            </div>
          )
        })}
      </div>
    </div>
  )
}

function logLineClass(line: string) {
  if (line.includes("##[group]") || line.includes("##[endgroup]")) return "font-semibold text-emerald-300"
  if (line.trimStart().startsWith("$ ")) return "font-semibold text-cyan-200"
  return ""
}

type ParsedLogLine = {
  index: number
  text: string
  displayText: string
  kind: "line" | "group-start" | "group-end"
  groupID?: number
}

function parseLogLines(lines: string[], collapsedGroups: Set<number>): ParsedLogLine[] {
  const visible: ParsedLogLine[] = []
  const stack: number[] = []

  lines.forEach((text, index) => {
    const hiddenByParent = stack.some((groupID) => collapsedGroups.has(groupID))

    if (text.includes("##[group]")) {
      if (!hiddenByParent) {
        visible.push({ index, text, displayText: text.replace("##[group]", ""), kind: "group-start", groupID: index })
      }
      stack.push(index)
      return
    }

    if (text.includes("##[endgroup]")) {
      stack.pop()
      return
    }

    if (!hiddenByParent) {
      visible.push({ index, text, displayText: text, kind: "line" })
    }
  })

  return visible
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

function pullRequestHeading(group: BuildGroup, jobGroup: RunnerJobGroup | null) {
  const title = jobGroup?.pull_request_title?.trim()
  const label = jobGroup?.title || group.title
  return title ? `${label}: ${title}` : label
}

function workflowGroups(jobs: RunnerState[]) {
  const groups = new Map<number | string, { id: number | string; name: string; jobs: RunnerState[] }>()
  for (const job of jobs) {
    const id = job.workflow_run_id || job.id
    const group = groups.get(id)
    if (group) {
      group.jobs.push(job)
      continue
    }
    groups.set(id, {
      id,
      name: job.workflow_name || "Workflow run",
      jobs: [job],
    })
  }
  return Array.from(groups.values())
}

function workflowStatus(jobs: RunnerState[]) {
  if (jobs.some((job) => job.status === "failed")) return "failed"
  if (jobs.some((job) => ["queued", "creating", "running", "stopping"].includes(job.status))) return "active"
  return "completed"
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
        "group relative flex w-full gap-2 bg-background/60 py-4 pl-3 pr-4 text-left transition-colors hover:bg-accent/70",
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

function BuildGroupStatusIcon({ status }: { status: RunnerStatusSummary }) {
  const className = "h-4 w-4"
  if (status === "failed") return <X className={className} />
  if (status === "active") return <Loader2 className={cn(className, "animate-spin")} />
  return <Check className={className} />
}

function BuildGroupStatusBadge({ group, status }: { group: BuildGroup; status: RunnerStatusSummary }) {
  if (status === "failed") {
    return (
      <Badge variant="danger" className="self-center">
        <X />
        {buildGroupStatusLabel(group, "failed")}
      </Badge>
    )
  }
  if (status === "active") {
    return (
      <Badge variant="warning" className="self-center">
        <Loader2 className="animate-spin" />
        {buildGroupStatusLabel(group, "running")}
      </Badge>
    )
  }
  return (
    <Badge variant="success" className="self-center">
      <Check />
      {buildGroupStatusLabel(group, "passed")}
    </Badge>
  )
}

function buildGroupStatusLabel(group: BuildGroup, statusText: "failed" | "running" | "passed") {
  switch (group.kind) {
    case "pull_request":
      return `PR ${statusText}`
    case "workflow_run":
      return `run ${statusText}`
    case "branch":
      return `branch ${statusText}`
    default:
      return `job ${statusText}`
  }
}

function buildGroupStatus(group: BuildGroup): RunnerStatusSummary {
  const jobs = currentBuildJobs(group)
  if (jobs.some((job) => job.status === "failed")) return "failed"
  if (jobs.some((job) => job.status === "queued" || job.status === "creating" || job.status === "running" || job.status === "stopping")) {
    return "active"
  }
  return "completed"
}

function jobStatusSummary(status: RunnerState["status"]): RunnerStatusSummary {
  if (status === "failed") return "failed"
  if (status === "queued" || status === "creating" || status === "running" || status === "stopping") return "active"
  return "completed"
}

function jobStatusTextClass(status: RunnerState["status"]) {
  return buildGroupStatusClasses(jobStatusSummary(status)).title
}

function jobStatusMark(status: RunnerState["status"]) {
  const className = "h-4 w-4"
  switch (status) {
    case "failed":
      return <X className={className} />
    case "queued":
    case "creating":
    case "running":
    case "stopping":
      return <Loader2 className={cn(className, "animate-spin")} />
    default:
      return <Check className={className} />
  }
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
        bar: "bg-yellow-500",
        icon: "text-yellow-700 dark:text-yellow-400",
        title: "text-yellow-700 dark:text-yellow-400",
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

function isTerminalAvailable(job: RunnerState) {
  return Boolean(job.sandbox_id && ["creating", "running", "stopping"].includes(job.status))
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

function isRunnerJobGroup(value: unknown): value is RunnerJobGroup {
  if (!value || typeof value !== "object") return false
  const candidate = value as Partial<RunnerJobGroup>
  return Array.isArray(candidate.jobs) && Array.isArray(candidate.current_jobs) && Array.isArray(candidate.previous_jobs)
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
