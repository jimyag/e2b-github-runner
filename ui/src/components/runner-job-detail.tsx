import { type ReactNode, useCallback, useEffect, useMemo, useState } from "react"
import { ArrowLeft, CheckCircle2, Clock3, ExternalLink, Loader2, Play, RefreshCw, SquareTerminal, XCircle } from "lucide-react"

import type { RunnerJobGroup, RunnerState } from "@/admin-types"
import { logNames } from "@/admin-types"
import { formatRunnerDuration, formatTime } from "@/admin-format"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useSandboxTerminal } from "@/hooks/use-sandbox-terminal"
import { cn } from "@/lib/utils"

type LogName = (typeof logNames)[number]

type RunnerJobDetailProps = {
  id: string
  apiBase: string
  onBack: () => void
  onOpenJob?: (id: string) => void
  request: (url: string, options?: RequestInit) => Promise<unknown>
}

type ActiveGuard = {
  current: boolean
}

export function RunnerJobDetail({ id, apiBase, onBack, onOpenJob, request }: RunnerJobDetailProps) {
  const [job, setJob] = useState<RunnerState | null>(null)
  const [jobGroup, setJobGroup] = useState<RunnerJobGroup | null>(null)
  const [loading, setLoading] = useState(true)
  const [selectedLog, setSelectedLog] = useState<LogName>("control.log")
  const [logText, setLogText] = useState("Loading log...")
  const [githubLogText, setGithubLogText] = useState("Loading GitHub log...")
  const [githubLogLoading, setGithubLogLoading] = useState(false)
  const endpoint = useMemo(() => `${apiBase}/${encodeURIComponent(id)}`, [apiBase, id])
  const terminalAvailable = job ? isTerminalAvailable(job) : false
  const { terminalEl, terminalSession, terminalError, terminalConnecting, connectTerminal } = useSandboxTerminal({
    endpoint,
    available: terminalAvailable,
    request,
    connectingMessage: "Connecting to sandbox terminal...",
    streamDisconnectedMessage: "Terminal stream disconnected",
    connectErrorMessage: "Failed to connect terminal",
  })

  const loadJob = useCallback(async (active: ActiveGuard = { current: true }) => {
    setLoading(true)
    try {
      const data = await request(endpoint)
      if (active.current) {
        setJob(data as RunnerState)
      }
    } finally {
      if (active.current) {
        setLoading(false)
      }
    }
  }, [endpoint, request])

  const loadJobGroup = useCallback(async (active: ActiveGuard = { current: true }) => {
    try {
      const data = await request(`${endpoint}/group`)
      if (active.current) {
        setJobGroup(data as RunnerJobGroup)
      }
    } catch {
      if (active.current) {
        setJobGroup(null)
      }
    }
  }, [endpoint, request])

  const loadLog = useCallback(async (name = selectedLog, active: ActiveGuard = { current: true }) => {
    setLogText("Loading log...")
    try {
      const text = await request(`${endpoint}/logs/${encodeURIComponent(name)}`)
      if (active.current) {
        setLogText(typeof text === "string" ? text || "Log is empty" : JSON.stringify(text, null, 2))
      }
    } catch (error) {
      if (active.current) {
        setLogText(error instanceof Error ? error.message : "Failed to load log")
      }
    }
  }, [endpoint, request, selectedLog])

  const loadGithubLog = useCallback(async (active: ActiveGuard = { current: true }) => {
    setGithubLogLoading(true)
    setGithubLogText("Loading GitHub log...")
    try {
      const text = await request(`${endpoint}/github-log`)
      if (active.current) {
        setGithubLogText(typeof text === "string" ? text || "GitHub log is empty" : JSON.stringify(text, null, 2))
      }
    } catch (error) {
      if (active.current) {
        setGithubLogText(error instanceof Error ? error.message : "Failed to load GitHub log")
      }
    } finally {
      if (active.current) {
        setGithubLogLoading(false)
      }
    }
  }, [endpoint, request])

  useEffect(() => {
    const active = { current: true }
    void loadJob(active)
    void loadJobGroup(active)
    return () => {
      active.current = false
    }
  }, [loadJob, loadJobGroup])

  useEffect(() => {
    const active = { current: true }
    void loadLog(selectedLog, active)
    return () => {
      active.current = false
    }
  }, [loadLog, selectedLog])

  useEffect(() => {
    const active = { current: true }
    void loadGithubLog(active)
    return () => {
      active.current = false
    }
  }, [loadGithubLog])

  const refreshPage = () => {
    void loadJob()
    void loadJobGroup()
  }

  const openSibling = (jobID: string) => {
    if (jobID === id) return
    if (onOpenJob) {
      onOpenJob(jobID)
      return
    }
    window.history.pushState(null, "", `/jobs/${encodeURIComponent(jobID)}`)
  }

  const groupJobs = jobGroup ? [...jobGroup.current_jobs, ...jobGroup.previous_jobs] : job ? [job] : []

  return (
    <div className="flex min-h-screen flex-col bg-background">
      <header className="border-b bg-background/95 px-4 py-3 lg:px-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-3">
            <Button type="button" variant="outline" size="icon" onClick={onBack} title="Back to jobs">
              <ArrowLeft />
            </Button>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="truncate text-xl font-semibold">{job ? runnerJobTitle(job) : id}</h1>
                {job ? <Badge className={statusClass(job.status)}>{job.status}</Badge> : null}
              </div>
              <p className="truncate text-sm text-muted-foreground">
                {job?.repository_full_name || "runner request"} {job?.workflow_name ? ` / ${job.workflow_name}` : ""}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            {job?.github_job_url ? (
              <Button type="button" variant="outline" asChild>
                <a href={job.github_job_url} target="_blank" rel="noreferrer">
                  <ExternalLink />
                  GitHub
                </a>
              </Button>
            ) : null}
            <Button type="button" variant="outline" onClick={refreshPage} disabled={loading}>
              <RefreshCw className={cn(loading && "animate-spin")} />
              Refresh
            </Button>
          </div>
        </div>
      </header>

      <main className="grid min-h-0 flex-1 lg:grid-cols-[320px_minmax(0,1fr)]">
        <aside className="min-h-0 border-b bg-muted/20 lg:sticky lg:top-0 lg:h-screen lg:self-start lg:border-r lg:border-b-0">
          <div className="border-b px-4 py-4">
            <div className="flex items-center justify-between gap-3">
              <div className="min-w-0">
                <h2 className="truncate text-sm font-semibold">{jobGroupTitle(jobGroup)}</h2>
                <p className="text-xs text-muted-foreground">{jobGroup?.jobs.length || (job ? 1 : 0)} jobs</p>
              </div>
              <Button type="button" variant="outline" size="icon" onClick={() => void loadJobGroup()} title="Refresh jobs">
                <RefreshCw />
              </Button>
            </div>
          </div>
          <div className="max-h-72 overflow-auto p-2 lg:max-h-[calc(100vh-133px)]">
            {groupJobs.map((item) => (
              <JobListItem key={item.id} job={item} selected={item.id === id} onOpen={() => openSibling(item.id)} />
            ))}
          </div>
        </aside>

        <section className="min-w-0 p-4 lg:p-6">
          <div className="mb-4 grid gap-3 border-b pb-4 md:grid-cols-3 xl:grid-cols-5">
            {job ? (
              <>
                <SummaryMetric label="Runner spec" value={job.runner_spec_name || "matched by labels"} />
                <SummaryMetric label="Workflow run" value={workflowRunLink(job)} />
                <SummaryMetric label="Queued" value={formatTime(job.created_at)} />
                <SummaryMetric label="Started" value={formatTime(job.running_at)} />
                <SummaryMetric label="Finished" value={formatTime(job.completed_at || job.failed_at)} />
              </>
            ) : (
              <div className="text-sm text-muted-foreground">Loading...</div>
            )}
          </div>
          <Tabs defaultValue="github-logs" className="h-full">
            <TabsList>
              <TabsTrigger value="github-logs">GitHub logs</TabsTrigger>
              <TabsTrigger value="logs">Runner logs</TabsTrigger>
              <TabsTrigger value="terminal">Terminal</TabsTrigger>
              <TabsTrigger value="details">Details</TabsTrigger>
            </TabsList>
            <TabsContent value="logs" className="mt-4">
              <Card className="gap-0 py-0">
                <CardHeader className="sticky top-0 z-10 border-b bg-card px-5 py-4">
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div>
                      <CardTitle>Runner logs</CardTitle>
                      <CardDescription>Runner lifecycle, sandbox stdout, and stderr captured by runnerd.</CardDescription>
                    </div>
                    <Button type="button" variant="outline" size="sm" onClick={() => void loadLog()}>
                      <RefreshCw />
                      Refresh
                    </Button>
                  </div>
                </CardHeader>
                <CardContent className="p-5">
                  <Tabs value={selectedLog} onValueChange={(value) => setSelectedLog(value as LogName)}>
                    <TabsList>
                      {logNames.map((name) => (
                        <TabsTrigger key={name} value={name}>
                          {name.replace(".log", "")}
                        </TabsTrigger>
                      ))}
                    </TabsList>
                  </Tabs>
                  <pre className="mt-4 font-mono text-xs leading-relaxed whitespace-pre-wrap break-words">
                    {logText}
                  </pre>
                </CardContent>
              </Card>
            </TabsContent>
            <TabsContent value="github-logs" className="mt-4">
              <Card className="gap-0 py-0">
                <CardHeader className="sticky top-0 z-10 border-b bg-card px-5 py-4">
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div>
                      <CardTitle>GitHub logs</CardTitle>
                      <CardDescription>Workflow job output downloaded from GitHub Actions.</CardDescription>
                    </div>
                    <Button type="button" variant="outline" size="sm" onClick={() => void loadGithubLog()} disabled={githubLogLoading}>
                      <RefreshCw className={cn(githubLogLoading && "animate-spin")} />
                      Refresh
                    </Button>
                  </div>
                </CardHeader>
                <CardContent className="p-5">
                  <pre className="font-mono text-xs leading-relaxed whitespace-pre-wrap break-words">
                    {githubLogText}
                  </pre>
                </CardContent>
              </Card>
            </TabsContent>
            <TabsContent value="terminal" forceMount className="mt-4 data-[state=inactive]:hidden">
              <Card className="gap-0 py-0">
                <CardHeader className="border-b px-5 py-4">
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div>
                      <CardTitle>Sandbox terminal</CardTitle>
                      <CardDescription>{job?.sandbox_id || "No active sandbox"}</CardDescription>
                    </div>
                    <Button type="button" onClick={() => void connectTerminal()} disabled={!terminalAvailable || terminalConnecting || Boolean(terminalSession)}>
                      <SquareTerminal />
                      {terminalSession ? "Connected" : terminalConnecting ? "Connecting" : "Connect"}
                    </Button>
                  </div>
                </CardHeader>
                <CardContent className="p-0">
                  <div className="relative min-h-[520px] bg-[#111318] p-2">
                    <div ref={terminalEl} className="h-[500px] overflow-hidden rounded-md" />
                    {!terminalSession ? (
                      <div className="absolute inset-2 flex items-center justify-center rounded-md border border-white/10 bg-[#111318] text-sm text-slate-300">
                        {terminalAvailable ? "Connect when you need an interactive shell." : "Terminal is available only while the sandbox job is active."}
                      </div>
                    ) : null}
                  </div>
                  {terminalError ? <div className="border-t px-5 py-3 text-sm text-destructive">{terminalError}</div> : null}
                </CardContent>
              </Card>
            </TabsContent>
            <TabsContent value="details" className="mt-4">
              <Card>
                <CardHeader>
                  <CardTitle>Details</CardTitle>
                </CardHeader>
                <CardContent className="grid gap-2 text-sm">
                  {job ? detailRows(job).map(([label, value]) => <Detail key={label} label={label} value={value} />) : "Loading..."}
                </CardContent>
              </Card>
            </TabsContent>
          </Tabs>
        </section>
      </main>
    </div>
  )
}

