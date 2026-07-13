import { type ReactNode, useCallback, useEffect, useRef, useState } from "react"
import { RefreshCw } from "lucide-react"

import type { SandboxInstance, SandboxTemplate } from "@/admin-types"
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
  return (
    <CardHeader className="flex flex-col gap-4 border-b 2xl:flex-row 2xl:items-center 2xl:justify-between">
      <div>
        <CardTitle>{title}</CardTitle>
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
        <Button variant="outline" size="icon" onClick={onRefresh} disabled={loading} aria-label="Refresh">
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
        setError(cause instanceof Error ? cause.message : "Failed to load sandbox templates")
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
    <Card className="overflow-hidden">
      <Header
        title="Sandbox templates"
        description="Runnable images available to runner specs in the selected region."
        region={region}
        loading={loading}
        onRegion={setRegion}
        onRefresh={() => void load()}
      />
      <CardContent className="p-0">
        {error ? (
          <p className="p-6 text-sm text-destructive">{error}</p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Template</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Resources</TableHead>
                <TableHead>Visibility</TableHead>
                <TableHead className="text-right">Spawns</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item) => (
                <TableRow key={item.template_id}>
                  <TableCell>
                    <div className="font-medium">{item.aliases?.[0] || item.template_id}</div>
                    <div className="max-w-[360px] truncate text-xs text-muted-foreground">{item.template_id}</div>
                  </TableCell>
                  <TableCell>{item.build_status || "unknown"}</TableCell>
                  <TableCell>
                    {item.cpu_count} CPU · {item.memory_mb} MB · {item.disk_size_mb} MB disk
                  </TableCell>
                  <TableCell>{item.public ? "Public" : "Private"}</TableCell>
                  <TableCell className="text-right tabular-nums">{item.spawn_count}</TableCell>
                </TableRow>
              ))}
              {!loading && items.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="h-32 text-center text-muted-foreground">
                    No templates in this region.
                  </TableCell>
                </TableRow>
              ) : null}
            </TableBody>
          </Table>
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
        setTemplatesError(cause instanceof Error ? cause.message : "Failed to load sandbox templates")
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
        setInstancesError(cause instanceof Error ? cause.message : "Failed to load sandboxes")
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
    <Card className="overflow-hidden">
      <Header
        title="Sandbox instances"
        description="Live and recent sandbox capacity in the selected region."
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
            <SelectValue placeholder="Filter by template" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All templates</SelectItem>
            {templates.map((item) => (
              <SelectItem key={item.template_id} value={item.template_id}>
                {item.aliases?.[0] || item.template_id}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Header>
      <CardContent className="p-0">
        {error ? (
          <p className="p-6 text-sm text-destructive">{error}</p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Sandbox</TableHead>
                <TableHead>State</TableHead>
                <TableHead>Template</TableHead>
                <TableHead>Resources</TableHead>
                <TableHead>Started</TableHead>
                <TableHead>Expires</TableHead>
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
                    {item.cpu_count} CPU · {item.memory_mb} MB
                  </TableCell>
                  <TableCell>{formatOptionalTime(item.started_at)}</TableCell>
                  <TableCell>{formatOptionalTime(item.expires_at)}</TableCell>
                </TableRow>
              ))}
              {!loading && items.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6} className="h-32 text-center text-muted-foreground">
                    No sandboxes match these filters.
                  </TableCell>
                </TableRow>
              ) : null}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  )
}
