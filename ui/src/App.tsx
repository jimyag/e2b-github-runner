import { type ReactNode, useCallback, useEffect, useMemo, useState } from "react"
import {
  AlertCircle,
  CheckCircle2,
  Clock3,
  Copy,
  ExternalLink,
  Github,
  Loader2,
  Play,
  Plus,
  RefreshCw,
  ShieldCheck,
  Square,
  Trash2,
} from "lucide-react"
import { toast } from "sonner"

import { AppSidebar } from "@/components/app-sidebar"
import { SiteHeader } from "@/components/site-header"
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
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Toaster } from "@/components/ui/sonner"
import { cn } from "@/lib/utils"

type RunnerStatus = "queued" | "creating" | "running" | "stopping" | "completed" | "failed"

type RunnerState = {
  id: string
  status: RunnerStatus
  repository_full_name?: string
  requested_labels?: string[]
  runner_spec_name?: string
  runner_group?: string
  runner_name: string
  sandbox_id?: string
  process_pid?: number
  workflow_job_id?: number
  workflow_run_id?: number
  github_job_url?: string
  pull_request_number?: number
  assigned_job_id?: number
  assigned_job_name?: string
  error?: string
  failure_stage?: string
  failure_reason?: string
  last_error_code?: string
  last_error_message?: string
  last_error_retryable?: boolean
  retry_count?: number
  updated_at: string
  created_at: string
  next_retry_at?: string
  completed_at?: string
}

type RunnerSpec = {
  name: string
  labels: string[]
  template_id: string
  runner_group?: string
  max_concurrency: number
  min_idle: number
  priority: number
  enabled: boolean
  default_available: boolean
  created_at: string
  updated_at: string
}

type RunnerPolicy = {
  id: number
  repository_full_name: string
  runner_spec_name?: string
  runner_group_name?: string
  enabled: boolean
  created_at: string
}

type RunnerGroup = {
  name: string
  description?: string
  spec_names: string[]
  enabled: boolean
  created_at: string
  updated_at: string
}

type RunnerSpecMatch = {
  repository_full_name: string
  labels: string[]
  runner_spec?: RunnerSpec
  reason?: string
}

type DiagnosticsSummary = {
  pprof: Array<{ address: string; address_file: string; dump_script: string }>
  state: { backend: string; database: string }
  github: { auth_mode: string; installation_id?: number; api_base_url: string }
  sandbox: { api_url: string }
  recent_failures: RunnerState[]
}

type AuditEvent = {
  id: number
  actor: string
  action: string
  resource_type: string
  resource_id: string
  payload_json?: string
  created_at: string
}

type AuthSession = {
  authenticated: boolean
  oauth_enabled: boolean
  login?: string
  role?: string
  avatar_url?: string
  expires_at?: string
}

type Metric = {
  label: string
  value: number
  description: string
}

const activeStatuses = new Set<RunnerStatus>(["queued", "creating", "running", "stopping"])
const logNames = ["control.log", "stdout.log", "stderr.log"] as const
const adminSections = [
  "overview",
  "runner_requests",
  "runner_specs",
  "runner_groups",
  "runner_policies",
  "match",
  "audit",
  "diagnostics",
] as const

type AdminSection = (typeof adminSections)[number]

