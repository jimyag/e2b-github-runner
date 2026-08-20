import { useCallback, useEffect, useRef, useState } from "react"
import { AlertTriangle, CheckCircle2, ExternalLink, Loader2, RefreshCw } from "lucide-react"
import { useTranslation } from "react-i18next"

import {
  type CatalogMigrationGateCode,
  type CatalogMigrationManualRequirement,
  type CatalogMigrationReadiness,
  type CatalogMatchReplaySample,
} from "@/admin-types"
import { formatTime } from "@/admin-format"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

type RequestFunction = (url: string, options?: RequestInit) => Promise<unknown>

const evidenceWindows = [
  { hours: 72, key: "admin.releaseWindow72Hours" },
  { hours: 168, key: "admin.releaseWindow7Days" },
  { hours: 720, key: "admin.releaseWindow30Days" },
] as const

const gateTranslationKeys = {
  window_at_least_72_hours: "admin.releaseGateWindow",
  catalog_unchanged: "admin.releaseGateCatalog",
  matcher_parity: "admin.releaseGateParity",
  all_enabled_specs_full_lifecycle: "admin.releaseGateLifecycle",
} as const satisfies Record<CatalogMigrationGateCode, string>

const manualRequirementTranslationKeys = {
  backup_restore_verified: "admin.releaseManualBackup",
  continuous_service_observation: "admin.releaseManualContinuity",
  workflow_labels_unchanged: "admin.releaseManualWorkflowLabels",
} as const satisfies Record<CatalogMigrationManualRequirement, string>

const replayResultTranslationKeys = {
  legacy_only: "admin.releaseResultLegacyOnly",
  enabled_only: "admin.releaseResultEnabledOnly",
  different_profile: "admin.releaseResultDifferentProfile",
  error: "admin.releaseResultError",
} as const satisfies Record<CatalogMatchReplaySample["result"], string>