function JobListItem({ job, selected, onOpen }: { job: RunnerState; selected: boolean; onOpen: () => void }) {
  return (
    <button
      type="button"
      onClick={onOpen}
      className={cn(
        "mb-1 grid w-full grid-cols-[22px_minmax(0,1fr)] gap-2 rounded-md px-3 py-3 text-left text-sm transition-colors",
        selected ? "bg-background shadow-sm ring-1 ring-border" : "hover:bg-background/70"
      )}
    >
      <span className={cn("mt-0.5", jobStatusIconClass(job.status))}>{jobStatusIcon(job.status)}</span>
      <span className="min-w-0">
        <span className="flex min-w-0 items-center justify-between gap-2">
          <span className="truncate font-medium">{runnerJobTitle(job)}</span>
          <span className="shrink-0 text-xs text-muted-foreground">{formatRunnerDuration(job)}</span>
        </span>
        <span className="mt-1 block truncate text-xs text-muted-foreground">{job.runner_spec_name || job.runner_name || job.id}</span>
      </span>
    </button>
  )
}

function SummaryMetric({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="min-w-0">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 truncate text-sm font-medium">{value || "-"}</div>
    </div>
  )
}

function workflowRunLink(job: RunnerState) {
  if (!job.workflow_run_id) return "-"
  const url = workflowRunURL(job)
  if (!url) return job.workflow_run_id
  return (
    <a className="inline-flex items-center gap-1 text-primary hover:underline" href={url} target="_blank" rel="noreferrer">
      {job.workflow_run_id}
      <ExternalLink className="h-3 w-3" />
    </a>
  )
}

