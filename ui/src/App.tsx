import { useCallback, useEffect, useMemo, useState } from "react"
import { toast } from "sonner"

import { AppSidebar } from "@/components/app-sidebar"
import { AuditSection, DiagnosticsSection, MatchSection, OverviewSection } from "@/components/admin-sections"
import { LoginPage } from "@/components/login-page"
import { RunnerGroupsSection } from "@/components/runner-groups-section"
import { RunnerPoliciesSection } from "@/components/runner-policies-section"
import { RunnerRequestsSection } from "@/components/runner-requests-section"
import { RunnerSpecsSection } from "@/components/runner-specs-section"
import { SiteHeader } from "@/components/site-header"
import { UserDashboard } from "@/components/user-dashboard"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar"
import { Toaster } from "@/components/ui/sonner"
import {
  activeStatuses,
  adminSections,
  logNames,
  sectionFromPath,
  type AdminSection,
  type AuditEvent,
  type AuthSession,
  type AuthorizedRepositories,
  type DiagnosticsSummary,
  type GitHubAppConfig,
  type GitHubInstallation,
  type Metric,
  type RunnerGroup,
  type RunnerPolicy,
  type RunnerSpec,
  type RunnerSpecMatch,
  type RunnerState,
  type RunnerStatus,
} from "@/admin-types"
import { useRunnerCatalog } from "@/hooks/use-runner-catalog"

type AccountSettingsTab = "repositories" | "preferences"
type AccountSettingsRoute = {
  accountLogin?: string
  tab: AccountSettingsTab
}

