import { type ReactNode, useCallback, useEffect, useRef, useState } from "react"
import { RefreshCw } from "lucide-react"
import { useTranslation } from "react-i18next"

import type { SandboxInstance, SandboxTemplate } from "@/admin-types"
import appI18n from "@/i18n"
import {
  formatOptionalTime,
  loadSandboxInstances,
  loadSandboxTemplates,
  sandboxRegions,
  sandboxInstancesViewState,
  type SandboxCatalogRequest,
} from "@/components/sandbox-catalog-utils"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"

function Header({
  title,
  description,
  region,
  loading,
  onRegion,
  onRefresh,
  children,
}: {
  title: string
  description: string
  region: string
  loading: boolean
  onRegion: (value: string) => void
  onRefresh: () => void
  children?: ReactNode
}) {
  const { t } = useTranslation()
  return (
    <CardHeader className="flex flex-col gap-3 pb-3 2xl:flex-row 2xl:items-center 2xl:justify-between">
      <div>
        <CardTitle className="text-base">{title}</CardTitle>
        <CardDescription className="mt-1">{description}</CardDescription>
      </div>
      <div className="flex w-full flex-wrap justify-end gap-2 2xl:w-auto">
        <Select value={region} onValueChange={onRegion}>
          <SelectTrigger className="min-w-[200px] max-w-[280px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {sandboxRegions.map((item) => (
              <SelectItem key={item.id} value={item.id}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {children}
        <Button variant="outline" size="icon" onClick={onRefresh} disabled={loading} aria-label={t("common.refresh")}>
          <RefreshCw className={loading ? "animate-spin" : ""} />
        </Button>
      </div>
    </CardHeader>
  )
}

