import { type FormEvent } from "react"
import { Copy, ExternalLink, Plus, RefreshCw, Trash2 } from "lucide-react"

import { formatTime } from "@/admin-format"
import { activeStatuses, logNames, type RunnerState, type RunnerStatus } from "@/admin-types"
import { Detail, StatusBadge } from "@/components/admin-shared"
import { sandboxConfigSourceLabel } from "@/components/sandbox-service-default-utils"
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
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { cn } from "@/lib/utils"

type LogName = (typeof logNames)[number]

export function RunnerRequestsSection({
  hasAccess,
  loading,
  runners,
  filteredRunners,
  selected,
  selectedID,
  selectedLog,
  logText,
  createID,
  createRepository,
  createRunnerSpec,
  createLabels,
  createRunnerOpen,
  runnerStatusFilter,
  runnerRepositoryFilter,
  runnerSpecFilter,
  runnerRepositories,
  runnerSpecNames,
  onRefresh,
  onResetCreateRunnerForm,
  onCreateRunnerOpenChange,
  onCreateRunnerSubmit,
  onCreateIDChange,
  onCreateRepositoryChange,
  onCreateRunnerSpecChange,
  onCreateLabelsChange,
  onStatusFilterChange,
  onRepositoryFilterChange,
  onRunnerSpecFilterChange,
  onSelectRunner,
  onRetryRunner,
  onStopRunner,
  onCopySelectedID,
  onLoadLog,
  onSelectedLogChange,
}: {
  hasAccess: boolean
  loading: boolean
  runners: RunnerState[]
  filteredRunners: RunnerState[]
  selected?: RunnerState
  selectedID: string
  selectedLog: LogName
  logText: string
  createID: string
  createRepository: string
  createRunnerSpec: string
  createLabels: string
  createRunnerOpen: boolean
  runnerStatusFilter: RunnerStatus | "all"
  runnerRepositoryFilter: string
  runnerSpecFilter: string
  runnerRepositories: string[]
  runnerSpecNames: string[]
  onRefresh: () => void
  onResetCreateRunnerForm: () => void
  onCreateRunnerOpenChange: (open: boolean) => void
  onCreateRunnerSubmit: (event: FormEvent<HTMLFormElement>) => void
  onCreateIDChange: (value: string) => void
  onCreateRepositoryChange: (value: string) => void
  onCreateRunnerSpecChange: (value: string) => void
  onCreateLabelsChange: (value: string) => void
  onStatusFilterChange: (value: RunnerStatus | "all") => void
  onRepositoryFilterChange: (value: string) => void
  onRunnerSpecFilterChange: (value: string) => void
  onSelectRunner: (id: string) => void
  onRetryRunner: (id: string) => void
  onStopRunner: (id: string) => void
  onCopySelectedID: () => void
  onLoadLog: (id: string, name: LogName) => void
  onSelectedLogChange: (name: LogName) => void
}) {
  return (
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
                    onResetCreateRunnerForm()
                    onCreateRunnerOpenChange(true)
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
                  onClick={onRefresh}
                  disabled={loading}
                  title="Refresh"
                >
                  <RefreshCw className={cn(loading && "animate-spin")} />
                </Button>
              </div>
            </div>
            <Dialog open={createRunnerOpen} onOpenChange={onCreateRunnerOpenChange}>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Create runner request</DialogTitle>
                  <DialogDescription>
                    Manually enqueue a one-off runner request.
                  </DialogDescription>
                </DialogHeader>
                <form className="grid gap-3" onSubmit={onCreateRunnerSubmit}>
                  <Input
                    value={createID}
                    onChange={(event) => onCreateIDChange(event.target.value)}
                    placeholder="optional id"
                  />
                  <Input
                    value={createRepository}
                    onChange={(event) => onCreateRepositoryChange(event.target.value)}
                    placeholder="owner/repo"
                    required
                  />
                  <Input
                    value={createRunnerSpec}
                    onChange={(event) => onCreateRunnerSpecChange(event.target.value)}
                    placeholder="optional runner spec"
                  />
                  <Input
                    value={createLabels}
                    onChange={(event) => onCreateLabelsChange(event.target.value)}
                    placeholder="self-hosted,e2b"
                  />
                  <DialogFooter>
                    <Button type="button" variant="outline" onClick={() => onCreateRunnerOpenChange(false)}>
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
              <Select value={runnerStatusFilter} onValueChange={(value) => onStatusFilterChange(value as RunnerStatus | "all")}>
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
              <Select value={runnerRepositoryFilter} onValueChange={onRepositoryFilterChange}>
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
              <Select value={runnerSpecFilter} onValueChange={onRunnerSpecFilterChange}>
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
                    onClick={() => onSelectRunner(runner.id)}
                  >
                    <TableCell>
                      <StatusBadge status={runner.status} />
                    </TableCell>
                    <TableCell>
                      <div className="max-w-[220px] truncate">{runner.repository_full_name || "-"}</div>
                    </TableCell>
                    <TableCell>{runner.runner_spec_name || "-"}</TableCell>
                    <TableCell className="max-w-[260px]">
                      <div className="truncate font-medium">{runner.runner_name || runner.id}</div>
                      <div className="truncate text-xs text-muted-foreground">{runner.id}</div>
                    </TableCell>
                    <TableCell>
                      <div className="max-w-[180px] truncate">{runner.sandbox_id || "-"}</div>
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
                              onRetryRunner(runner.id)
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
                              onStopRunner(runner.id)
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
              onClick={onCopySelectedID}
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
              <Detail label="Sandbox config" value={sandboxConfigSourceLabel(selected.sandbox_config_source)} />
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
              <Detail label="Workflow" value={selected.workflow_name || "-"} />
              <Detail label="Workflow attempt" value={selected.workflow_run_attempt || "-"} />
              <Detail label="Pull request" value={selected.pull_request_number || "-"} />
              <Detail label="Branch" value={selected.head_branch || "-"} />
              <Detail label="Commit" value={selected.head_sha || "-"} />
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
              <Button type="button" variant="outline" onClick={() => onRetryRunner(selected.id)}>
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
                  onClick={() => onLoadLog(selected.id, selectedLog)}
                >
                  <RefreshCw />
                  Refresh
                </Button>
              </div>
              <Tabs
                value={selectedLog}
                onValueChange={(value) => onSelectedLogChange(value as LogName)}
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
  )
}