function App() {
  const [authSession, setAuthSession] = useState<AuthSession>({ authenticated: false, oauth_enabled: false })
  const [locationPath, setLocationPath] = useState(() => window.location.pathname)
  const [section, setSectionState] = useState<AdminSection>(() => sectionFromPath())
  const [runners, setRunners] = useState<RunnerState[]>([])
  const [runnerSpecs, setRunnerSpecs] = useState<RunnerSpec[]>([])
  const [runnerGroups, setRunnerGroups] = useState<RunnerGroup[]>([])
  const [runnerPolicies, setRunnerPolicies] = useState<RunnerPolicy[]>([])
  const [selectedID, setSelectedID] = useState("")
  const [selectedLog, setSelectedLog] = useState<(typeof logNames)[number]>("control.log")
  const [logText, setLogText] = useState("No runner selected")
  const [loading, setLoading] = useState(false)
  const [connected, setConnected] = useState(false)
  const [createID, setCreateID] = useState("")
  const [createRepository, setCreateRepository] = useState("")
  const [createRunnerSpec, setCreateRunnerSpec] = useState("")
  const [createLabels, setCreateLabels] = useState("self-hosted,e2b")
  const [createRunnerOpen, setCreateRunnerOpen] = useState(false)
  const [runnerStatusFilter, setRunnerStatusFilter] = useState<RunnerStatus | "all">("all")
  const [runnerRepositoryFilter, setRunnerRepositoryFilter] = useState("all")
  const [runnerSpecFilter, setRunnerSpecFilter] = useState("all")
  const [matchRepository, setMatchRepository] = useState("")
  const [matchLabels, setMatchLabels] = useState("self-hosted,e2b")
  const [matchResult, setMatchResult] = useState<RunnerSpecMatch | null>(null)
  const [diagnostics, setDiagnostics] = useState<DiagnosticsSummary | null>(null)
  const [diagnosticsVars, setDiagnosticsVars] = useState("")
  const [auditEvents, setAuditEvents] = useState<AuditEvent[]>([])
  const [userRunners, setUserRunners] = useState<RunnerState[]>([])
  const [githubApp, setGitHubApp] = useState<GitHubAppConfig | null>(null)
  const [authorizedRepositories, setAuthorizedRepositories] = useState<Record<number, string[]>>({})
  const [loadingRepositoriesFor, setLoadingRepositoriesFor] = useState<number | null>(null)
  const [userSelectedKey, setUserSelectedKey] = useState("")

  const setSection = useCallback((next: string) => {
    const section = adminSections.includes(next as AdminSection) ? (next as AdminSection) : "overview"
    setSectionState(section)
    const nextPath = section === "overview" ? "/admin/" : `/admin/${section}`
    if (window.location.pathname !== nextPath) {
      window.history.pushState(null, "", nextPath)
      setLocationPath(nextPath)
    }
  }, [])

  const setUserPage = useCallback((next: "home" | "repositories" | "settings") => {
    const nextPath = next === "settings" ? "/account/repositories" : next === "repositories" ? "/repositories" : "/"
    if (window.location.pathname !== nextPath) {
      window.history.pushState(null, "", nextPath)
    }
    setLocationPath(nextPath)
  }, [])

  const setAccountSettingsRoute = useCallback(
    (accountLogin: string | undefined, tab: AccountSettingsTab) => {
      const nextPath = accountSettingsPath(accountLogin, authSession.login, tab)
      if (window.location.pathname !== nextPath) {
        window.history.pushState(null, "", nextPath)
      }
      setLocationPath(nextPath)
    },
    [authSession.login]
  )

  const selected = useMemo(
    () => runners.find((runner) => runner.id === selectedID),
    [runners, selectedID]
  )

  const runnerRepositories = useMemo(
    () =>
      Array.from(new Set(runners.map((runner) => runner.repository_full_name).filter(Boolean) as string[])).sort(),
    [runners]
  )

  const runnerSpecNames = useMemo(
    () =>
      Array.from(
        new Set(
          [
            ...runnerSpecs.map((runnerSpec) => runnerSpec.name),
            ...runners.map((runner) => runner.runner_spec_name || ""),
          ].filter(Boolean)
        )
      ).sort(),
    [runnerSpecs, runners]
  )

  const filteredRunners = useMemo(
    () =>
      runners.filter((runner) => {
        if (runnerStatusFilter !== "all" && runner.status !== runnerStatusFilter) return false
        if (runnerRepositoryFilter !== "all" && runner.repository_full_name !== runnerRepositoryFilter) return false
        if (runnerSpecFilter !== "all" && runner.runner_spec_name !== runnerSpecFilter) return false
        return true
      }),
    [runnerRepositoryFilter, runnerSpecFilter, runnerStatusFilter, runners]
  )

  const hasAccess = authSession.authenticated && authSession.role === "admin"
  const isAdminRoute = locationPath === "/admin" || locationPath.startsWith("/admin/")
  const accountSettingsRoute = parseAccountSettingsRoute(locationPath, authSession.login)
  const userPage = accountSettingsRoute ? "settings" : locationPath === "/repositories" ? "repositories" : "home"

  const metrics = useMemo<Metric[]>(() => {
    const count = (status: RunnerStatus) => runners.filter((runner) => runner.status === status).length
    return [
      {
        label: "Active",
        value: runners.filter((runner) => activeStatuses.has(runner.status)).length,
        description: "queued / creating / running / stopping",
      },
      { label: "Completed", value: count("completed"), description: "cleaned after exit" },
      { label: "Failed", value: count("failed"), description: "needs inspection" },
      { label: "Runner specs", value: runnerSpecs.length, description: "active control-plane runner specs" },
    ]
  }, [runnerSpecs.length, runners])

  const request = useCallback(
    async (url: string, options: RequestInit = {}) => {
      const headers = new Headers(options.headers)
      const response = await fetch(url, { ...options, headers, credentials: "same-origin" })
      if (response.status === 401) {
        try {
          const sessionResponse = await fetch("/auth/session", { credentials: "same-origin" })
          if (sessionResponse.ok) {
            setAuthSession((await sessionResponse.json()) as AuthSession)
          }
        } catch {
          setAuthSession((current) => ({ ...current, authenticated: false, login: undefined, role: undefined, avatar_url: undefined, expires_at: undefined }))
        }
        setConnected(false)
        throw new Error("Session expired or access is not allowed")
      }
      if (!response.ok) {
        const text = await response.text()
        throw new Error(text || `${response.status} ${response.statusText}`)
      }
      const contentType = response.headers.get("content-type") || ""
      if (contentType.includes("application/json")) return response.json()
      return response.text()
    },
    []
  )

  const parseLabels = (value: string) =>
    value
      .split(",")
      .map((label) => label.trim())
      .filter(Boolean)

  const loadLog = useCallback(
    async (id: string, name: (typeof logNames)[number]) => {
      if (!hasAccess || !id) {
        setLogText("No runner selected")
        return
      }
      setLogText("Loading...")
      try {
        const text = (await request(
          `/runner_requests/${encodeURIComponent(id)}/logs/${encodeURIComponent(name)}`
        )) as string
        setLogText(text || "Log is empty")
      } catch (error) {
        setLogText(error instanceof Error ? error.message : "Failed to load log")
      }
    },
    [hasAccess, request]
  )

  const loadAll = useCallback(async () => {
    if (!hasAccess) {
      setConnected(false)
      return
    }
    setLoading(true)
    try {
      const [runnerData, runnerSpecData, runnerGroupData, policyData, auditData] = await Promise.all([
        request("/runner_requests"),
        request("/runner_specs"),
        request("/runner_groups"),
        request("/runner_policies"),
        request("/audit-events"),
      ])
      const nextRunners = Array.isArray(runnerData) ? (runnerData as RunnerState[]) : []
      setRunners(nextRunners)
      setRunnerSpecs(Array.isArray(runnerSpecData) ? (runnerSpecData as RunnerSpec[]) : [])
      setRunnerGroups(Array.isArray(runnerGroupData) ? (runnerGroupData as RunnerGroup[]) : [])
      setRunnerPolicies(Array.isArray(policyData) ? (policyData as RunnerPolicy[]) : [])
      setAuditEvents(Array.isArray(auditData) ? (auditData as AuditEvent[]) : [])
      setConnected(true)
      if (selectedID && !nextRunners.some((runner) => runner.id === selectedID)) {
        setSelectedID("")
        setLogText("No runner selected")
      }
    } catch (error) {
      setConnected(false)
      toast.error(error instanceof Error ? error.message : "Failed to load control plane data")
    } finally {
      setLoading(false)
    }
  }, [hasAccess, request, selectedID])

  const loadUserAll = useCallback(async () => {
    if (!authSession.authenticated || (hasAccess && isAdminRoute)) return
    setLoading(true)
    try {
      const [appData, runnerData] = await Promise.all([
        request("/user/github-app"),
        request("/user/runner_requests"),
      ])
      const nextApp = appData as GitHubAppConfig
      const nextRunners = Array.isArray(runnerData) ? (runnerData as RunnerState[]) : []
      setGitHubApp(nextApp)
      setUserRunners(nextRunners)
      if (nextRunners.length === 0) setUserSelectedKey("")
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to load workspace data")
    } finally {
      setLoading(false)
    }
  }, [authSession.authenticated, hasAccess, isAdminRoute, request])

  const syncGitHubAppSetupFromURL = useCallback(async () => {
    if (!authSession.authenticated || (hasAccess && isAdminRoute) || !isAccountSettingsPath(locationPath)) return
    const params = new URLSearchParams(window.location.search)
    const installationID = Number(params.get("installation_id") || "")
    if (!Number.isSafeInteger(installationID) || installationID <= 0) return
    const setupState = params.get("state") || ""
    setLoading(true)
    try {
      const installation = (await request("/user/github-app/installations", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ installation_id: installationID, setup_state: setupState }),
      })) as GitHubInstallation
      toast.success("GitHub App account connected")
      const nextPath = accountSettingsPathForInstallation(installation, authSession.login, "repositories")
      window.history.replaceState(null, "", nextPath)
      setLocationPath(nextPath)
      await loadUserAll()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to sync GitHub App repositories")
    } finally {
      setLoading(false)
    }
  }, [authSession.authenticated, authSession.login, hasAccess, isAdminRoute, loadUserAll, locationPath, request])

  const loadAuthorizedRepositories = useCallback(async (id: number) => {
    setLoadingRepositoriesFor(id)
    try {
      const data = (await request(
        `/user/github-app/installations/${encodeURIComponent(String(id))}/repositories`
      )) as AuthorizedRepositories
      setAuthorizedRepositories((current) => ({ ...current, [id]: data.repositories || [] }))
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to load GitHub repositories")
    } finally {
      setLoadingRepositoriesFor(null)
    }
  }, [request])

  const {
    runnerSpecOpen,
    runnerGroupOpen,
    runnerPolicyOpen,
    runnerSpecForm,
    runnerGroupForm,
    runnerPolicyForm,
    setRunnerSpecOpen,
    setRunnerGroupOpen,
    setRunnerPolicyOpen,
    setRunnerSpecForm,
    setRunnerGroupForm,
    setPolicyForm,
    groupNamesForSpec,
    resetRunnerSpecForm,
    resetRunnerGroupForm,
    createRunnerPolicy,
    saveRunnerSpec,
    loadRunnerSpecIntoForm,
    deleteRunnerSpec,
    saveRunnerGroup,
    loadRunnerGroupIntoForm,
    deleteRunnerGroup,
    savePolicy,
    loadPolicyIntoForm,
    deletePolicy,
  } = useRunnerCatalog({
    runnerSpecs,
    runnerGroups,
    setRunnerPolicies,
    request,
    loadAll,
    setSection,
    parseLabels,
  })

  useEffect(() => {
    void fetch("/healthz").catch(() => setConnected(false))
  }, [])

  useEffect(() => {
    void (async () => {
      try {
        const response = await fetch("/auth/session", { credentials: "same-origin" })
        if (response.ok) setAuthSession((await response.json()) as AuthSession)
      } catch {
        setAuthSession({ authenticated: false, oauth_enabled: false })
      }
    })()
  }, [])

  useEffect(() => {
    const handlePopState = () => {
      setLocationPath(window.location.pathname)
      setSectionState(sectionFromPath())
    }
    window.addEventListener("popstate", handlePopState)
    return () => window.removeEventListener("popstate", handlePopState)
  }, [])

  useEffect(() => {
    if (locationPath !== "/accounts" && locationPath !== "/settings") return
    const nextPath = `/account/repositories${window.location.search}`
    window.history.replaceState(null, "", nextPath)
    setLocationPath("/account/repositories")
  }, [locationPath])

  useEffect(() => {
    void loadAll()
    const timer = window.setInterval(() => void loadAll(), 5000)
    return () => window.clearInterval(timer)
  }, [loadAll])

  useEffect(() => {
    void loadUserAll()
    const timer = window.setInterval(() => void loadUserAll(), 5000)
    return () => window.clearInterval(timer)
  }, [loadUserAll])

  useEffect(() => {
    void syncGitHubAppSetupFromURL()
  }, [syncGitHubAppSetupFromURL])

  useEffect(() => {
    if (selectedID) void loadLog(selectedID, selectedLog)
  }, [loadLog, selectedID, selectedLog])

  useEffect(() => {
    if (section !== "diagnostics" || !hasAccess) return
    void (async () => {
      try {
        const [summary, vars] = await Promise.all([
          request("/diagnostics/pprof"),
          request("/diagnostics/vars").catch(() => ""),
        ])
        setDiagnostics(summary as DiagnosticsSummary)
        setDiagnosticsVars(typeof vars === "string" ? vars : JSON.stringify(vars, null, 2))
      } catch (error) {
        toast.error(error instanceof Error ? error.message : "Failed to load diagnostics")
      }
    })()
  }, [hasAccess, request, section])

  const signOut = () => {
    void fetch("/auth/logout", { method: "POST", credentials: "same-origin" }).finally(() => {
      setAuthSession((current) => ({ ...current, authenticated: false, login: undefined, role: undefined, avatar_url: undefined, expires_at: undefined }))
    })
    setRunners([])
    setRunnerSpecs([])
    setRunnerGroups([])
    setRunnerPolicies([])
    setAuditEvents([])
    setUserRunners([])
    setGitHubApp(null)
    setAuthorizedRepositories({})
    setLoadingRepositoriesFor(null)
    setUserSelectedKey("")
    setSelectedID("")
    setLogText("No runner selected")
  }

  const resetCreateRunnerForm = () => {
    setCreateID("")
    setCreateRepository("")
    setCreateRunnerSpec("")
    setCreateLabels("self-hosted,e2b")
  }

  const createRunner = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!hasAccess) {
      toast.error("Admin access required")
      return
    }
    const body: {
      id?: string
      repository_full_name?: string
      runner_spec_name?: string
      labels?: string[]
    } = {}
    const repository = createRepository.trim()
    if (!repository || repository.includes("*")) {
      toast.error("repository_full_name must be owner/repo")
      return
    }
    if (createID.trim()) body.id = createID.trim()
    body.repository_full_name = repository
    if (createRunnerSpec.trim()) body.runner_spec_name = createRunnerSpec.trim()
    const labels = parseLabels(createLabels)
    if (labels.length > 0) body.labels = labels
    try {
      const runner = (await request("/runner_requests", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      })) as RunnerState
      resetCreateRunnerForm()
      setCreateRunnerOpen(false)
      setSelectedID(runner.id)
      toast.success(`Runner ${runner.id} queued`)
      await loadAll()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to create runner")
    }
  }

  const stopRunner = async (id: string) => {
    try {
      const runner = (await request(`/runner_requests/${encodeURIComponent(id)}`, {
        method: "DELETE",
      })) as RunnerState
      setSelectedID(runner.id)
      toast.success(`Runner ${runner.id} completed`)
      await loadAll()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to stop runner")
    }
  }

  const retryRunner = async (id: string) => {
    try {
      const runner = (await request(`/runner_requests/${encodeURIComponent(id)}/retry`, {
        method: "POST",
      })) as RunnerState
      setSelectedID(runner.id)
      toast.success(`Runner ${runner.id} requeued`)
      await loadAll()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to retry runner")
    }
  }

  const runMatchTest = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    try {
      const result = (await request("/runner_specs/match", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          repository_full_name: matchRepository.trim(),
          labels: parseLabels(matchLabels),
        }),
      })) as RunnerSpecMatch
      setMatchResult(result)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to run match test")
    }
  }

  const copySelectedID = async () => {
    if (!selected) return
    await navigator.clipboard.writeText(selected.id)
    toast.success("Runner ID copied")
  }

  if (!authSession.authenticated || !authSession.oauth_enabled) {
    return (
      <>
        <LoginPage
          oauthEnabled={authSession.oauth_enabled}
          currentLogin={authSession.login}
          currentRole={authSession.role}
          onSignOut={signOut}
        />
        <Toaster richColors />
      </>
    )
  }

  if (!hasAccess || !isAdminRoute) {
    return (
      <>
        <UserDashboard
          authSession={authSession}
          githubApp={githubApp}
          runners={userRunners}
          selectedKey={userSelectedKey}
          page={userPage}
          accountSettingsRoute={accountSettingsRoute || defaultAccountSettingsRoute(authSession.login)}
          authorizedRepositories={authorizedRepositories}
          loadingRepositoriesFor={loadingRepositoriesFor}
          onLoadAuthorizedRepositories={(id) => void loadAuthorizedRepositories(id)}
          onNavigate={setUserPage}
          onNavigateAccountSettings={setAccountSettingsRoute}
          onSelectKey={setUserSelectedKey}
          onSignOut={signOut}
        />
        <Toaster richColors />
      </>
    )
  }

  return (
    <SidebarProvider>
      <AppSidebar
        section={section}
        connected={connected}
        activeCount={metrics[0]?.value || 0}
        authLabel={authSession.authenticated ? `@${authSession.login}` : "Locked"}
        onSectionChange={setSection}
        onSignOut={signOut}
      />
      <SidebarInset className="min-h-0 overflow-hidden">
        <SiteHeader />
        <main className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4 lg:gap-6 lg:p-6">
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            {metrics.map((metric) => (
              <Card key={metric.label} className="gap-3 py-5">
                <CardHeader className="px-5">
                  <CardDescription>{metric.label}</CardDescription>
                  <CardTitle className="text-3xl">{metric.value}</CardTitle>
                </CardHeader>
                <CardContent className="px-5 text-xs text-muted-foreground">
                  {metric.description}
                </CardContent>
              </Card>
            ))}
          </div>

          {section === "overview" ? (
            <OverviewSection
              runners={runners}
              runnerSpecs={runnerSpecs}
              runnerPolicies={runnerPolicies}
              onEditRunnerSpec={loadRunnerSpecIntoForm}
              onEditPolicy={loadPolicyIntoForm}
            />
          ) : null}

          {section === "runner_requests" ? (
            <RunnerRequestsSection
              hasAccess={hasAccess}
              loading={loading}
              runners={runners}
              filteredRunners={filteredRunners}
              selected={selected}
              selectedID={selectedID}
              selectedLog={selectedLog}
              logText={logText}
              createID={createID}
              createRepository={createRepository}
              createRunnerSpec={createRunnerSpec}
              createLabels={createLabels}
              createRunnerOpen={createRunnerOpen}
              runnerStatusFilter={runnerStatusFilter}
              runnerRepositoryFilter={runnerRepositoryFilter}
              runnerSpecFilter={runnerSpecFilter}
              runnerRepositories={runnerRepositories}
              runnerSpecNames={runnerSpecNames}
              onRefresh={() => void loadAll()}
              onResetCreateRunnerForm={resetCreateRunnerForm}
              onCreateRunnerOpenChange={setCreateRunnerOpen}
              onCreateRunnerSubmit={createRunner}
              onCreateIDChange={setCreateID}
              onCreateRepositoryChange={setCreateRepository}
              onCreateRunnerSpecChange={setCreateRunnerSpec}
              onCreateLabelsChange={setCreateLabels}
              onStatusFilterChange={setRunnerStatusFilter}
              onRepositoryFilterChange={setRunnerRepositoryFilter}
              onRunnerSpecFilterChange={setRunnerSpecFilter}
              onSelectRunner={setSelectedID}
              onRetryRunner={(id) => void retryRunner(id)}
              onStopRunner={(id) => void stopRunner(id)}
              onCopySelectedID={() => void copySelectedID()}
              onLoadLog={(id, name) => void loadLog(id, name)}
              onSelectedLogChange={setSelectedLog}
            />
          ) : null}

          {section === "runner_specs" ? (
            <RunnerSpecsSection
              loading={loading}
              runnerSpecs={runnerSpecs}
              runnerGroups={runnerGroups}
              runnerSpecOpen={runnerSpecOpen}
              runnerSpecForm={runnerSpecForm}
              onRefresh={() => void loadAll()}
              onResetRunnerSpecForm={resetRunnerSpecForm}
              onRunnerSpecOpenChange={setRunnerSpecOpen}
              onRunnerSpecFormChange={setRunnerSpecForm}
              onSubmitRunnerSpec={saveRunnerSpec}
              onEditRunnerSpec={loadRunnerSpecIntoForm}
              onDeleteRunnerSpec={(name) => void deleteRunnerSpec(name)}
              groupNamesForSpec={groupNamesForSpec}
            />
          ) : null}

          {section === "runner_groups" ? (
            <RunnerGroupsSection
              loading={loading}
              runnerGroups={runnerGroups}
              runnerSpecs={runnerSpecs}
              runnerGroupOpen={runnerGroupOpen}
              runnerGroupForm={runnerGroupForm}
              onRefresh={() => void loadAll()}
              onResetRunnerGroupForm={resetRunnerGroupForm}
              onRunnerGroupOpenChange={setRunnerGroupOpen}
              onRunnerGroupFormChange={setRunnerGroupForm}
              onSubmitRunnerGroup={saveRunnerGroup}
              onEditRunnerGroup={loadRunnerGroupIntoForm}
              onDeleteRunnerGroup={(name) => void deleteRunnerGroup(name)}
            />
          ) : null}

          {section === "runner_policies" ? (
            <RunnerPoliciesSection
              loading={loading}
              runnerPolicies={runnerPolicies}
              runnerSpecs={runnerSpecs}
              runnerGroups={runnerGroups}
              runnerPolicyOpen={runnerPolicyOpen}
              runnerPolicyForm={runnerPolicyForm}
              onRefresh={() => void loadAll()}
              onCreateRunnerPolicy={createRunnerPolicy}
              onRunnerPolicyOpenChange={setRunnerPolicyOpen}
              onRunnerPolicyFormChange={setPolicyForm}
              onSubmitRunnerPolicy={savePolicy}
              onEditRunnerPolicy={loadPolicyIntoForm}
              onDeleteRunnerPolicy={(id) => void deletePolicy(id)}
            />
          ) : null}

          {section === "match" ? (
            <MatchSection
              matchRepository={matchRepository}
              matchLabels={matchLabels}
              matchResult={matchResult}
              onRepositoryChange={setMatchRepository}
              onLabelsChange={setMatchLabels}
              onSubmit={runMatchTest}
            />
          ) : null}

          {section === "audit" ? <AuditSection auditEvents={auditEvents} /> : null}

          {section === "diagnostics" ? (
            <DiagnosticsSection diagnostics={diagnostics} diagnosticsVars={diagnosticsVars} />
          ) : null}
        </main>
      </SidebarInset>
      <Toaster richColors />
    </SidebarProvider>
  )
}