function workflowRunURL(job: RunnerState) {
  if (!job.github_job_url || !job.workflow_run_id) return ""
  const marker = `/actions/runs/${job.workflow_run_id}`
  const index = job.github_job_url.indexOf(marker)
  if (index === -1) return ""
  return job.github_job_url.slice(0, index + marker.length)
}

function Detail({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="grid grid-cols-[140px_minmax(0,1fr)] gap-3 border-b py-2 last:border-b-0">
      <span className="text-muted-foreground">{label}</span>
      <span className="min-w-0 break-words font-medium">{value || "-"}</span>
    </div>
  )
}

function detailRows(job: RunnerState): Array<[string, ReactNode]> {
  return [
    ["ID", job.id],
    ["Status", job.status],
    ["Repository", job.repository_full_name],
    ["Runner spec", job.runner_spec_name || "matched by labels"],
    ["Runner group", job.runner_group],
    ["Sandbox", job.sandbox_id],
    ["PID", job.process_pid],
    ["Job", job.assigned_job_name || job.assigned_job_id || job.workflow_job_id],
    ["Workflow", job.workflow_name],
    ["Workflow run", job.workflow_run_id],
    ["Workflow attempt", job.workflow_run_attempt],
    ["Pull request", job.pull_request_number],
    ["Branch", job.head_branch],
    ["Commit", job.head_sha],
    ["Created", formatTime(job.created_at)],
    ["Updated", formatTime(job.updated_at)],
    ["Completed", formatTime(job.completed_at)],
    ["Failed", formatTime(job.failed_at)],
    ["Retry count", job.retry_count],
    ["Next retry", formatTime(job.next_retry_at)],
    ["Requested labels", job.requested_labels?.join(", ")],
    ["Failure", job.failure_reason],
    ["Last error", job.last_error_message || job.error],
  ]
}