function sectionFromPath(): AdminSection {
  const slug = window.location.pathname.replace(/^\/admin\/?/, "") || "overview"
  return adminSections.includes(slug as AdminSection) ? (slug as AdminSection) : "overview"
}

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
  const [runnerSpecForm, setRunnerSpecForm] = useState({
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
  const [runnerPolicyForm, setPolicyForm] = useState({
    id: 0,
    repository_full_name: "",
    target_type: "spec",
    runner_spec_name: "",
    runner_group_name: "",
    enabled: true,
  })
  const [runnerGroupForm, setRunnerGroupForm] = useState({
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
            <div className="grid gap-4 xl:grid-cols-2">
              <Card>
                <CardHeader>
                  <CardTitle>Recent runner requests</CardTitle>
                  <CardDescription>Newest requests and their matched runner specs.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  {runners.slice(0, 8).map((runner) => (
                    <div key={runner.id} className="flex items-center justify-between gap-3 rounded-md border p-3">
                      <div className="min-w-0">
                        <div className="truncate font-medium">{runner.repository_full_name || runner.id}</div>
                        <div className="truncate text-xs text-muted-foreground">
                          {runner.runner_spec_name || "-"} · {runner.runner_name}
                        </div>
                      </div>
                      <StatusBadge status={runner.status} />
                    </div>
                  ))}
                  {runners.length === 0 ? (
                    <div className="text-sm text-muted-foreground">No runner requests yet.</div>
                  ) : null}
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardTitle>Runner specs and runner policies</CardTitle>
                  <CardDescription>Current seeded and runtime-managed routing rules.</CardDescription>
                </CardHeader>
                <CardContent className="grid gap-4 lg:grid-cols-2">
                  <div className="space-y-3">
                    <div className="text-sm font-medium">Runner specs</div>
                    {runnerSpecs.map((runnerSpec) => (
                      <div
                        key={runnerSpec.name}
                        className="rounded-md border p-3 text-sm"
                        onClick={() => loadRunnerSpecIntoForm(runnerSpec)}
                      >
                        <div className="font-medium">{runnerSpec.name}</div>
                        <div className="text-xs text-muted-foreground">
                          {runnerSpec.labels.join(", ")} · template {runnerSpec.template_id}
                        </div>
                      </div>
                    ))}
                  </div>
                  <div className="space-y-3">
                    <div className="text-sm font-medium">Runner policies</div>
                    {runnerPolicies.map((policy) => (
                      <div
                        key={policy.id}
                        className="rounded-md border p-3 text-sm"
                        onClick={() => loadPolicyIntoForm(policy)}
                      >
                        <div className="font-medium">{policy.repository_full_name}</div>
                        <div className="text-xs text-muted-foreground">{policy.runner_spec_name}</div>
                      </div>
                    ))}
                  </div>
                </CardContent>
              </Card>
            </div>
          ) : null}

          {section === "runner_requests" ? (
            <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(520px,640px)]">
              <Card className="min-w-0 gap-0 py-0">
                <CardHeader className="border-b px-5 py-4">
                  <div className="flex flex-col gap-3">
                    <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                      <div>
                        <CardTitle>Runner Requests</CardTitle>
                        <CardDescription>
                          Webhook and manual requests with matched runner policy context.
                        </CardDescription>
                      </div>
                      <div className="flex gap-2">
                        <Button
                          type="button"
                          onClick={() => {
                            resetCreateRunnerForm()
                            setCreateRunnerOpen(true)
                          }}
                          disabled={!hasAccess}
                        >
                          <Plus />
                          Create
                        </Button>
                        <Button
                          type="button"
                          variant="outline"
                          size="icon"
                          onClick={() => void loadAll()}
                          disabled={loading}
                          title="Refresh"
                        >
                          <RefreshCw className={cn(loading && "animate-spin")} />
                        </Button>
                      </div>
                    </div>
                    <Dialog open={createRunnerOpen} onOpenChange={setCreateRunnerOpen}>
                      <DialogContent>
                        <DialogHeader>
                          <DialogTitle>Create runner request</DialogTitle>
                          <DialogDescription>
                            Manually enqueue a one-off runner request.
                          </DialogDescription>
                        </DialogHeader>
                        <form className="grid gap-3" onSubmit={createRunner}>
                          <Input
                            value={createID}
                            onChange={(event) => setCreateID(event.target.value)}
                            placeholder="optional id"
                          />
                          <Input
                            value={createRepository}
                            onChange={(event) => setCreateRepository(event.target.value)}
                            placeholder="owner/repo"
                            required
                          />
                          <Input
                            value={createRunnerSpec}
                            onChange={(event) => setCreateRunnerSpec(event.target.value)}
                            placeholder="optional runner spec"
                          />
                          <Input
                            value={createLabels}
                            onChange={(event) => setCreateLabels(event.target.value)}
                            placeholder="self-hosted,e2b"
                          />
                          <DialogFooter>
                            <Button type="button" variant="outline" onClick={() => setCreateRunnerOpen(false)}>
                              Cancel
                            </Button>
                            <Button type="submit" disabled={!hasAccess}>
                              Create
                            </Button>
                          </DialogFooter>
                        </form>
                      </DialogContent>
                    </Dialog>
                    <div className="grid gap-2 md:grid-cols-[minmax(160px,220px)_minmax(180px,1fr)_minmax(180px,1fr)]">
                      <Select value={runnerStatusFilter} onValueChange={(value) => setRunnerStatusFilter(value as RunnerStatus | "all")}>
                        <SelectTrigger>
                          <SelectValue placeholder="Status" />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="all">All statuses</SelectItem>
                          {(["queued", "creating", "running", "stopping", "completed", "failed"] as RunnerStatus[]).map((status) => (
                            <SelectItem key={status} value={status}>
                              {status}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <Select value={runnerRepositoryFilter} onValueChange={setRunnerRepositoryFilter}>
                        <SelectTrigger>
                          <SelectValue placeholder="Repository" />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="all">All repositories</SelectItem>
                          {runnerRepositories.map((repository) => (
                            <SelectItem key={repository} value={repository}>
                              {repository}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <Select value={runnerSpecFilter} onValueChange={setRunnerSpecFilter}>
                        <SelectTrigger>
                          <SelectValue placeholder="Runner spec" />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="all">All runner specs</SelectItem>
                          {runnerSpecNames.map((runnerSpecName) => (
                            <SelectItem key={runnerSpecName} value={runnerSpecName}>
                              {runnerSpecName}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="text-xs text-muted-foreground">
                      Showing {filteredRunners.length} of {runners.length} runner requests.
                    </div>
                  </div>
                </CardHeader>
                <CardContent className="max-h-[calc(100vh-18rem)] overflow-auto p-0">
                  <Table>
                    <TableHeader className="sticky top-0 z-10 bg-background">
                      <TableRow>
                        <TableHead>Status</TableHead>
                        <TableHead>Repository</TableHead>
                        <TableHead>Runner spec</TableHead>
                        <TableHead>Runner</TableHead>
                        <TableHead>Sandbox</TableHead>
                        <TableHead>GitHub</TableHead>
                        <TableHead>Updated</TableHead>
                        <TableHead className="w-36" />
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {filteredRunners.length === 0 ? (
                        <TableRow>
                          <TableCell colSpan={8} className="h-24 text-center text-muted-foreground">
                            No runner requests found
                          </TableCell>
                        </TableRow>
                      ) : (
                        filteredRunners.map((runner) => (
                          <TableRow
                            key={runner.id}
                            data-state={runner.id === selectedID ? "selected" : undefined}
                            className="cursor-pointer"
                            onClick={() => setSelectedID(runner.id)}
                          >
                            <TableCell>
                              <StatusBadge status={runner.status} />
                            </TableCell>
                            <TableCell className="max-w-[220px] truncate">
                              {runner.repository_full_name || "-"}
                            </TableCell>
                            <TableCell>{runner.runner_spec_name || "-"}</TableCell>
                            <TableCell>
                              <div className="font-medium">{runner.runner_name || runner.id}</div>
                              <div className="text-xs text-muted-foreground">{runner.id}</div>
                            </TableCell>
                            <TableCell className="max-w-[180px] truncate">
                              {runner.sandbox_id || "-"}
                            </TableCell>
                            <TableCell>
                              {runner.github_job_url ? (
                                <Button
                                  asChild
                                  type="button"
                                  variant="outline"
                                  size="sm"
                                  onClick={(event) => event.stopPropagation()}
                                >
                                  <a href={runner.github_job_url} target="_blank" rel="noreferrer">
                                    <ExternalLink />
                                    Job
                                  </a>
                                </Button>
                              ) : (
                                <span className="text-muted-foreground">-</span>
                              )}
                            </TableCell>
                            <TableCell>{formatTime(runner.updated_at)}</TableCell>
                            <TableCell>
                              <div className="flex gap-2">
                                {runner.status === "failed" ? (
                                  <Button
                                    type="button"
                                    variant="outline"
                                    size="sm"
                                    onClick={(event) => {
                                      event.stopPropagation()
                                      void retryRunner(runner.id)
                                    }}
                                  >
                                    <RefreshCw />
                                    Retry
                                  </Button>
                                ) : null}
                                {activeStatuses.has(runner.status) ? (
                                  <Button
                                    type="button"
                                    variant="outline"
                                    size="sm"
                                    onClick={(event) => {
                                      event.stopPropagation()
                                      void stopRunner(runner.id)
                                    }}
                                  >
                                    <Trash2 />
                                    Stop
                                  </Button>
                                ) : null}
                              </div>
                            </TableCell>
                          </TableRow>
                        ))
                      )}
                    </TableBody>
                  </Table>
                </CardContent>
              </Card>

              <Card className="min-w-0 gap-0 py-0">
                <CardHeader className="border-b px-5 py-4">
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <CardTitle>Request details</CardTitle>
                      <CardDescription>{selected?.runner_name || "Select a request"}</CardDescription>
                    </div>
                    <Button
                      type="button"
                      variant="outline"
                      size="icon"
                      onClick={() => void copySelectedID()}
                      disabled={!selected}
                      title="Copy runner ID"
                    >
                      <Copy />
                    </Button>
                  </div>
                </CardHeader>
                {selected ? (
                  <CardContent className="grid gap-5 p-5">
                    <div className="space-y-2">
                      <Detail label="ID" value={selected.id} />
                      <Detail label="Status" value={selected.status} />
                      <Detail label="Repository" value={selected.repository_full_name || "-"} />
                      <Detail label="Runner spec" value={selected.runner_spec_name || "-"} />
                      <Detail label="Sandbox" value={selected.sandbox_id || "-"} />
                      <Detail label="PID" value={selected.process_pid || "-"} />
                      <Detail
                        label="Job"
                        value={selected.assigned_job_name || selected.assigned_job_id || "-"}
                      />
                      <Detail
                        label="GitHub job"
                        value={
                          selected.github_job_url ? (
                            <a
                              className="inline-flex items-center gap-1 text-primary underline-offset-4 hover:underline"
                              href={selected.github_job_url}
                              target="_blank"
                              rel="noreferrer"
                            >
                              Open job
                              <ExternalLink className="size-3.5" />
                            </a>
                          ) : (
                            "-"
                          )
                        }
                      />
                      <Detail label="Workflow run" value={selected.workflow_run_id || "-"} />
                      <Detail label="Pull request" value={selected.pull_request_number || "-"} />
                      <Detail label="Created" value={formatTime(selected.created_at)} />
                      <Detail label="Updated" value={formatTime(selected.updated_at)} />
                      <Detail label="Completed" value={formatTime(selected.completed_at)} />
                      <Detail label="Retry count" value={selected.retry_count || "-"} />
                      <Detail label="Next retry" value={formatTime(selected.next_retry_at)} />
                      <Detail label="Requested labels" value={selected.requested_labels?.join(", ") || "-"} />
                      <Detail label="Failure" value={selected.failure_reason || "-"} />
                      <Detail label="Last error code" value={selected.last_error_code || "-"} />
                      <Detail label="Error" value={selected.error || "-"} />
                    </div>
                    {selected.status === "failed" ? (
                      <Button type="button" variant="outline" onClick={() => void retryRunner(selected.id)}>
                        <RefreshCw />
                        Retry request
                      </Button>
                    ) : null}
                    <div className="space-y-3">
                      <div className="flex items-center justify-between gap-3">
                        <div>
                          <div className="text-sm font-medium">Logs</div>
                          <div className="text-xs text-muted-foreground">control, stdout, and stderr captured by runnerd.</div>
                        </div>
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={() => void loadLog(selected.id, selectedLog)}
                        >
                          <RefreshCw />
                          Refresh
                        </Button>
                      </div>
                      <Tabs
                        value={selectedLog}
                        onValueChange={(value) => setSelectedLog(value as (typeof logNames)[number])}
                      >
                        <TabsList>
                          {logNames.map((name) => (
                            <TabsTrigger key={name} value={name}>
                              {name.replace(".log", "")}
                            </TabsTrigger>
                          ))}
                        </TabsList>
                      </Tabs>
                      <pre className="max-h-[52vh] min-h-80 overflow-auto rounded-lg border bg-muted/50 p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap">
                        {logText}
                      </pre>
                    </div>
                  </CardContent>
                ) : (
                  <CardContent className="p-8 text-sm text-muted-foreground">
                    No runner request selected
                  </CardContent>
                )}
              </Card>
            </div>
          ) : null}

          {section === "runner_specs" ? (
            <div className="grid gap-4">
              <Card className="min-w-0">
                <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div>
                    <CardTitle>Runner specs</CardTitle>
                    <CardDescription>Click a runner spec row to edit it.</CardDescription>
                  </div>
                  <div className="flex gap-2">
                    <Button
                      type="button"
                      onClick={() => {
                        resetRunnerSpecForm()
                        setRunnerSpecOpen(true)
                      }}
                    >
                      <Plus />
                      Create
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      size="icon"
                      onClick={() => void loadAll()}
                      disabled={loading}
                      title="Refresh"
                    >
                      <RefreshCw className={cn(loading && "animate-spin")} />
                    </Button>
                  </div>
                </CardHeader>
                <CardContent className="p-0">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Name</TableHead>
                        <TableHead>Labels</TableHead>
                        <TableHead>Template</TableHead>
                        <TableHead>GitHub group</TableHead>
                        <TableHead>Runner groups</TableHead>
                        <TableHead>Default</TableHead>
                        <TableHead>Limit</TableHead>
                        <TableHead className="w-24" />
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {runnerSpecs.map((runnerSpec) => (
                        <TableRow key={runnerSpec.name} className="cursor-pointer" onClick={() => loadRunnerSpecIntoForm(runnerSpec)}>
                          <TableCell>{runnerSpec.name}</TableCell>
                          <TableCell className="max-w-[260px] truncate">{runnerSpec.labels.join(", ")}</TableCell>
                          <TableCell>{runnerSpec.template_id}</TableCell>
                          <TableCell>{runnerSpec.runner_group || "-"}</TableCell>
                          <TableCell>{groupNamesForSpec(runnerSpec.name).join(", ") || "-"}</TableCell>
                          <TableCell>{runnerSpec.default_available ? "yes" : "no"}</TableCell>
                          <TableCell>{runnerSpec.max_concurrency}</TableCell>
                          <TableCell>
                            <Button
                              type="button"
                              variant="outline"
                              size="sm"
                              onClick={(event) => {
                                event.stopPropagation()
                                void deleteRunnerSpec(runnerSpec.name)
                              }}
                            >
                              <Trash2 />
                              Delete
                            </Button>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </CardContent>
              </Card>
              <Dialog open={runnerSpecOpen} onOpenChange={setRunnerSpecOpen}>
                <DialogContent>
                  <DialogHeader>
                    <DialogTitle>{runnerSpecForm.name ? "Edit runner spec" : "Create runner spec"}</DialogTitle>
                    <DialogDescription>Define labels, template, group membership, and capacity.</DialogDescription>
                  </DialogHeader>
                  <form className="grid gap-3" onSubmit={saveRunnerSpec}>
                    <Input
                      value={runnerSpecForm.name}
                      onChange={(event) => setRunnerSpecForm((current) => ({ ...current, name: event.target.value }))}
                      placeholder="runner spec name"
                    />
                    <Input
                      value={runnerSpecForm.labels}
                      onChange={(event) => setRunnerSpecForm((current) => ({ ...current, labels: event.target.value }))}
                      placeholder="self-hosted,e2b"
                    />
                    <div className="grid gap-2 sm:grid-cols-2">
                      <Input
                        value={runnerSpecForm.template_id}
                        onChange={(event) => setRunnerSpecForm((current) => ({ ...current, template_id: event.target.value }))}
                        placeholder="template id"
                      />
                      <Input
                        value={runnerSpecForm.runner_group}
                        onChange={(event) => setRunnerSpecForm((current) => ({ ...current, runner_group: event.target.value }))}
                        placeholder="optional GitHub runner group"
                      />
                    </div>
                    <div className="grid gap-2 rounded-md border p-3">
                      {runnerGroups.length === 0 ? (
                        <div className="text-sm text-muted-foreground">No internal runner groups configured.</div>
                      ) : (
                        runnerGroups.map((group) => (
                          <label key={group.name} className="flex items-center gap-2 text-sm">
                            <input
                              type="checkbox"
                              checked={runnerSpecForm.group_names.includes(group.name)}
                              onChange={(event) =>
                                setRunnerSpecForm((current) => ({
                                  ...current,
                                  group_names: event.target.checked
                                    ? [...current.group_names, group.name]
                                    : current.group_names.filter((name) => name !== group.name),
                                }))
                              }
                            />
                            {group.name}
                          </label>
                        ))
                      )}
                    </div>
                    <div className="grid grid-cols-3 gap-2">
                      <Input
                        value={runnerSpecForm.max_concurrency}
                        onChange={(event) => setRunnerSpecForm((current) => ({ ...current, max_concurrency: event.target.value }))}
                        placeholder="max concurrency"
                      />
                      <Input
                        value={runnerSpecForm.min_idle}
                        onChange={(event) => setRunnerSpecForm((current) => ({ ...current, min_idle: event.target.value }))}
                        placeholder="min idle"
                      />
                      <Input
                        value={runnerSpecForm.priority}
                        onChange={(event) => setRunnerSpecForm((current) => ({ ...current, priority: event.target.value }))}
                        placeholder="priority"
                      />
                    </div>
                    <label className="flex items-center gap-2 text-sm">
                      <input
                        type="checkbox"
                        checked={runnerSpecForm.enabled}
                        onChange={(event) => setRunnerSpecForm((current) => ({ ...current, enabled: event.target.checked }))}
                      />
                      enabled
                    </label>
                    <label className="flex items-center gap-2 text-sm">
                      <input
                        type="checkbox"
                        checked={runnerSpecForm.default_available}
                        onChange={(event) =>
                          setRunnerSpecForm((current) => ({ ...current, default_available: event.target.checked }))
                        }
                      />
                      globally available by default
                    </label>
                    <DialogFooter>
                      <Button type="button" variant="outline" onClick={() => setRunnerSpecOpen(false)}>
                        Cancel
                      </Button>
                      <Button type="submit">Save runner spec</Button>
                    </DialogFooter>
                  </form>
                </DialogContent>
              </Dialog>
            </div>
          ) : null}

          {section === "runner_groups" ? (
            <div className="grid gap-4">
              <Card className="min-w-0">
                <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div>
                    <CardTitle>Runner groups</CardTitle>
                    <CardDescription>Click a group row to edit its runner specs.</CardDescription>
                  </div>
                  <div className="flex gap-2">
                    <Button
                      type="button"
                      onClick={() => {
                        resetRunnerGroupForm()
                        setRunnerGroupOpen(true)
                      }}
                    >
                      <Plus />
                      Create
                    </Button>
                    <Button type="button" variant="outline" size="icon" onClick={() => void loadAll()} disabled={loading} title="Refresh">
                      <RefreshCw className={cn(loading && "animate-spin")} />
                    </Button>
                  </div>
                </CardHeader>
                <CardContent className="p-0">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Name</TableHead>
                        <TableHead>Specs</TableHead>
                        <TableHead>Enabled</TableHead>
                        <TableHead>Updated</TableHead>
                        <TableHead className="w-24" />
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {runnerGroups.map((group) => (
                        <TableRow key={group.name} className="cursor-pointer" onClick={() => loadRunnerGroupIntoForm(group)}>
                          <TableCell>{group.name}</TableCell>
                          <TableCell className="max-w-[420px] truncate">{group.spec_names.join(", ") || "-"}</TableCell>
                          <TableCell>{group.enabled ? "yes" : "no"}</TableCell>
                          <TableCell>{formatTime(group.updated_at)}</TableCell>
                          <TableCell>
                            <Button
                              type="button"
                              variant="outline"
                              size="sm"
                              onClick={(event) => {
                                event.stopPropagation()
                                void deleteRunnerGroup(group.name)
                              }}
                            >
                              <Trash2 />
                              Delete
                            </Button>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </CardContent>
              </Card>
              <Dialog open={runnerGroupOpen} onOpenChange={setRunnerGroupOpen}>
                <DialogContent>
                  <DialogHeader>
                    <DialogTitle>{runnerGroupForm.name ? "Edit runner group" : "Create runner group"}</DialogTitle>
                    <DialogDescription>Group runner specs so repositories can allow a named set.</DialogDescription>
                  </DialogHeader>
                  <form className="grid gap-3" onSubmit={saveRunnerGroup}>
                    <Input
                      value={runnerGroupForm.name}
                      onChange={(event) => setRunnerGroupForm((current) => ({ ...current, name: event.target.value }))}
                      placeholder="runner group name"
                    />
                    <Input
                      value={runnerGroupForm.description}
                      onChange={(event) => setRunnerGroupForm((current) => ({ ...current, description: event.target.value }))}
                      placeholder="description"
                    />
                    <div className="grid gap-2 rounded-md border p-3">
                      {runnerSpecs.length === 0 ? (
                        <div className="text-sm text-muted-foreground">Create a runner spec before adding specs to a group.</div>
                      ) : (
                        runnerSpecs.map((runnerSpec) => (
                          <label key={runnerSpec.name} className="flex items-center gap-2 text-sm">
                            <input
                              type="checkbox"
                              checked={runnerGroupForm.spec_names.includes(runnerSpec.name)}
                              onChange={(event) =>
                                setRunnerGroupForm((current) => ({
                                  ...current,
                                  spec_names: event.target.checked
                                    ? [...current.spec_names, runnerSpec.name]
                                    : current.spec_names.filter((name) => name !== runnerSpec.name),
                                }))
                              }
                            />
                            {runnerSpec.name}
                          </label>
                        ))
                      )}
                    </div>
                    <label className="flex items-center gap-2 text-sm">
                      <input
                        type="checkbox"
                        checked={runnerGroupForm.enabled}
                        onChange={(event) => setRunnerGroupForm((current) => ({ ...current, enabled: event.target.checked }))}
                      />
                      enabled
                    </label>
                    <DialogFooter>
                      <Button type="button" variant="outline" onClick={() => setRunnerGroupOpen(false)}>
                        Cancel
                      </Button>
                      <Button type="submit">Save runner group</Button>
                    </DialogFooter>
                  </form>
                </DialogContent>
              </Dialog>
            </div>
          ) : null}

          {section === "runner_policies" ? (
            <div className="grid gap-4">
              <Card className="min-w-0">
                <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div>
                    <CardTitle>Runner policies</CardTitle>
                    <CardDescription>Click a policy row to edit it.</CardDescription>
                  </div>
                  <div className="flex gap-2">
                    <Button
                      type="button"
                      onClick={() => {
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
                      }}
                    >
                      <Plus />
                      Create
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      size="icon"
                      onClick={() => void loadAll()}
                      disabled={loading}
                      title="Refresh"
                    >
                      <RefreshCw className={cn(loading && "animate-spin")} />
                    </Button>
                  </div>
                </CardHeader>
                <CardContent className="p-0">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Repository</TableHead>
                        <TableHead>Target</TableHead>
                        <TableHead>Enabled</TableHead>
                        <TableHead>Created</TableHead>
                        <TableHead className="w-24" />
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {runnerPolicies.map((policy) => (
                        <TableRow key={policy.id} className="cursor-pointer" onClick={() => loadPolicyIntoForm(policy)}>
                          <TableCell>{policy.repository_full_name}</TableCell>
                          <TableCell>
                            {policy.runner_group_name
                              ? `group:${policy.runner_group_name}`
                              : `spec:${policy.runner_spec_name || "-"}`}
                          </TableCell>
                          <TableCell>{policy.enabled ? "yes" : "no"}</TableCell>
                          <TableCell>{formatTime(policy.created_at)}</TableCell>
                          <TableCell>
                            <Button
                              type="button"
                              variant="outline"
                              size="sm"
                              onClick={(event) => {
                                event.stopPropagation()
                                void deletePolicy(policy.id)
                              }}
                            >
                              <Trash2 />
                              Delete
                            </Button>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </CardContent>
              </Card>
              <Dialog open={runnerPolicyOpen} onOpenChange={setRunnerPolicyOpen}>
                <DialogContent>
                  <DialogHeader>
                    <DialogTitle>{runnerPolicyForm.id > 0 ? "Edit runner policy" : "Create runner policy"}</DialogTitle>
                    <DialogDescription>Bind a repository pattern to an allowed runner spec or group.</DialogDescription>
                  </DialogHeader>
                  <form className="grid gap-3" onSubmit={savePolicy}>
                    <Input
                      value={runnerPolicyForm.repository_full_name}
                      onChange={(event) =>
                        setPolicyForm((current) => ({ ...current, repository_full_name: event.target.value }))
                      }
                      placeholder="owner/repo or owner/*"
                    />
                    <Select
                      value={runnerPolicyForm.target_type}
                      onValueChange={(value) =>
                        setPolicyForm((current) => ({ ...current, target_type: value }))
                      }
                    >
                      <SelectTrigger className="w-full">
                        <SelectValue placeholder="target type" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="group">Runner group</SelectItem>
                        <SelectItem value="spec">Runner spec</SelectItem>
                      </SelectContent>
                    </Select>
                    {runnerPolicyForm.target_type === "group" ? (
                      runnerGroups.length === 0 ? (
                        <div className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">
                          Create a runner group before adding group policies.
                        </div>
                      ) : (
                        <Select
                          value={runnerPolicyForm.runner_group_name}
                          onValueChange={(value) =>
                            setPolicyForm((current) => ({ ...current, runner_group_name: value }))
                          }
                        >
                          <SelectTrigger className="w-full">
                            <SelectValue placeholder="runner group" />
                          </SelectTrigger>
                          <SelectContent>
                            {runnerGroups.map((group) => (
                              <SelectItem key={group.name} value={group.name}>
                                {group.name}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      )
                    ) : runnerSpecs.length === 0 ? (
                      <div className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">
                        Create a runner spec before adding runner policies.
                      </div>
                    ) : (
                      <Select
                        value={runnerPolicyForm.runner_spec_name}
                        onValueChange={(value) =>
                          setPolicyForm((current) => ({ ...current, runner_spec_name: value }))
                        }
                      >
                        <SelectTrigger className="w-full">
                          <SelectValue placeholder="runner spec" />
                        </SelectTrigger>
                        <SelectContent>
                          {runnerSpecs.map((runnerSpec) => (
                            <SelectItem key={runnerSpec.name} value={runnerSpec.name}>
                              {runnerSpec.name}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    )}
                    <label className="flex items-center gap-2 text-sm">
                      <input
                        type="checkbox"
                        checked={runnerPolicyForm.enabled}
                        onChange={(event) => setPolicyForm((current) => ({ ...current, enabled: event.target.checked }))}
                      />
                      enabled
                    </label>
                    <DialogFooter>
                      <Button type="button" variant="outline" onClick={() => setRunnerPolicyOpen(false)}>
                        Cancel
                      </Button>
                      <Button type="submit">Save policy</Button>
                    </DialogFooter>
                  </form>
                </DialogContent>
              </Dialog>
            </div>
          ) : null}

          {section === "match" ? (
            <div className="grid gap-4 xl:grid-cols-[420px_minmax(0,1fr)]">
              <Card>
                <CardHeader>
                  <CardTitle>Label matching test</CardTitle>
                  <CardDescription>Preview which runner spec a repository and label set would use.</CardDescription>
                </CardHeader>
                <CardContent>
                  <form className="grid gap-3" onSubmit={runMatchTest}>
                    <Input
                      value={matchRepository}
                      onChange={(event) => setMatchRepository(event.target.value)}
                      placeholder="owner/repo"
                    />
                    <Input
                      value={matchLabels}
                      onChange={(event) => setMatchLabels(event.target.value)}
                      placeholder="self-hosted,e2b"
                    />
                    <Button type="submit">Run match</Button>
                  </form>
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardTitle>Match result</CardTitle>
                  <CardDescription>Runner policy + label coverage resolution.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  {matchResult ? (
                    <>
                      <Detail label="Repository" value={matchResult.repository_full_name || "-"} />
                      <Detail label="Labels" value={matchResult.labels.join(", ") || "-"} />
                      <Detail label="Runner spec" value={matchResult.runner_spec?.name || "-"} />
                      <Detail label="Reason" value={matchResult.reason || "matched"} />
                    </>
                  ) : (
                    <div className="text-sm text-muted-foreground">No match run yet.</div>
                  )}
                </CardContent>
              </Card>
            </div>
          ) : null}

          {section === "audit" ? (
            <Card className="min-w-0">
              <CardHeader>
                <CardTitle>Audit events</CardTitle>
                <CardDescription>Recent admin and recovery control-plane actions.</CardDescription>
              </CardHeader>
              <CardContent className="p-0">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Time</TableHead>
                      <TableHead>Actor</TableHead>
                      <TableHead>Action</TableHead>
                      <TableHead>Resource</TableHead>
                      <TableHead>Payload</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {auditEvents.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">
                          No audit events yet
                        </TableCell>
                      </TableRow>
                    ) : (
                      auditEvents.map((event) => (
                        <TableRow key={event.id}>
                          <TableCell>{formatTime(event.created_at)}</TableCell>
                          <TableCell>{event.actor}</TableCell>
                          <TableCell>{event.action}</TableCell>
                          <TableCell>
                            {event.resource_type} · {event.resource_id}
                          </TableCell>
                          <TableCell className="max-w-[420px] truncate text-xs text-muted-foreground">
                            {event.payload_json || "-"}
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>
          ) : null}

          {section === "diagnostics" ? (
            <div className="grid gap-4 xl:grid-cols-2">
              <Card>
                <CardHeader>
                  <CardTitle>Diagnostics summary</CardTitle>
                  <CardDescription>DB, GitHub auth, sandbox API, and pprof discovery.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  <Detail label="State backend" value={diagnostics?.state.backend || "-"} />
                  <Detail label="Database" value={diagnostics?.state.database || "-"} />
                  <Detail label="GitHub auth" value={diagnostics?.github.auth_mode || "-"} />
                  <Detail label="Installation" value={diagnostics?.github.installation_id || "-"} />
                  <Detail label="GitHub API" value={diagnostics?.github.api_base_url || "-"} />
                  <Detail label="Sandbox API" value={diagnostics?.sandbox.api_url || "-"} />
                  <div className="space-y-2">
                    <div className="text-sm font-medium">pprof endpoints</div>
                    {diagnostics?.pprof?.length ? (
                      diagnostics.pprof.map((item) => (
                        <div key={item.address_file} className="rounded-md border p-3 text-xs">
                          <div className="font-medium">{item.address}</div>
                          <div className="text-muted-foreground">{item.address_file}</div>
                          <div className="text-muted-foreground">{item.dump_script}</div>
                        </div>
                      ))
                    ) : (
                      <div className="text-sm text-muted-foreground">No pprof artifact discovered yet.</div>
                    )}
                  </div>
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardTitle>Recent failures</CardTitle>
                  <CardDescription>Latest failed requests plus the current /debug/vars snapshot.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="space-y-2">
                    {diagnostics?.recent_failures?.length ? (
                      diagnostics.recent_failures.map((failure) => (
                        <div key={failure.id} className="rounded-md border p-3 text-sm">
                          <div className="font-medium">{failure.id}</div>
                          <div className="text-xs text-muted-foreground">
                            {failure.repository_full_name || "-"} · {failure.runner_spec_name || "-"} ·{" "}
                            {failure.failure_reason || failure.error || "-"}
                          </div>
                        </div>
                      ))
                    ) : (
                      <div className="text-sm text-muted-foreground">No recent failures.</div>
                    )}
                  </div>
                  <pre className="max-h-[48vh] min-h-72 overflow-auto rounded-lg border bg-muted/50 p-3 text-xs leading-relaxed whitespace-pre-wrap">
                    {diagnosticsVars || "No /debug/vars data available"}
                  </pre>
                </CardContent>
              </Card>
            </div>
          ) : null}
        </main>
      </SidebarInset>
      <Toaster richColors />
    </SidebarProvider>
  )
}

function LoginPage({
  oauthEnabled,
  currentLogin,
  currentRole,
  onSignOut,
}: {
  oauthEnabled: boolean
  currentLogin?: string
  currentRole?: string
  onSignOut: () => void
}) {
  return (
    <main className="min-h-screen bg-background text-foreground">
      <div className="min-h-screen">
        <section className="relative flex min-h-screen overflow-hidden bg-[radial-gradient(circle_at_30%_20%,oklch(0.92_0.08_195),transparent_34%),linear-gradient(135deg,oklch(0.98_0.02_180),oklch(0.94_0.03_250))] p-6 sm:p-10">
          <div className="absolute inset-x-10 top-1/2 h-px bg-foreground/10" />
          <div className="absolute bottom-28 left-10 right-20 grid grid-cols-6 gap-2 opacity-70">
            {Array.from({ length: 36 }).map((_, index) => (
              <span
                key={index}
                className={cn(
                  "h-2 rounded-full",
                  index % 7 === 0 ? "bg-primary" : index % 5 === 0 ? "bg-emerald-500" : "bg-foreground/15"
                )}
              />
            ))}
          </div>
          <div className="relative z-10 flex h-fit items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-foreground text-background shadow-sm">
              <Play className="h-5 w-5" />
            </div>
            <div>
              <div className="text-sm font-semibold tracking-wide">E2B Runner Console</div>
              <div className="text-xs text-muted-foreground">Ephemeral Actions control plane</div>
            </div>
          </div>

          <div className="absolute inset-0 z-20 flex items-center justify-center px-5">
            <Card className="rounded-lg shadow-sm">
              <CardHeader className="gap-2">
                <div className="flex h-11 w-11 items-center justify-center rounded-lg border bg-muted">
                  <ShieldCheck className="h-5 w-5 text-primary" />
                </div>
                <CardTitle className="text-2xl">Sign in</CardTitle>
                <CardDescription>Use your allowed GitHub account to access runnerd.</CardDescription>
              </CardHeader>
              <CardContent className="space-y-5">
                {currentLogin ? (
                  <div className="space-y-3">
                    <div className="rounded-lg border bg-muted/50 p-3 text-sm text-muted-foreground">
                      @{currentLogin} is signed in as {currentRole || "user"} and does not have admin access.
                    </div>
                    <Button
                      type="button"
                      variant="outline"
                      size="lg"
                      className="w-full justify-center"
                      onClick={onSignOut}
                    >
                      Sign out
                    </Button>
                  </div>
                ) : oauthEnabled ? (
                  <Button
                    type="button"
                    size="lg"
                    className="w-full justify-center"
                    onClick={() => {
                      window.location.href = "/auth/github/login"
                    }}
                  >
                    <Github className="h-4 w-4" />
                    Continue with GitHub
                  </Button>
                ) : (
                  <div className="rounded-lg border bg-muted/50 p-3 text-sm text-muted-foreground">
                    GitHub OAuth is required but not configured on this runnerd instance.
                  </div>
                )}
              </CardContent>
            </Card>
          </div>

          <div className="absolute bottom-10 left-6 right-6 z-10 max-w-2xl sm:left-10 sm:right-auto">
            <h1 className="max-w-lg text-3xl font-semibold leading-tight text-foreground sm:text-4xl">
              Sign in before touching live runner capacity.
            </h1>
            <div className="mt-5 grid max-w-xl gap-3 sm:grid-cols-3">
              {[
                ["Isolated", "E2B sandboxes per job"],
                ["Scoped", "GitHub allowlists and policies"],
                ["Audited", "Admin actions are traceable"],
              ].map(([label, detail]) => (
                <div key={label} className="rounded-lg border bg-background/70 p-3 shadow-sm backdrop-blur">
                  <div className="text-sm font-semibold">{label}</div>
                  <div className="mt-1 text-xs leading-relaxed text-muted-foreground">{detail}</div>
                </div>
              ))}
            </div>
          </div>
        </section>
      </div>
    </main>
  )
}

function StatusBadge({ status }: { status: RunnerStatus }) {
  if (status === "running") {
    return (
      <Badge variant="success">
        <Play />
        running
      </Badge>
    )
  }
  if (status === "failed") {
    return (
      <Badge variant="danger">
        <AlertCircle />
        failed
      </Badge>
    )
  }
  if (status === "completed") {
    return (
      <Badge variant="outline">
        <Square />
        completed
      </Badge>
    )
  }
  if (status === "creating") {
    return (
      <Badge variant="warning">
        <Loader2 className="animate-spin" />
        creating
      </Badge>
    )
  }
  if (status === "stopping") {
    return (
      <Badge variant="warning">
        <Clock3 />
        stopping
      </Badge>
    )
  }
  return (
    <Badge variant="secondary">
      <CheckCircle2 />
      queued
    </Badge>
  )
}

function Detail({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="grid grid-cols-[110px_minmax(0,1fr)] gap-x-3 gap-y-2 text-sm">
      <div className="text-muted-foreground">{label}</div>
      <div className="min-w-0 break-words font-medium">{value}</div>
    </div>
  )
}

function formatTime(value?: string) {
  if (!value) return "-"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

export default App
