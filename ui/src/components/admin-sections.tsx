import { type FormEvent } from "react"

import {
  type AuditEvent,
  type DiagnosticsSummary,
  type RunnerPolicy,
  type RunnerSpec,
  type RunnerSpecMatch,
  type RunnerState,
} from "@/admin-types"
import { formatTime } from "@/admin-format"
import { Detail, StatusBadge } from "@/components/admin-shared"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

export function OverviewSection({
  runners,
  runnerSpecs,
  runnerPolicies,
  onEditRunnerSpec,
  onEditPolicy,
}: {
  runners: RunnerState[]
  runnerSpecs: RunnerSpec[]
  runnerPolicies: RunnerPolicy[]
  onEditRunnerSpec: (runnerSpec: RunnerSpec) => void
  onEditPolicy: (policy: RunnerPolicy) => void
}) {
  return (
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
                onClick={() => onEditRunnerSpec(runnerSpec)}
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
                onClick={() => onEditPolicy(policy)}
              >
                <div className="font-medium">{policy.repository_full_name}</div>
                <div className="text-xs text-muted-foreground">{policy.runner_spec_name}</div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

export function MatchSection({
  matchRepository,
  matchLabels,
  matchResult,
  onRepositoryChange,
  onLabelsChange,
  onSubmit,
}: {
  matchRepository: string
  matchLabels: string
  matchResult: RunnerSpecMatch | null
  onRepositoryChange: (value: string) => void
  onLabelsChange: (value: string) => void
  onSubmit: (event: FormEvent<HTMLFormElement>) => void
}) {
  return (
    <div className="grid gap-4 xl:grid-cols-[420px_minmax(0,1fr)]">
      <Card>
        <CardHeader>
          <CardTitle>Label matching test</CardTitle>
          <CardDescription>Preview which runner spec a repository and label set would use.</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="grid gap-3" onSubmit={onSubmit}>
            <Input
              value={matchRepository}
              onChange={(event) => onRepositoryChange(event.target.value)}
              placeholder="owner/repo"
            />
            <Input
              value={matchLabels}
              onChange={(event) => onLabelsChange(event.target.value)}
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
  )
}

export function AuditSection({ auditEvents }: { auditEvents: AuditEvent[] }) {
  return (
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
  )
}

export function DiagnosticsSection({
  diagnostics,
  diagnosticsVars,
}: {
  diagnostics: DiagnosticsSummary | null
  diagnosticsVars: string
}) {
  return (
    <div className="grid gap-4 xl:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>Diagnostics summary</CardTitle>
          <CardDescription>DB, GitHub auth, recent failures, and pprof discovery.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <Detail label="State backend" value={diagnostics?.state.backend || "-"} />
          <Detail label="Database" value={diagnostics?.state.database || "-"} />
          <Detail label="GitHub auth" value={diagnostics?.github.auth_mode || "-"} />
          <Detail label="Installation" value={diagnostics?.github.installation_id || "-"} />
          <Detail label="GitHub API" value={diagnostics?.github.api_base_url || "-"} />
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
  )
}