export function SandboxTemplatesSection({
  request,
  installationID,
}: {
  request: SandboxCatalogRequest
  installationID?: number
}) {
  const { t } = useTranslation()
  const [region, setRegion] = useState(sandboxRegions[0].id)
  const [items, setItems] = useState<SandboxTemplate[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState("")
  const loadGeneration = useRef(0)

  const load = useCallback(async () => {
    const generation = ++loadGeneration.current
    setLoading(true)
    setError("")
    setItems([])
    try {
      const data = await loadSandboxTemplates(request, region, installationID)
      if (generation === loadGeneration.current) {
        setItems(data)
      }
    } catch (cause) {
      if (generation === loadGeneration.current) {
        setError(cause instanceof Error ? cause.message : appI18n.t("user.loadSandboxTemplatesFailed"))
      }
    } finally {
      if (generation === loadGeneration.current) {
        setLoading(false)
      }
    }
  }, [installationID, region, request])

  useEffect(() => {
    void load()
    return () => {
      loadGeneration.current += 1
    }
  }, [load])

  return (
    <Card className="rounded-lg">
      <Header
        title={t("user.sandboxTemplates")}
        description={t("user.catalogTemplatesDescription")}
        region={region}
        loading={loading}
        onRegion={setRegion}
        onRefresh={() => void load()}
      />
      <CardContent>
        {error ? (
          <p className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>
        ) : (
          <div className="overflow-x-auto rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("common.template")}</TableHead>
                  <TableHead>{t("common.status")}</TableHead>
                  <TableHead>{t("common.resources")}</TableHead>
                  <TableHead>{t("common.visibility")}</TableHead>
                  <TableHead className="text-right">{t("user.spawns")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((item) => (
                  <TableRow key={item.template_id}>
                    <TableCell>
                      <div className="font-medium">{item.aliases?.[0] || item.template_id}</div>
                      <div className="max-w-[360px] truncate text-xs text-muted-foreground">{item.template_id}</div>
                    </TableCell>
                    <TableCell>{item.build_status || t("common.unknown")}</TableCell>
                    <TableCell>
                      {t("user.catalogTemplateResources", {
                        cpu: item.cpu_count,
                        memory: item.memory_mb,
                        disk: item.disk_size_mb,
                      })}
                    </TableCell>
                    <TableCell>{item.public ? t("common.public") : t("common.private")}</TableCell>
                    <TableCell className="text-right tabular-nums">{item.spawn_count}</TableCell>
                  </TableRow>
                ))}
                {!loading && items.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={5} className="h-32 text-center text-muted-foreground">
                      {t("user.noTemplatesInRegion")}
                    </TableCell>
                  </TableRow>
                ) : null}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

export function SandboxesSection({
  request,
  installationID,
}: {
  request: SandboxCatalogRequest
  installationID?: number
}) {
  const { t, i18n } = useTranslation()
  const [region, setRegion] = useState(sandboxRegions[0].id)
  const [template, setTemplate] = useState("all")
  const [templates, setTemplates] = useState<SandboxTemplate[]>([])
  const [items, setItems] = useState<SandboxInstance[]>([])
  const [templatesLoading, setTemplatesLoading] = useState(false)
  const [instancesLoading, setInstancesLoading] = useState(false)
  const [templatesError, setTemplatesError] = useState("")
  const [instancesError, setInstancesError] = useState("")
  const templateLoadGeneration = useRef(0)
  const instanceLoadGeneration = useRef(0)

  const loadTemplates = useCallback(async () => {
    const generation = ++templateLoadGeneration.current
    setTemplatesLoading(true)
    setTemplatesError("")
    setTemplates([])
    try {
      const data = await loadSandboxTemplates(request, region, installationID)
      if (generation === templateLoadGeneration.current) {
        setTemplates(data)
      }
    } catch (cause) {
      if (generation === templateLoadGeneration.current) {
        setTemplatesError(cause instanceof Error ? cause.message : appI18n.t("user.loadSandboxTemplatesFailed"))
      }
    } finally {
      if (generation === templateLoadGeneration.current) {
        setTemplatesLoading(false)
      }
    }
  }, [installationID, region, request])

  const loadInstances = useCallback(async () => {
    const generation = ++instanceLoadGeneration.current
    setInstancesLoading(true)
    setInstancesError("")
    setItems([])
    try {
      const templateID = template === "all" ? "" : template
      const data = await loadSandboxInstances(request, region, installationID, templateID)
      if (generation === instanceLoadGeneration.current) {
        setItems(data)
      }
    } catch (cause) {
      if (generation === instanceLoadGeneration.current) {
        setInstancesError(cause instanceof Error ? cause.message : appI18n.t("user.loadSandboxesFailed"))
      }
    } finally {
      if (generation === instanceLoadGeneration.current) {
        setInstancesLoading(false)
      }
    }
  }, [installationID, region, request, template])

  useEffect(() => {
    void loadTemplates()
    return () => {
      templateLoadGeneration.current += 1
    }
  }, [loadTemplates])

  useEffect(() => {
    void loadInstances()
    return () => {
      instanceLoadGeneration.current += 1
    }
  }, [loadInstances])

  const { loading, error, filterDisabled } = sandboxInstancesViewState({
    templatesLoading,
    instancesLoading,
    templatesError,
    instancesError,
  })

  return (
    <Card className="rounded-lg">
      <Header
        title={t("user.sandboxInstances")}
        description={t("user.catalogInstancesDescription")}
        region={region}
        loading={loading}
        onRegion={(value) => {
          setRegion(value)
          setTemplate("all")
        }}
        onRefresh={() => {
          void loadTemplates()
          void loadInstances()
        }}
      >
        <Select value={template} onValueChange={setTemplate} disabled={filterDisabled}>
          <SelectTrigger className="min-w-[200px] max-w-[280px]">
            <SelectValue placeholder={t("user.filterTemplate")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("user.allTemplates")}</SelectItem>
            {templates.map((item) => (
              <SelectItem key={item.template_id} value={item.template_id}>
                {item.aliases?.[0] || item.template_id}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Header>
      <CardContent>
        {error ? (
          <p className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>
        ) : (
          <div className="overflow-x-auto rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("common.sandbox")}</TableHead>
                  <TableHead>{t("user.state")}</TableHead>
                  <TableHead>{t("common.template")}</TableHead>
                  <TableHead>{t("common.resources")}</TableHead>
                  <TableHead>{t("common.started")}</TableHead>
                  <TableHead>{t("common.expires")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((item) => (
                  <TableRow key={item.sandbox_id}>
                    <TableCell>
                      <div className="font-medium">{item.alias || item.sandbox_id}</div>
                      <div className="max-w-[300px] truncate text-xs text-muted-foreground">{item.sandbox_id}</div>
                    </TableCell>
                    <TableCell>{item.state}</TableCell>
                    <TableCell>
                      <div className="max-w-[260px] truncate">{item.template_id}</div>
                    </TableCell>
                    <TableCell>
                      {t("user.catalogInstanceResources", {
                        cpu: item.cpu_count,
                        memory: item.memory_mb,
                      })}
                    </TableCell>
                    <TableCell>{formatOptionalTime(item.started_at, i18n.resolvedLanguage)}</TableCell>
                    <TableCell>{formatOptionalTime(item.expires_at, i18n.resolvedLanguage)}</TableCell>
                  </TableRow>
                ))}
                {!loading && items.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={6} className="h-32 text-center text-muted-foreground">
                      {t("user.noSandboxesForFilters")}
                    </TableCell>
                  </TableRow>
                ) : null}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
