import { type FormEvent } from "react"
import { useTranslation } from "react-i18next"

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
  const { t } = useTranslation()
  return (
    <div className="grid gap-4 xl:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>{t("admin.recentRunnerRequests")}</CardTitle>
          <CardDescription>{t("admin.recentRunnerRequestsDescription")}</CardDescription>
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
            <div className="text-sm text-muted-foreground">{t("admin.noRunnerRequests")}</div>
          ) : null}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{t("admin.runnerSpecsAndPolicies")}</CardTitle>
          <CardDescription>{t("admin.routingRules")}</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4 lg:grid-cols-2">
          <div className="space-y-3">
            <div className="text-sm font-medium">{t("admin.runnerSpecs")}</div>
            {runnerSpecs.map((runnerSpec) => (
              <div
                key={runnerSpec.name}
                className="min-w-0 rounded-md border p-3 text-sm"
                onClick={() => onEditRunnerSpec(runnerSpec)}
              >
                <div className="truncate font-medium">{runnerSpec.name}</div>
                <div className="truncate text-xs text-muted-foreground">
                  {runnerSpec.labels.join(", ")} · {t("admin.namedTemplate", { id: runnerSpec.template_id })}
                </div>
              </div>
            ))}
          </div>
          <div className="space-y-3">
            <div className="text-sm font-medium">{t("admin.runnerPolicies")}</div>
            {runnerPolicies.map((policy) => (
              <div
                key={policy.id}
                className="min-w-0 rounded-md border p-3 text-sm"
                onClick={() => onEditPolicy(policy)}
              >
                <div className="truncate font-medium">{policy.repository_full_name}</div>
                <div className="truncate text-xs text-muted-foreground">{policy.runner_spec_name}</div>
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
  const { t } = useTranslation()
  return (
    <div className="grid gap-4 xl:grid-cols-[420px_minmax(0,1fr)]">
      <Card>
        <CardHeader>
          <CardTitle>{t("admin.labelMatchingTest")}</CardTitle>
          <CardDescription>{t("admin.labelMatchingDescription")}</CardDescription>
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
            <Button type="submit">{t("admin.runMatch")}</Button>
          </form>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{t("admin.matchResult")}</CardTitle>
          <CardDescription>{t("admin.matchResultDescription")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {matchResult ? (
            <>
              <Detail label={t("admin.repository")} value={matchResult.repository_full_name || "-"} />
              <Detail label={t("admin.labels")} value={matchResult.labels.join(", ") || "-"} />
              <Detail label={t("admin.runnerSpec")} value={matchResult.runner_spec?.name || "-"} />
              <Detail label={t("admin.reason")} value={matchResult.reason || t("admin.matched")} />
            </>
          ) : (
            <div className="text-sm text-muted-foreground">{t("admin.noMatchRun")}</div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

export function AuditSection({ auditEvents }: { auditEvents: AuditEvent[] }) {
  const { t, i18n } = useTranslation()
  return (
    <Card className="min-w-0">
      <CardHeader>
        <CardTitle>{t("admin.auditEvents")}</CardTitle>
        <CardDescription>{t("admin.auditDescription")}</CardDescription>
      </CardHeader>
      <CardContent className="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("admin.time")}</TableHead>
              <TableHead>{t("admin.actor")}</TableHead>
              <TableHead>{t("admin.action")}</TableHead>
              <TableHead>{t("admin.resource")}</TableHead>
              <TableHead>{t("admin.payload")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {auditEvents.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">
                  {t("admin.noAuditEvents")}
                </TableCell>
              </TableRow>
            ) : (
              auditEvents.map((event) => (
                <TableRow key={event.id}>
                  <TableCell>{formatTime(event.created_at, i18n.resolvedLanguage)}</TableCell>
                  <TableCell>{event.actor}</TableCell>
                  <TableCell>{event.action}</TableCell>
                  <TableCell>
                    {event.resource_type} · {event.resource_id}
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    <div className="max-w-[420px] truncate">{event.payload_json || "-"}</div>
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
  const { t } = useTranslation()
  return (
    <div className="grid gap-4 xl:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>{t("admin.diagnosticsSummary")}</CardTitle>
          <CardDescription>{t("admin.diagnosticsDescription")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <Detail label={t("admin.stateBackend")} value={diagnostics?.state.backend || "-"} />
          <Detail label={t("admin.database")} value={diagnostics?.state.database || "-"} />
          <Detail label={t("admin.githubAuth")} value={diagnostics?.github.auth_mode || "-"} />
          <Detail label={t("admin.installation")} value={diagnostics?.github.installation_id || "-"} />
          <Detail label={t("admin.githubAPI")} value={diagnostics?.github.api_base_url || "-"} />
          <div className="space-y-2">
            <div className="text-sm font-medium">{t("admin.pprofEndpoints")}</div>
            {diagnostics?.pprof?.length ? (
              diagnostics.pprof.map((item) => (
                <div key={item.address_file} className="rounded-md border p-3 text-xs">
                  <div className="font-medium">{item.address}</div>
                  <div className="text-muted-foreground">{item.address_file}</div>
                  <div className="text-muted-foreground">{item.dump_script}</div>
                </div>
              ))
            ) : (
              <div className="text-sm text-muted-foreground">{t("admin.noPprof")}</div>
            )}
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{t("admin.recentFailures")}</CardTitle>
          <CardDescription>{t("admin.failuresDescription")}</CardDescription>
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
              <div className="text-sm text-muted-foreground">{t("admin.noRecentFailures")}</div>
            )}
          </div>
          <pre className="max-h-[48vh] min-h-72 overflow-auto rounded-lg border bg-muted/50 p-3 text-xs leading-relaxed whitespace-pre-wrap">
            {diagnosticsVars || t("admin.noDebugVars")}
          </pre>
        </CardContent>
      </Card>
    </div>
  )
}
