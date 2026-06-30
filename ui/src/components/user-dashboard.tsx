import { Github, GitPullRequest, ListTree, LogOut, Trash2, UserRound } from "lucide-react"
import { type MouseEvent, useEffect, useMemo, useState } from "react"

import type { AuthSession, GitHubAppConfig, RunnerState } from "@/admin-types"
import { formatTime } from "@/admin-format"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Separator } from "@/components/ui/separator"
import { cn } from "@/lib/utils"

type PRGroup = {
  key: string
  repository: string
  prLabel: string
  updatedAt: string
  jobs: RunnerState[]
}

type UserPage = "home" | "repositories" | "accounts"

export function UserDashboard({
  authSession,
  githubApp,
  runners,
  selectedKey,
  page,
  authorizedRepositories,
  loadingRepositoriesFor,
  onDeleteInstallation,
  onLoadAuthorizedRepositories,
  onNavigate,
  onSelectKey,
  onSignOut,
}: {
  authSession: AuthSession
  githubApp: GitHubAppConfig | null
  runners: RunnerState[]
  selectedKey: string
  page: UserPage
  authorizedRepositories: Record<number, string[]>
  loadingRepositoriesFor: number | null
  onDeleteInstallation: (id: number) => void
  onLoadAuthorizedRepositories: (id: number) => void
  onNavigate: (page: UserPage) => void
  onSelectKey: (key: string) => void
  onSignOut: () => void
}) {
  const groups = useMemo(() => groupRunnersByPR(runners), [runners])
  const selected = groups.find((group) => group.key === selectedKey) || groups[0]
  const installations = githubApp?.installations ?? []
  const hasInstallations = installations.length > 0
  const repositoryCount = useMemo(
    () => new Set(installations.flatMap((installation) => installation.repositories)).size,
    [installations]
  )
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
          <div className="text-sm font-semibold">E2B Runner Console</div>
          <div className="text-xs text-muted-foreground">@{authSession.login} workspace</div>
        </div>
        <nav className="ml-3 hidden items-center gap-1 md:flex" aria-label="Workspace">
          <a href="/" className={navItemClass(page === "home")} onClick={(event) => goToPage(event, "home")}>
            <GitPullRequest className="h-4 w-4" />
            Pull Requests
          </a>
          <a
            href="/repositories"
            className={navItemClass(page === "repositories")}
            onClick={(event) => goToPage(event, "repositories")}
          >
            <ListTree className="h-4 w-4" />
            Activity
          </a>
          <a href="/accounts" className={navItemClass(page === "accounts")} onClick={(event) => goToPage(event, "accounts")}>
            <UserRound className="h-4 w-4" />
            Accounts
          </a>
        </nav>
        <div className="ml-auto flex items-center gap-2">
          <Button type="button" variant="ghost" size="icon" onClick={onSignOut} aria-label="Sign out">
            <LogOut className="h-4 w-4" />
          </Button>
        </div>
      </header>

      <nav className="flex items-center gap-1 border-b px-4 py-2 md:hidden" aria-label="Workspace">
        <a href="/" className={navItemClass(page === "home")} onClick={(event) => goToPage(event, "home")}>
          <GitPullRequest className="h-4 w-4" />
          Pull Requests
        </a>
        <a
          href="/repositories"
          className={navItemClass(page === "repositories")}
          onClick={(event) => goToPage(event, "repositories")}
        >
          <ListTree className="h-4 w-4" />
          Activity
        </a>
        <a href="/accounts" className={navItemClass(page === "accounts")} onClick={(event) => goToPage(event, "accounts")}>
          <UserRound className="h-4 w-4" />
          Accounts
        </a>
      </nav>

      {page === "repositories" ? (
        <ActivityRepositoriesPage
          installations={installations}
          repositoryCount={repositoryCount}
          onNavigate={onNavigate}
        />
      ) : page === "accounts" ? (
        <AccountsPage
          githubApp={githubApp}
          installations={installations}
          authorizedRepositories={authorizedRepositories}
          loadingRepositoriesFor={loadingRepositoriesFor}
          onDeleteInstallation={onDeleteInstallation}
          onLoadAuthorizedRepositories={onLoadAuthorizedRepositories}
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
  repositoryCount,
  onNavigate,
}: {
  installations: NonNullable<GitHubAppConfig["installations"]>
  repositoryCount: number
  onNavigate: (page: UserPage) => void
}) {
  return (
    <>
      <section className="border-b bg-muted/35 px-4 py-4 lg:px-6">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h1 className="text-xl font-semibold">Activity repositories</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Local repositories appear here after runnerd observes jobs from them.
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="secondary">{installations.length} installations</Badge>
            <Badge variant="secondary">{repositoryCount} active repositories</Badge>
            <Button type="button" variant="outline" size="sm" onClick={() => onNavigate("accounts")}>
              <UserRound className="h-4 w-4" />
              Accounts
            </Button>
          </div>
        </div>
      </section>

      <section className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4 lg:p-6">
        {installations.length ? (
          <div className="grid gap-4">
            {installations.map((installation) => (
              <Card key={installation.id} className="rounded-lg">
                <CardHeader className="gap-3 pb-3">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <CardTitle className="text-base">{installation.account_login || "GitHub App"}</CardTitle>
                    </div>
                    <Badge variant="secondary">{installation.repositories.length} active repositories</Badge>
                  </div>
                </CardHeader>
                <CardContent>
                  {installation.repositories.length ? (
                    <div className="grid gap-2 md:grid-cols-2 xl:grid-cols-3">
                      {installation.repositories.map((repository) => (
                        <div key={repository} className="rounded-md border bg-muted/25 px-3 py-2">
                          <div className="truncate text-sm font-medium">{repository}</div>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <div className="rounded-md border bg-muted/25 px-3 py-2 text-sm text-muted-foreground">
                      No repositories have runner jobs for this installation yet.
                    </div>
                  )}
                </CardContent>
              </Card>
            ))}
          </div>
        ) : (
          <div className="rounded-lg border bg-muted/30 p-6 text-sm text-muted-foreground">
            <button type="button" className="text-left text-primary hover:underline" onClick={() => onNavigate("accounts")}>
              Connect a GitHub App account, then trigger a PR workflow to show active repositories here.
            </button>
          </div>
        )}
      </section>
    </>
  )
}

function AccountsPage({
  githubApp,
  installations,
  authorizedRepositories,
  loadingRepositoriesFor,
  onDeleteInstallation,
  onLoadAuthorizedRepositories,
}: {
  githubApp: GitHubAppConfig | null
  installations: NonNullable<GitHubAppConfig["installations"]>
  authorizedRepositories: Record<number, string[]>
  loadingRepositoriesFor: number | null
  onDeleteInstallation: (id: number) => void
  onLoadAuthorizedRepositories: (id: number) => void
}) {
  const [selectedID, setSelectedID] = useState<number | null>(null)
  const [filter, setFilter] = useState("")
  const selected = installations.find((installation) => installation.id === selectedID) || installations[0]
  const authorized = selected ? authorizedRepositories[selected.id] : undefined
  const filteredRepositories = useMemo(() => {
    const query = filter.trim().toLowerCase()
    const repositories = authorized || []
    if (!query) return repositories
    return repositories.filter((repository) => repository.toLowerCase().includes(query))
  }, [authorized, filter])

  useEffect(() => {
    if (!selected) {
      setSelectedID(null)
      return
    }
    if (selectedID !== selected.id) {
      setSelectedID(selected.id)
    }
    if (!authorizedRepositories[selected.id]) {
      onLoadAuthorizedRepositories(selected.id)
    }
  }, [authorizedRepositories, onLoadAuthorizedRepositories, selected, selectedID])

  return (
    <>
      <section className="border-b bg-muted/35 px-4 py-4 lg:px-6">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h1 className="text-xl font-semibold">Accounts</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Manage GitHub App accounts and inspect repositories currently authorized on GitHub.
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
            <div className="flex h-12 items-center justify-between border-b px-4">
              <span className="text-sm font-semibold">GitHub accounts</span>
              <Badge variant="outline">{installations.length}</Badge>
            </div>
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
                      setSelectedID(installation.id)
                      setFilter("")
                      if (!authorizedRepositories[installation.id]) onLoadAuthorizedRepositories(installation.id)
                    }}
                  >
                    <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-foreground text-xs font-semibold text-background">
                      {(installation.account_login || "GH").slice(0, 2).toUpperCase()}
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm font-semibold">{installation.account_login || "GitHub App"}</div>
                    </div>
                  </button>
                ))
              ) : (
                <div className="p-4 text-sm text-muted-foreground">
                  Install the GitHub App to connect a user or organization account.
                </div>
              )}
            </div>
          </div>
        </aside>

        <section className="min-h-0 overflow-y-auto p-4 lg:p-6">
          {selected ? (
            <div className="space-y-4">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <h2 className="text-2xl font-semibold">{selected.account_login || "GitHub App"}</h2>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <Button type="button" variant="outline" size="sm" asChild>
                    <a href={`https://github.com/settings/installations/${selected.installation_id}`}>
                      <Github className="h-4 w-4" />
                      Manage on GitHub
                    </a>
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8"
                    onClick={() => onDeleteInstallation(selected.id)}
                    aria-label="Delete installation"
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              </div>

              <Card className="rounded-lg">
                <CardHeader className="gap-3 pb-3">
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div>
                      <CardTitle className="text-base">Authorized repositories</CardTitle>
                      <CardDescription>Loaded from GitHub App authorization for this account.</CardDescription>
                    </div>
                    <Badge variant="secondary">
                      {authorized ? `${authorized.length} repositories` : "Not loaded"}
                    </Badge>
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
            </div>
          ) : (
            <div className="rounded-lg border bg-muted/30 p-6 text-sm text-muted-foreground">
              Install the GitHub App to connect a user or organization account.
            </div>
          )}
        </section>
      </div>
    </>
  )
}