export function ReleaseReadinessSection({ request }: { request: RequestFunction }) {
  const { t, i18n } = useTranslation()
  const [windowHours, setWindowHours] = useState(72)
  const [report, setReport] = useState<CatalogMigrationReadiness | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState("")
  const requestGeneration = useRef(0)

  const load = useCallback(async () => {
    const generation = ++requestGeneration.current
    setLoading(true)
    setError("")
    try {
      const next = await request(`/diagnostics/catalog-migration-readiness?window_hours=${windowHours}`)
      if (generation !== requestGeneration.current) return
      setReport(next as CatalogMigrationReadiness)
    } catch (loadError) {
      if (generation !== requestGeneration.current) return
      setError(loadError instanceof Error ? loadError.message : t("admin.releaseReadinessLoadFailed"))
    } finally {
      if (generation === requestGeneration.current) setLoading(false)
    }
  }, [request, t, windowHours])

  useEffect(() => {
    void load()
  }, [load])

  return (
    <section className="space-y-4" aria-labelledby="release-readiness-title">
      <Card>
        <CardHeader className="gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="space-y-1.5">
            <div className="flex flex-wrap items-center gap-2">
              <CardTitle id="release-readiness-title">{t("admin.releaseReadinessTitle")}</CardTitle>
              {report ? (
                <Badge variant={report.automated_gates_passed ? "default" : "destructive"}>
                  {report.automated_gates_passed
                    ? t("admin.releaseAutomatedPassed")
                    : t("admin.releaseAutomatedBlocked")}
                </Badge>
              ) : null}
            </div>
            <CardDescription>{t("admin.releaseReadinessDescription")}</CardDescription>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {evidenceWindows.map((window) => (
              <Button
                key={window.hours}
                type="button"
                size="sm"
                variant={windowHours === window.hours ? "default" : "outline"}
                aria-pressed={windowHours === window.hours}
                onClick={() => setWindowHours(window.hours)}
              >
                {t(window.key)}
              </Button>
            ))}
            <Button type="button" size="sm" variant="outline" disabled={loading} onClick={() => void load()}>
              {loading ? <Loader2 className="animate-spin" aria-hidden="true" /> : <RefreshCw aria-hidden="true" />}
              {t("common.refresh")}
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="rounded-md border border-amber-500/35 bg-amber-500/8 p-3 text-sm">
            <div className="font-medium">{t("admin.releaseManualRequired")}</div>
            <div className="mt-1 text-muted-foreground">{t("admin.releaseManualDescription")}</div>
          </div>
          {error ? (
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive" role="alert">
              <span>{error}</span>
              <Button type="button" size="sm" variant="outline" onClick={() => void load()}>
                {t("admin.releaseRetry")}
              </Button>
            </div>
          ) : null}
          {loading && !report ? (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="size-4 animate-spin" aria-hidden="true" />
              {t("common.loading")}
            </div>
          ) : null}
          {report ? (
            <>
              <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                {report.gates.map((gate) => (
                  <div key={gate.code} className="rounded-md border p-3">
                    <div className="flex items-center gap-2 text-sm font-medium">
                      {gate.passed
                        ? <CheckCircle2 className="size-4 text-emerald-600" aria-hidden="true" />
                        : <AlertTriangle className="size-4 text-destructive" aria-hidden="true" />}
                      {t(gateTranslationKeys[gate.code])}
                    </div>
                    <div className="mt-1 text-xs text-muted-foreground">
                      {gate.passed ? t("admin.releasePassed") : t("admin.releaseBlocked")}
                    </div>
                  </div>
                ))}
              </div>
              <div className="grid gap-4 xl:grid-cols-2">
                <ParityCard report={report} />
                <ManualRequirementsCard report={report} />
              </div>
              {report.replay_samples.length ? <ReplaySamplesCard report={report} /> : null}
            </>
          ) : null}
        </CardContent>
      </Card>

      {report ? (
        <>
          <Card>
            <CardHeader>
              <CardTitle>{t("admin.releaseSpecEvidence")}</CardTitle>
              <CardDescription>{t("admin.releaseSpecEvidenceDescription")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="overflow-x-auto rounded-md border">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t("admin.runnerSpec")}</TableHead>
                      <TableHead>{t("admin.releaseWorkflowLabels")}</TableHead>
                      <TableHead className="text-right">{t("admin.releaseRequests")}</TableHead>
                      <TableHead className="text-right">{t("admin.releaseRegistered")}</TableHead>
                      <TableHead className="text-right">{t("admin.releaseCompleted")}</TableHead>
                      <TableHead className="text-right">{t("admin.releaseCleanupFinalized")}</TableHead>
                      <TableHead>{t("admin.releaseLatestEvidence")}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {report.specs.map((spec) => (
                      <TableRow key={spec.name}>
                        <TableCell>
                          <div className="flex items-center gap-2 font-medium">
                            {spec.cleanup_finalized_requests > 0
                              ? <CheckCircle2 className="size-4 text-emerald-600" aria-hidden="true" />
                              : <AlertTriangle className="size-4 text-destructive" aria-hidden="true" />}
                            {spec.name}
                          </div>
                        </TableCell>
                        <TableCell className="font-mono text-xs">{spec.workflow_labels.join(", ") || "-"}</TableCell>
                        <TableCell className="text-right tabular-nums">{spec.request_count}</TableCell>
                        <TableCell className="text-right tabular-nums">{spec.registered_requests}</TableCell>
                        <TableCell className="text-right tabular-nums">{spec.completed_requests}</TableCell>
                        <TableCell className="text-right tabular-nums">{spec.cleanup_finalized_requests}</TableCell>
                        <TableCell>
                          {spec.latest ? (
                            <div className="space-y-1 text-xs">
                              {spec.latest.github_job_url ? (
                                <a className="inline-flex items-center gap-1 text-primary hover:underline" href={spec.latest.github_job_url} target="_blank" rel="noreferrer">
                                  {spec.latest.request_id}
                                  <ExternalLink className="size-3" aria-hidden="true" />
                                </a>
                              ) : <span>{spec.latest.request_id}</span>}
                              <div className="text-muted-foreground">{formatTime(spec.latest.cleanup_finalized_at, i18n.resolvedLanguage)}</div>
                            </div>
                          ) : <span className="text-xs text-muted-foreground">{t("admin.releaseMissingEvidence")}</span>}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
              <p className="text-xs leading-5 text-muted-foreground">{t("admin.releaseCleanupMeaning")}</p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t("admin.releaseCatalogChanges")}</CardTitle>
              <CardDescription>{t("admin.releaseCatalogChangesDescription")}</CardDescription>
            </CardHeader>
            <CardContent>
              {report.catalog_changes_truncated ? (
                <div className="mb-3 text-sm text-destructive">{t("admin.releaseCatalogChangesTruncated")}</div>
              ) : null}
              {report.catalog_changes.length ? (
                <div className="space-y-2">
                  {report.catalog_changes.map((event) => (
                    <div key={event.id} className="rounded-md border p-3 text-sm">
                      <div className="font-medium">{event.action}</div>
                      <div className="mt-1 text-xs text-muted-foreground">
                        {event.resource_type} · {event.resource_id} · {formatTime(event.created_at, i18n.resolvedLanguage)}
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="text-sm text-muted-foreground">{t("admin.releaseNoCatalogChanges")}</div>
              )}
            </CardContent>
          </Card>
        </>
      ) : null}
    </section>
  )
}

function ReplaySamplesCard({ report }: { report: CatalogMigrationReadiness }) {
  const { t } = useTranslation()
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("admin.releaseMismatchSamples")}</CardTitle>
        <CardDescription>{t("admin.releaseMismatchSamplesDescription")}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="overflow-x-auto rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("admin.repository")}</TableHead>
                <TableHead>{t("admin.releaseWorkflowLabels")}</TableHead>
                <TableHead>{t("admin.releaseResult")}</TableHead>
                <TableHead>{t("admin.releaseLegacyMatch")}</TableHead>
                <TableHead>{t("admin.releaseEnabledMatch")}</TableHead>
                <TableHead className="text-right">{t("admin.releaseRequests")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {report.replay_samples.map((sample, index) => (
                <TableRow key={`${sample.repository_full_name}-${sample.result}-${index}`}>
                  <TableCell className="font-medium">{sample.repository_full_name}</TableCell>
                  <TableCell className="font-mono text-xs">{sample.labels?.join(", ") || "-"}</TableCell>
                  <TableCell>
                    <Badge variant="destructive">{t(replayResultTranslationKeys[sample.result])}</Badge>
                    {sample.error ? <div className="mt-1 max-w-80 text-xs text-destructive">{sample.error}</div> : null}
                  </TableCell>
                  <TableCell className="text-xs">
                    <div>{sample.legacy_profile || "-"}</div>
                    {sample.legacy_reason ? <div className="text-muted-foreground">{sample.legacy_reason}</div> : null}
                  </TableCell>
                  <TableCell className="text-xs">
                    <div>{sample.enabled_profile || "-"}</div>
                    {sample.enabled_reason ? <div className="text-muted-foreground">{sample.enabled_reason}</div> : null}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">{sample.request_count}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </CardContent>
    </Card>
  )
}

function ParityCard({ report }: { report: CatalogMigrationReadiness }) {
  const { t, i18n } = useTranslation()
  const historical = [
    ["same", report.replay.same],
    ["legacy_only", report.replay.legacy_only],
    ["enabled_only", report.replay.enabled_only],
    ["different_profile", report.replay.different_profile],
    ["errors", report.replay.errors],
  ] as const
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("admin.releaseHistoricalReplay")}</CardTitle>
        <CardDescription>
          {t("admin.releaseHistoricalReplayDescription", {
            requests: report.replay.request_count,
            inputs: report.replay.distinct_input_count,
          })}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-5">
          {historical.map(([key, value]) => (
            <div key={key} className="rounded-md border p-2 text-center">
              <div className="text-lg font-semibold tabular-nums">{value}</div>
              <div className="text-[11px] text-muted-foreground">{key}</div>
            </div>
          ))}
        </div>
        {report.replay.truncated ? <div className="text-xs text-destructive">{t("admin.releaseReplayTruncated")}</div> : null}
        <div className="text-xs text-muted-foreground">
          {formatTime(report.window_start, i18n.resolvedLanguage)} – {formatTime(report.window_end, i18n.resolvedLanguage)}
        </div>
      </CardContent>
    </Card>
  )
}

function ManualRequirementsCard({ report }: { report: CatalogMigrationReadiness }) {
  const { t, i18n } = useTranslation()
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("admin.releaseCurrentProcess")}</CardTitle>
        <CardDescription>
          {t("admin.releaseCurrentProcessDescription", {
            time: formatTime(report.current_process.started_at, i18n.resolvedLanguage),
          })}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
          {Object.entries(report.current_process.catalog_match_counts).map(([key, value]) => (
            <div key={key} className="rounded-md border p-2 text-center">
              <div className="text-lg font-semibold tabular-nums">{value}</div>
              <div className="text-[11px] text-muted-foreground">{key}</div>
            </div>
          ))}
        </div>
        <div className="space-y-2">
          {report.manual_requirements.map((requirement) => (
            <div key={requirement} className="flex items-start gap-2 text-sm text-muted-foreground">
              <AlertTriangle className="mt-0.5 size-4 shrink-0 text-amber-600" aria-hidden="true" />
              {t(manualRequirementTranslationKeys[requirement])}
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}
