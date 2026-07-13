import { type ReactNode, useCallback, useEffect, useMemo, useState } from "react"
import { toast } from "sonner"

import { AppSidebar } from "@/components/app-sidebar"
import { AuditSection, DiagnosticsSection, MatchSection, OverviewSection } from "@/components/admin-sections"
import { LoginPage } from "@/components/login-page"
import { RunnerJobDetail } from "@/components/runner-job-detail"
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
  type UserPreferences,
} from "@/admin-types"
import { useRunnerCatalog } from "@/hooks/use-runner-catalog"

type AccountSettingsTab = "repositories" | "preferences" | "sandbox-templates" | "sandbox-instances"
type AccountSettingsRoute = {
  accountLogin?: string
  tab: AccountSettingsTab
}

function App() {
  const [authSession, setAuthSession] = useState<AuthSession>({ authenticated: false, oauth_enabled: false })
  const [locationPath, setLocationPath] = useState(() => window.location.pathname)
  const [locationSearch, setLocationSearch] = useState(() => window.location.search)
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
  const [userPreferences, setUserPreferences] = useState<UserPreferences | null>(null)
  const [authorizedRepositories, setAuthorizedRepositories] = useState<Record<number, string[]>>({})
  const [loadingRepositoriesFor, setLoadingRepositoriesFor] = useState<number | null>(null)
  const [userSelectedKey, setUserSelectedKey] = useState(() => userJobsGroupKeyFromLocation(window.location.pathname, window.location.search))

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
    const nextPath = next === "settings" ? "/account/repositories" : next === "repositories" ? "/repositories" : userJobsPath(userSelectedKey)
    if (window.location.pathname + window.location.search !== nextPath) {
      window.history.pushState(null, "", nextPath)
    }
    setLocationPath(window.location.pathname)
    setLocationSearch(window.location.search)
  }, [userSelectedKey])

  const setAccountSettingsRoute = useCallback(
    (accountLogin: string | undefined, tab: AccountSettingsTab) => {
      const nextPath = accountSettingsPath(accountLogin, authSession.login, tab)
      if (window.location.pathname !== nextPath) {
        window.history.pushState(null, "", nextPath)
      }
      setLocationPath(window.location.pathname)
      setLocationSearch(window.location.search)
    },
    [authSession.login]
  )

  const setUserJobsSelection = useCallback((key: string) => {
    setUserSelectedKey(key)
    const nextPath = userJobsPath(key)
    if (window.location.pathname + window.location.search !== nextPath) {
      window.history.pushState(null, "", nextPath)
    }
    setLocationPath(window.location.pathname)
    setLocationSearch(window.location.search)
  }, [])

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
  const userJobID = userJobIDFromPath(locationPath)
  const userSelectedJobID = userJobIDFromSearch(locationSearch)
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
      const nextRoute = parseAccountSettingsRoute(locationPath, authSession.login)
      const preferencesPath = userPreferencesPath(
        preferenceInstallationID(nextApp, nextRoute, authSession.login)
      )
      const preferencesData = await request(preferencesPath)
      setGitHubApp(nextApp)
      setUserRunners(nextRunners)
      setUserPreferences(preferencesData as UserPreferences)
      if (nextRunners.length === 0) setUserSelectedKey("")
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to load workspace data")
    } finally {
      setLoading(false)
    }
  }, [authSession.authenticated, authSession.login, hasAccess, isAdminRoute, locationPath, request])

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
      setLocationPath(window.location.pathname)
      setLocationSearch(window.location.search)
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

  const saveSandboxConfig = useCallback(async (
    apiURL: string,
    apiKey: string,
    installationID?: number,
    mode: "custom" | "inherit" = "custom",
    replaceInheritedSource = false,
  ) => {
    const preferences = (await request(userPreferencesPath(installationID, "/user/preferences/sandbox"), {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ mode, api_url: apiURL, api_key: apiKey, replace_inherited_source: replaceInheritedSource }),
    })) as UserPreferences
    setUserPreferences(preferences)
    toast.success("Sandbox service settings saved")
  }, [request])

  const deleteSandboxAPIKey = useCallback(async (installationID?: number) => {
    const preferences = (await request(userPreferencesPath(installationID, "/user/preferences/sandbox-api-key"), {
      method: "DELETE",
    })) as UserPreferences
    setUserPreferences(preferences)
    toast.success("Sandbox API Key removed")
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
      setLocationSearch(window.location.search)
      setUserSelectedKey(userJobsGroupKeyFromLocation(window.location.pathname, window.location.search))
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
    setLocationSearch(window.location.search)
  }, [locationPath])

  useEffect(() => {
    if (locationPath !== "/account/sandbox" && !/^\/organizations\/[^/]+\/sandbox$/.test(locationPath)) return
    const nextPath = locationPath.replace(/\/sandbox$/, "/sandbox-templates")
    window.history.replaceState(null, "", nextPath)
    setLocationPath(nextPath)
  }, [locationPath])

  useEffect(() => {
    if (!isUserJobsRoute(locationPath)) return
    const key = userJobsGroupKeyFromLocation(locationPath, locationSearch)
    setUserSelectedKey(key)
    const canonicalPath = userJobsPath(key)
    const currentLocation = `${locationPath}${locationSearch}`
    const nextPath = withPreservedJobSearch(canonicalPath, locationSearch)
    if (key && currentLocation !== nextPath) {
      window.history.replaceState(null, "", nextPath)
      setLocationPath(window.location.pathname)
      setLocationSearch(window.location.search)
    }
  }, [locationPath, locationSearch])

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

  const openUserJob = (id: string) => {
    const groupPath = userSelectedKey ? userJobsPath(userSelectedKey) : ""
    const nextPath = groupPath && groupPath !== "/" ? withSearchParam(groupPath, "job", id) : `/jobs/${encodeURIComponent(id)}`
    window.history.pushState(null, "", nextPath)
    setLocationPath(window.location.pathname)
    setLocationSearch(window.location.search)
  }

  const backToUserJobs = () => {
    const nextPath = userJobsPath(userSelectedKey)
    window.history.pushState(null, "", nextPath)
    setLocationPath(window.location.pathname)
    setLocationSearch(window.location.search)
  }

  const loadUserJobGroup = useCallback((key: string) => {
    const path = userJobGroupAPIPath(key)
    return path ? request(path) : Promise.resolve(null)
  }, [request])

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
    if (userJobID) {
      return (
        <>
          <UserJobRedirect
            id={userJobID}
            request={request}
            onResolved={(key) => {
              setUserSelectedKey(key)
              const nextPath = withSearchParam(userJobsPath(key), "job", userJobID)
              window.history.replaceState(null, "", nextPath)
              setLocationPath(window.location.pathname)
              setLocationSearch(window.location.search)
            }}
            fallback={
              <RunnerJobDetail
                id={userJobID}
                apiBase="/user/runner_requests"
                onBack={backToUserJobs}
                onOpenJob={openUserJob}
                request={request}
              />
            }
          />
          <Toaster richColors />
        </>
      )
    }
    return (
      <>
        <UserDashboard
          authSession={authSession}
          githubApp={githubApp}
          userPreferences={userPreferences}
          runners={userRunners}
          selectedKey={userSelectedKey}
          selectedJobID={userSelectedJobID}
          page={userPage}
          accountSettingsRoute={accountSettingsRoute || defaultAccountSettingsRoute(authSession.login)}
          authorizedRepositories={authorizedRepositories}
          loadingRepositoriesFor={loadingRepositoriesFor}
          onLoadAuthorizedRepositories={(id) => void loadAuthorizedRepositories(id)}
          onSaveSandboxConfig={saveSandboxConfig}
          onDeleteSandboxAPIKey={deleteSandboxAPIKey}
          onNavigate={setUserPage}
          onNavigateAccountSettings={setAccountSettingsRoute}
          onOpenJob={openUserJob}
          onLoadJobGroup={loadUserJobGroup}
          request={request}
          onSelectKey={setUserJobsSelection}
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

function UserJobRedirect({
  id,
  request,
  onResolved,
  fallback,
}: {
  id: string
  request: (url: string, options?: RequestInit) => Promise<unknown>
  onResolved: (key: string) => void
  fallback: ReactNode
}) {
  const [failedID, setFailedID] = useState("")

  useEffect(() => {
    let cancelled = false
    void request(`/user/runner_requests/${encodeURIComponent(id)}/group`)
      .then((group) => {
        if (cancelled) return
        const key = isRunnerJobGroupResponse(group) ? group.key : ""
        if (key) {
          onResolved(key)
          return
        }
        setFailedID(id)
      })
      .catch(() => {
        if (!cancelled) setFailedID(id)
      })
    return () => {
      cancelled = true
    }
  }, [id, onResolved, request])

  if (failedID === id) return <>{fallback}</>
  return (
    <div className="flex min-h-screen items-center justify-center bg-background text-sm text-muted-foreground">
      Opening job in its build context...
    </div>
  )
}

function defaultAccountSettingsRoute(currentLogin?: string): AccountSettingsRoute {
  return { accountLogin: currentLogin, tab: "repositories" }
}

function userPreferencesPath(installationID?: number, base = "/user/preferences") {
  if (!installationID) return base
  return `${base}?installation_id=${encodeURIComponent(String(installationID))}`
}

function preferenceInstallationID(
  githubApp: GitHubAppConfig,
  route: AccountSettingsRoute | null,
  currentLogin: string | undefined
) {
  const accountLogin = route?.accountLogin?.trim()
  if (!accountLogin || accountLogin === currentLogin) return undefined
  const installation = githubApp.installations.find((item) => item.account_login === accountLogin)
  return installation?.id
}

function isAccountSettingsPath(path: string): boolean {
  return (
    path === "/settings" ||
    path === "/accounts" ||
    path === "/account/repositories" ||
    path === "/account/preferences" ||
    path === "/account/sandbox" ||
    path === "/account/sandbox-templates" ||
    path === "/account/sandbox-instances" ||
    /^\/organizations\/[^/]+\/(repositories|preferences|sandbox|sandbox-templates|sandbox-instances)$/.test(path)
  )
}

function parseAccountSettingsRoute(path: string, currentLogin?: string): AccountSettingsRoute | null {
  if (path === "/settings" || path === "/accounts") return defaultAccountSettingsRoute(currentLogin)
  if (path === "/account/repositories") return { accountLogin: currentLogin, tab: "repositories" }
  if (path === "/account/preferences") return { accountLogin: currentLogin, tab: "preferences" }
  if (path === "/account/sandbox" || path === "/account/sandbox-templates") return { accountLogin: currentLogin, tab: "sandbox-templates" }
  if (path === "/account/sandbox-instances") return { accountLogin: currentLogin, tab: "sandbox-instances" }

  const organizationMatch = path.match(/^\/organizations\/([^/]+)\/(repositories|preferences|sandbox|sandbox-templates|sandbox-instances)$/)
  if (!organizationMatch) return null
  const accountLogin = safeDecodePathSegment(organizationMatch[1])
  if (!accountLogin) return null

  return {
    accountLogin,
    tab: (organizationMatch[2] === "sandbox" ? "sandbox-templates" : organizationMatch[2]) as AccountSettingsTab,
  }
}

function safeDecodePathSegment(value: string): string | null {
  try {
    return decodeURIComponent(value)
  } catch {
    return null
  }
}

function userJobIDFromPath(path: string) {
  const match = path.match(/^\/jobs\/([^/]+)$/)
  return match ? safeDecodePathSegment(match[1]) || "" : ""
}

function userJobIDFromSearch(search: string) {
  return new URLSearchParams(search).get("job") || ""
}

function isRunnerJobGroupResponse(value: unknown): value is { key: string } {
  return Boolean(value && typeof value === "object" && typeof (value as { key?: unknown }).key === "string")
}

function isUserJobsRoute(path: string) {
  return path === "/" || Boolean(userJobsGroupKeyFromPath(path))
}

function userJobsGroupKeyFromLocation(path: string, search: string) {
  const pathKey = userJobsGroupKeyFromPath(path, search)
  if (pathKey) return pathKey
  if (path !== "/") return ""
  return new URLSearchParams(search).get("group") || ""
}

function userJobsGroupKeyFromPath(path: string, search = "") {
  const pullRequestMatch = path.match(/^\/github\/pulls\/([^/]+)\/([^/]+)\/(\d+)\/jobs$/)
  if (pullRequestMatch) {
    return `pr:${decodeRepositoryPath(pullRequestMatch[1], pullRequestMatch[2])}:${pullRequestMatch[3]}`
  }

  const legacyPullRequestMatch = path.match(/^\/jobs\/pulls\/([^/]+)\/([^/]+)\/(\d+)$/)
  if (legacyPullRequestMatch) {
    return `pr:${decodeRepositoryPath(legacyPullRequestMatch[1], legacyPullRequestMatch[2])}:${legacyPullRequestMatch[3]}`
  }

  const runMatch = path.match(/^\/github\/runs\/([^/]+)\/([^/]+)\/(\d+)\/jobs$/)
  if (runMatch) {
    return `run:${decodeRepositoryPath(runMatch[1], runMatch[2])}:${runMatch[3]}`
  }

  const legacyRunMatch = path.match(/^\/jobs\/runs\/([^/]+)\/([^/]+)\/(\d+)$/)
  if (legacyRunMatch) {
    return `run:${decodeRepositoryPath(legacyRunMatch[1], legacyRunMatch[2])}:${legacyRunMatch[3]}`
  }

  const branchMatch = path.match(/^\/github\/branches\/([^/]+)\/([^/]+)\/(.+)\/([^/]+)\/jobs$/)
  if (branchMatch) {
    const repository = decodeRepositoryPath(branchMatch[1], branchMatch[2])
    const branch = safeDecodePathSegment(branchMatch[3])
    const sha = safeDecodePathSegment(branchMatch[4])
    if (!branch || !sha) return ""
    return `branch:${repository}:${branch}:${sha}`
  }

  const branchQueryMatch = path.match(/^\/github\/branches\/([^/]+)\/([^/]+)\/([^/]+)\/jobs$/)
  if (branchQueryMatch) {
    const repository = decodeRepositoryPath(branchQueryMatch[1], branchQueryMatch[2])
    const sha = safeDecodePathSegment(branchQueryMatch[3])
    const branch = new URLSearchParams(search).get("branch") || ""
    if (!branch || !sha) return ""
    return `branch:${repository}:${branch}:${sha}`
  }

  const legacyBranchMatch = path.match(/^\/jobs\/branches\/([^/]+)\/([^/]+)\/(.+)\/([^/]+)$/)
  if (legacyBranchMatch) {
    const repository = decodeRepositoryPath(legacyBranchMatch[1], legacyBranchMatch[2])
    const branch = safeDecodePathSegment(legacyBranchMatch[3])
    const sha = safeDecodePathSegment(legacyBranchMatch[4])
    if (!branch || !sha) return ""
    return `branch:${repository}:${branch}:${sha}`
  }

  const manualMatch = path.match(/^\/jobs\/manual\/([^/]+)\/([^/]+)\/([^/]+)$/)
  if (manualMatch) {
    const repository = decodeRepositoryPath(manualMatch[1], manualMatch[2])
    const id = safeDecodePathSegment(manualMatch[3])
    if (!id) return ""
    return `manual:${repository}:${id}`
  }

  return ""
}

function userJobsPath(groupKey: string) {
  if (!groupKey) return "/"
  const pullRequestMatch = groupKey.match(/^pr:(.+):(\d+)$/)
  if (pullRequestMatch) return `/github/pulls/${encodeRepositoryPath(pullRequestMatch[1])}/${pullRequestMatch[2]}/jobs`

  const runMatch = groupKey.match(/^run:(.+):(\d+)$/)
  if (runMatch) return `/github/runs/${encodeRepositoryPath(runMatch[1])}/${runMatch[2]}/jobs`

  const branchMatch = groupKey.match(/^branch:([^:]+):(.+):([^:]+)$/)
  if (branchMatch) {
    return withSearchParam(`/github/branches/${encodeRepositoryPath(branchMatch[1])}/${encodeURIComponent(branchMatch[3])}/jobs`, "branch", branchMatch[2])
  }

  const manualMatch = groupKey.match(/^manual:(.+):([^:]+)$/)
  if (manualMatch) return `/jobs/manual/${encodeRepositoryPath(manualMatch[1])}/${encodeURIComponent(manualMatch[2])}`

  return `/?group=${encodeURIComponent(groupKey)}`
}

function userJobGroupAPIPath(groupKey: string) {
  if (!groupKey) return ""
  const pullRequestMatch = groupKey.match(/^pr:(.+):(\d+)$/)
  if (pullRequestMatch) return `/user/github/pulls/${encodeRepositoryPath(pullRequestMatch[1])}/${pullRequestMatch[2]}/jobs`

  const runMatch = groupKey.match(/^run:(.+):(\d+)$/)
  if (runMatch) return `/user/github/runs/${encodeRepositoryPath(runMatch[1])}/${runMatch[2]}/jobs`

  const branchMatch = groupKey.match(/^branch:([^:]+):(.+):([^:]+)$/)
  if (branchMatch) {
    return withSearchParam(`/user/github/branches/${encodeRepositoryPath(branchMatch[1])}/${encodeURIComponent(branchMatch[3])}/jobs`, "branch", branchMatch[2])
  }

  return ""
}

function encodeRepositoryPath(repository: string) {
  return repository.split("/").map((segment) => encodeURIComponent(segment)).join("/")
}

function withSearchParam(path: string, key: string, value: string) {
  const [pathname, search = ""] = path.split("?")
  const params = new URLSearchParams(search)
  params.set(key, value)
  const query = params.toString()
  return query ? `${pathname}?${query}` : pathname
}

function withPreservedJobSearch(path: string, search: string) {
  const job = new URLSearchParams(search).get("job")
  return job ? withSearchParam(path, "job", job) : path
}

function decodeRepositoryPath(ownerSegment: string, repoSegment: string) {
  const owner = safeDecodePathSegment(ownerSegment)
  const repo = safeDecodePathSegment(repoSegment)
  if (!owner || !repo) return "unknown/repository"
  return `${owner}/${repo}`
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
  const segment = tab === "preferences" ? "preferences" : tab === "sandbox-templates" ? "sandbox-templates" : tab === "sandbox-instances" ? "sandbox-instances" : "repositories"
  const login = accountLogin?.trim()
  if (!login || login === currentLogin) return `/account/${segment}`
  return `/organizations/${encodeURIComponent(login)}/${segment}`
}

export default App