function PullRequestsPage({
  groups,
  hasInstallations,
  selected,
  onSelectKey,
  onNavigate,
}: {
  groups: PRGroup[]
  hasInstallations: boolean
  selected: PRGroup | undefined
  onSelectKey: (key: string) => void
  onNavigate: (page: UserPage) => void
}) {
  return (
    <>
      <section className="border-b bg-muted/35 px-4 py-4 lg:px-6">
        <div className="space-y-3">
          <div>
            <h1 className="text-xl font-semibold">Pull requests</h1>
            <p className="text-sm text-muted-foreground">
              Review pull requests and the runner jobs currently attached to them.
            </p>
          </div>
        </div>
      </section>

      <div className="grid min-h-0 flex-1 xl:grid-cols-[360px_minmax(0,1fr)]">
        <aside className="min-h-0 border-r bg-muted/20">
          <div className="flex h-full flex-col">
            <div className="flex h-12 items-center justify-between border-b px-4">
              <span className="text-sm font-semibold">Repo + PR</span>
              <Badge variant="outline">{groups.length}</Badge>
            </div>
            <div className="min-h-0 flex-1 overflow-y-auto">
              {groups.length ? (
                groups.map((group) => (
                  <button
                    key={group.key}
                    type="button"
                    onClick={() => onSelectKey(group.key)}
                    className={cn(
                      "flex w-full flex-col gap-2 border-b px-4 py-3 text-left transition-colors hover:bg-accent",
                      selected?.key === group.key ? "bg-accent" : ""
                    )}
                  >
                    <div className="flex items-center gap-2">
                      <GitPullRequest className="h-4 w-4 text-primary" />
                      <span className="truncate text-sm font-semibold">{group.prLabel}</span>
                    </div>
                    <div className="truncate text-xs text-muted-foreground">{group.repository}</div>
                    <div className="flex items-center gap-2">
                      <Badge variant="secondary">{group.jobs.length} jobs</Badge>
                      <span className="text-xs text-muted-foreground">{formatTime(group.updatedAt)}</span>
                    </div>
                  </button>
                ))
              ) : (
                <div className="p-4 text-sm text-muted-foreground">
                  {hasInstallations ? (
                    "No PR jobs yet. Trigger a workflow in an installed repository, then refresh."
                  ) : (
                    <button
                      type="button"
                      className="text-left text-primary hover:underline"
                      onClick={() => onNavigate("accounts")}
                    >
                      Connect a GitHub App account to start tracking PR jobs.
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
                <div className="text-sm text-muted-foreground">{selected.repository}</div>
                <h2 className="text-2xl font-semibold">{selected.prLabel}</h2>
              </div>
              <div className="grid gap-3">
                {selected.jobs.map((job) => (
                  <Card key={job.id} className="rounded-lg">
                    <CardHeader className="gap-3 pb-3">
                      <div className="flex flex-wrap items-start justify-between gap-3">
                        <div>
                          <CardTitle className="text-base">{job.assigned_job_name || job.runner_name}</CardTitle>
                          <CardDescription>{job.id}</CardDescription>
                        </div>
                        <Badge className={userStatusClass(job.status)}>{job.status}</Badge>
                      </div>
                    </CardHeader>
                    <CardContent className="grid gap-3 text-sm md:grid-cols-3">
                      <JobField label="Runner spec" value={job.runner_spec_name || "matched by labels"} />
                      <JobField label="Workflow run" value={job.workflow_run_id ? String(job.workflow_run_id) : "unknown"} />
                      <JobField label="Updated" value={formatTime(job.updated_at)} />
                      {job.github_job_url ? (
                        <>
                          <Separator className="md:col-span-3" />
                          <a className="text-sm font-medium text-primary hover:underline md:col-span-3" href={job.github_job_url}>
                            Open GitHub Actions job
                          </a>
                        </>
                      ) : null}
                    </CardContent>
                  </Card>
                ))}
              </div>
            </div>
          ) : (
            <div className="rounded-lg border bg-muted/30 p-6 text-sm text-muted-foreground">
              {hasInstallations ? (
                "No runner jobs are available yet. Trigger a PR workflow in an installed repository to see jobs here."
              ) : (
                <button
                  type="button"
                  className="text-left text-primary hover:underline"
                  onClick={() => onNavigate("accounts")}
                >
                  Connect a GitHub App account to start tracking PR jobs.
                </button>
              )}
            </div>
          )}
        </section>
      </div>
    </>
  )
}

function JobField({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 break-words font-medium">{value}</div>
    </div>
  )
}

function groupRunnersByPR(runners: RunnerState[]): PRGroup[] {
  const groups = new Map<string, PRGroup>()
  for (const runner of runners) {
    const repository = runner.repository_full_name || "unknown/repository"
    const pr = runner.pull_request_number ? `PR #${runner.pull_request_number}` : "Manual or workflow job"
    const key = `${repository}:${runner.pull_request_number || runner.workflow_run_id || runner.id}`
    const current = groups.get(key)
    if (current) {
      current.jobs.push(runner)
      if (runner.updated_at > current.updatedAt) current.updatedAt = runner.updated_at
      continue
    }
    groups.set(key, {
      key,
      repository,
      prLabel: pr,
      updatedAt: runner.updated_at,
      jobs: [runner],
    })
  }
  return Array.from(groups.values()).sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
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
      return "bg-destructive text-destructive-foreground"
    default:
      return ""
  }
}
