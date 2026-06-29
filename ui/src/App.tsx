import { useCallback, useEffect, useMemo, useState } from "react"
import { toast } from "sonner"

import { AppSidebar } from "@/components/app-sidebar"
import { AuditSection, DiagnosticsSection, MatchSection, OverviewSection } from "@/components/admin-sections"
import { LoginPage } from "@/components/login-page"
import { RunnerGroupsSection, type RunnerGroupFormState } from "@/components/runner-groups-section"
import { RunnerPoliciesSection, type RunnerPolicyFormState } from "@/components/runner-policies-section"
import { RunnerRequestsSection } from "@/components/runner-requests-section"
import { RunnerSpecsSection, type RunnerSpecFormState } from "@/components/runner-specs-section"
import { SiteHeader } from "@/components/site-header"
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
  type DiagnosticsSummary,
  type Metric,
  type RunnerGroup,
  type RunnerPolicy,
  type RunnerSpec,
  type RunnerSpecMatch,
  type RunnerState,
  type RunnerStatus,
} from "@/admin-types"

function App() {
  const [authSession, setAuthSession] = useState<AuthSession>({ authenticated: false, oauth_enabled: false })
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
  const [runnerSpecOpen, setRunnerSpecOpen] = useState(false)
  const [runnerGroupOpen, setRunnerGroupOpen] = useState(false)
  const [runnerPolicyOpen, setRunnerPolicyOpen] = useState(false)
  const [runnerSpecForm, setRunnerSpecForm] = useState<RunnerSpecFormState>({
    name: "",
    labels: "self-hosted,e2b",
    template_id: "",
    runner_group: "",
    group_names: [] as string[],
    max_concurrency: "10",
    min_idle: "0",
    priority: "0",
    enabled: true,
    default_available: true,
  })
  const [runnerPolicyForm, setPolicyForm] = useState<RunnerPolicyFormState>({
    id: 0,
    repository_full_name: "",
    target_type: "spec",
    runner_spec_name: "",
    runner_group_name: "",
    enabled: true,
  })
  const [runnerGroupForm, setRunnerGroupForm] = useState<RunnerGroupFormState>({
    name: "",
    description: "",
    spec_names: [] as string[],
    enabled: true,
  })
  const [matchRepository, setMatchRepository] = useState("")
  const [matchLabels, setMatchLabels] = useState("self-hosted,e2b")
  const [matchResult, setMatchResult] = useState<RunnerSpecMatch | null>(null)
  const [diagnostics, setDiagnostics] = useState<DiagnosticsSummary | null>(null)
  const [diagnosticsVars, setDiagnosticsVars] = useState("")
  const [auditEvents, setAuditEvents] = useState<AuditEvent[]>([])

  const setSection = useCallback((next: string) => {
    const section = adminSections.includes(next as AdminSection) ? (next as AdminSection) : "overview"
    setSectionState(section)
    const nextPath = section === "overview" ? "/admin/" : `/admin/${section}`
    if (window.location.pathname !== nextPath) {
      window.history.pushState(null, "", nextPath)
    }
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

  const groupNamesForSpec = useCallback(
    (specName: string) =>
      runnerGroups
        .filter((group) => group.spec_names.includes(specName))
        .map((group) => group.name),
    [runnerGroups]
  )

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
        throw new Error("You do not have admin access")
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
    const handlePopState = () => setSectionState(sectionFromPath())
    window.addEventListener("popstate", handlePopState)
    return () => window.removeEventListener("popstate", handlePopState)
  }, [])

  useEffect(() => {
    void loadAll()
    const timer = window.setInterval(() => void loadAll(), 5000)
    return () => window.clearInterval(timer)
  }, [loadAll])

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
    setSelectedID("")
    setLogText("No runner selected")
  }

  const resetCreateRunnerForm = () => {
    setCreateID("")
    setCreateRepository("")
    setCreateRunnerSpec("")
    setCreateLabels("self-hosted,e2b")
  }

  const resetRunnerSpecForm = () => {
    setRunnerSpecForm({
      name: "",
      labels: "self-hosted,e2b",
      template_id: "",
      runner_group: "",
      group_names: [],
      max_concurrency: "10",
      min_idle: "0",
      priority: "0",
      enabled: true,
      default_available: true,
    })
  }

  const resetRunnerPolicyForm = () => {
    setPolicyForm({ id: 0, repository_full_name: "", target_type: "spec", runner_spec_name: "", runner_group_name: "", enabled: true })
  }

  const resetRunnerGroupForm = () => {
    setRunnerGroupForm({ name: "", description: "", spec_names: [], enabled: true })
  }

  const createRunnerPolicy = () => {
    if (runnerSpecs.length === 0 && runnerGroups.length === 0) {
      toast.error("Create a runner spec or runner group before adding policies")
      setSection("runner_specs")
      return
    }
    setPolicyForm({
      id: 0,
      repository_full_name: "",
      target_type: runnerGroups.length > 0 ? "group" : "spec",
      runner_spec_name: runnerSpecs[0]?.name || "",
      runner_group_name: runnerGroups[0]?.name || "",
      enabled: true,
    })
    setRunnerPolicyOpen(true)
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

  const saveRunnerSpec = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    try {
      const payload = {
        name: runnerSpecForm.name.trim(),
        labels: parseLabels(runnerSpecForm.labels),
        template_id: runnerSpecForm.template_id.trim(),
        runner_group: runnerSpecForm.runner_group.trim(),
        max_concurrency: Number(runnerSpecForm.max_concurrency) || 0,
        min_idle: Number(runnerSpecForm.min_idle) || 0,
        priority: Number(runnerSpecForm.priority) || 0,
        enabled: runnerSpecForm.enabled,
        default_available: runnerSpecForm.default_available,
      }
      const isUpdate = runnerSpecs.some((runnerSpec) => runnerSpec.name === payload.name)
      const url = isUpdate ? `/runner_specs/${encodeURIComponent(payload.name)}` : "/runner_specs"
      const method = isUpdate ? "PATCH" : "POST"
      await request(url, {
        method,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      })
      await Promise.all(
        runnerGroups.map((group) => {
          const shouldContain = runnerSpecForm.group_names.includes(group.name)
          const currentSpecs = new Set(group.spec_names)
          if (shouldContain) currentSpecs.add(payload.name)
          else currentSpecs.delete(payload.name)
          return request(`/runner_groups/${encodeURIComponent(group.name)}`, {
            method: "PATCH",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              spec_names: Array.from(currentSpecs).sort(),
              enabled: group.enabled,
            }),
          })
        })
      )
      toast.success(`Runner spec ${payload.name} saved`)
      setRunnerSpecOpen(false)
      await loadAll()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to save runner spec")
    }
  }

  const loadRunnerSpecIntoForm = (runnerSpec: RunnerSpec) => {
    setSection("runner_specs")
    setRunnerSpecForm({
      name: runnerSpec.name,
      labels: runnerSpec.labels.join(","),
      template_id: runnerSpec.template_id,
      runner_group: runnerSpec.runner_group || "",
      group_names: runnerGroups
        .filter((group) => group.spec_names.includes(runnerSpec.name))
        .map((group) => group.name),
      max_concurrency: String(runnerSpec.max_concurrency),
      min_idle: String(runnerSpec.min_idle),
      priority: String(runnerSpec.priority),
      enabled: runnerSpec.enabled,
      default_available: runnerSpec.default_available,
    })
    setRunnerSpecOpen(true)
  }

  const deleteRunnerSpec = async (name: string) => {
    try {
      await request(`/runner_specs/${encodeURIComponent(name)}`, { method: "DELETE" })
      toast.success(`Runner spec ${name} deleted`)
      if (runnerSpecForm.name === name) {
        resetRunnerSpecForm()
      }
      await loadAll()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to delete runner spec")
    }
  }

  const saveRunnerGroup = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    try {
      const payload = {
        name: runnerGroupForm.name.trim(),
        description: runnerGroupForm.description.trim(),
        spec_names: runnerGroupForm.spec_names,
        enabled: runnerGroupForm.enabled,
      }
      const isUpdate = runnerGroups.some((group) => group.name === payload.name)
      const url = isUpdate ? `/runner_groups/${encodeURIComponent(payload.name)}` : "/runner_groups"
      const method = isUpdate ? "PATCH" : "POST"
      await request(url, {
        method,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      })
      toast.success(`Runner group ${payload.name} saved`)
      setRunnerGroupOpen(false)
      await loadAll()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to save runner group")
    }
  }

  const loadRunnerGroupIntoForm = (group: RunnerGroup) => {
    setSection("runner_groups")
    setRunnerGroupForm({
      name: group.name,
      description: group.description || "",
      spec_names: [...group.spec_names],
      enabled: group.enabled,
    })
    setRunnerGroupOpen(true)
  }

  const deleteRunnerGroup = async (name: string) => {
    try {
      await request(`/runner_groups/${encodeURIComponent(name)}`, { method: "DELETE" })
      toast.success(`Runner group ${name} deleted`)
      if (runnerGroupForm.name === name) resetRunnerGroupForm()
      await loadAll()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to delete runner group")
    }
  }

  const savePolicy = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    try {
      const payload: {
        repository_full_name: string
        runner_spec_name?: string
        runner_group_name?: string
        enabled: boolean
      } = {
        repository_full_name: runnerPolicyForm.repository_full_name.trim(),
        enabled: runnerPolicyForm.enabled,
      }
      if (runnerPolicyForm.target_type === "group") payload.runner_group_name = runnerPolicyForm.runner_group_name.trim()
      else payload.runner_spec_name = runnerPolicyForm.runner_spec_name.trim()
      const isUpdate = runnerPolicyForm.id > 0
      const url = isUpdate ? `/runner_policies/${runnerPolicyForm.id}` : "/runner_policies"
      const method = isUpdate ? "PATCH" : "POST"
      const saved = (await request(url, {
        method,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      })) as RunnerPolicy
      setRunnerPolicies((current) => {
        const index = current.findIndex((policy) => policy.id === saved.id)
        if (index === -1) return [saved, ...current]
        const next = [...current]
        next[index] = saved
        return next
      })
      toast.success(`Runner policy #${saved.id} saved`)
      setRunnerPolicyOpen(false)
      await loadAll()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to save runner policy")
    }
  }

  const loadPolicyIntoForm = (policy: RunnerPolicy) => {
    setSection("runner_policies")
    setPolicyForm({
      id: policy.id,
      repository_full_name: policy.repository_full_name,
      target_type: policy.runner_group_name ? "group" : "spec",
      runner_spec_name: policy.runner_spec_name || "",
      runner_group_name: policy.runner_group_name || "",
      enabled: policy.enabled,
    })
    setRunnerPolicyOpen(true)
  }

  const deletePolicy = async (id: number) => {
    try {
      await request(`/runner_policies/${id}`, { method: "DELETE" })
      toast.success("Runner policy deleted")
      if (runnerPolicyForm.id === id) {
        resetRunnerPolicyForm()
      }
      await loadAll()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to delete runner policy")
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

  if (!hasAccess) {
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

export default App