function defaultAccountSettingsRoute(currentLogin?: string): AccountSettingsRoute {
  return { accountLogin: currentLogin, tab: "repositories" }
}

function isAccountSettingsPath(path: string): boolean {
  return (
    path === "/settings" ||
    path === "/accounts" ||
    path === "/account/repositories" ||
    path === "/account/preferences" ||
    /^\/organizations\/[^/]+\/(repositories|preferences)$/.test(path)
  )
}

function parseAccountSettingsRoute(path: string, currentLogin?: string): AccountSettingsRoute | null {
  if (path === "/settings" || path === "/accounts") return defaultAccountSettingsRoute(currentLogin)
  if (path === "/account/repositories") return { accountLogin: currentLogin, tab: "repositories" }
  if (path === "/account/preferences") return { accountLogin: currentLogin, tab: "preferences" }

  const organizationMatch = path.match(/^\/organizations\/([^/]+)\/(repositories|preferences)$/)
  if (!organizationMatch) return null
  const accountLogin = safeDecodePathSegment(organizationMatch[1])
  if (!accountLogin) return null

  return {
    accountLogin,
    tab: organizationMatch[2] as AccountSettingsTab,
  }
}

function safeDecodePathSegment(value: string): string | null {
  try {
    return decodeURIComponent(value)
  } catch {
    return null
  }
}

function accountSettingsPathForInstallation(
  installation: Pick<GitHubInstallation, "account_login">,
  currentLogin: string | undefined,
  tab: AccountSettingsTab
): string {
  return accountSettingsPath(installation.account_login, currentLogin, tab)
}

function accountSettingsPath(
  accountLogin: string | undefined,
  currentLogin: string | undefined,
  tab: AccountSettingsTab
): string {
  const segment = tab === "preferences" ? "preferences" : "repositories"
  const login = accountLogin?.trim()
  if (!login || login === currentLogin) return `/account/${segment}`
  return `/organizations/${encodeURIComponent(login)}/${segment}`
}

export default App