function runnerJobTitle(job: RunnerState) {
  if (job.assigned_job_name && job.assigned_job_name !== "__runner_job_started__") return job.assigned_job_name
  return job.workflow_name || job.runner_name || job.id
}

function isTerminalAvailable(job: RunnerState) {
  return Boolean(job.sandbox_id && ["creating", "running", "stopping"].includes(job.status))
}

function jobGroupTitle(group: RunnerJobGroup | null) {
  if (!group) return "Workflow jobs"
  return group.title ? `${group.title} jobs` : "Workflow jobs"
}

function jobStatusIcon(status: RunnerState["status"]) {
  switch (status) {
    case "completed":
      return <CheckCircle2 className="h-4 w-4" />
    case "failed":
      return <XCircle className="h-4 w-4" />
    case "running":
      return <Play className="h-4 w-4" />
    case "creating":
      return <Loader2 className="h-4 w-4 animate-spin" />
    case "stopping":
      return <Clock3 className="h-4 w-4" />
    default:
      return <Clock3 className="h-4 w-4" />
  }
}

function jobStatusIconClass(status: RunnerState["status"]) {
  switch (status) {
    case "completed":
      return "text-emerald-600"
    case "failed":
      return "text-destructive"
    case "running":
      return "text-blue-600"
    case "creating":
    case "stopping":
      return "text-amber-600"
    default:
      return "text-muted-foreground"
  }
}

function statusClass(status: RunnerState["status"]) {
  switch (status) {
    case "completed":
      return "bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300"
    case "failed":
      return "bg-red-100 text-red-700 dark:bg-red-500/15 dark:text-red-300"
    case "running":
      return "bg-blue-100 text-blue-700 dark:bg-blue-500/15 dark:text-blue-300"
    default:
      return "bg-muted text-muted-foreground"
  }
}
