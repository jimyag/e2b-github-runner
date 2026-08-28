import { Copy, Loader2, Plus, RotateCcw } from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import type { UserRunnerSpec } from "@/admin-types"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

type RequestFn = (url: string, options?: RequestInit) => Promise<unknown>

export function UserRunnerTypesSection({ request, installationID }: { request: RequestFn; installationID?: number }) {
  const { t } = useTranslation()
  const [items, setItems] = useState<UserRunnerSpec[]>([])
  const [sandboxSource, setSandboxSource] = useState("none")
  const [loading, setLoading] = useState(true)
  const [copying, setCopying] = useState<string | null>(null)
  const query = installationID ? `?installation_id=${installationID}` : ""

  const load = async () => {
    setLoading(true)
    try {
      const response = (await request(`/user/runner-specs${query}`)) as { items?: UserRunnerSpec[]; sandbox_source?: string }
      setItems(response.items || [])
      setSandboxSource(response.sandbox_source || "none")
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    void request(`/user/runner-specs${query}`).then((response) => {
      if (cancelled) return
      const data = response as { items?: UserRunnerSpec[]; sandbox_source?: string }
      setItems(data.items || [])
      setSandboxSource(data.sandbox_source || "none")
    }).catch(() => {
      if (!cancelled) setItems([])
    }).finally(() => {
      if (!cancelled) setLoading(false)
    })
    return () => { cancelled = true }
  }, [query, request])

  const copyYAML = async (item: UserRunnerSpec) => {
    setCopying(item.name)
    try {
      await navigator.clipboard?.writeText(`runs-on: [${item.workflow_labels.join(", ")}]`)
    } finally {
      setCopying(null)
    }
  }

  const canCreate = sandboxSource !== "none" && sandboxSource !== "admin_default"
  const sorted = useMemo(() => [...items].sort((a, b) => a.name.localeCompare(b.name)), [items])

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-3">
        <CardTitle>{t("user.runnerTypes")}</CardTitle>
        <Button type="button" size="sm" disabled={!canCreate} title={!canCreate ? t("user.runnerTypesConfigureSandbox") : undefined}>
          <Plus className="h-4 w-4" />{t("user.createRunnerType")}
        </Button>
      </CardHeader>
      <CardContent>
        {loading ? <div className="flex items-center gap-2 text-sm text-muted-foreground"><Loader2 className="h-4 w-4 animate-spin" />{t("common.loading")}</div> : null}
        {!loading && sorted.length === 0 ? <p className="text-sm text-muted-foreground">{t("user.noRunnerTypes")}</p> : null}
        <div className="grid gap-3">
          {sorted.map((item) => (
            <div key={`${item.source}:${item.name}`} className="flex min-w-0 flex-wrap items-center justify-between gap-3 rounded-md border p-3">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2"><span className="font-medium">{item.name}</span><Badge variant={item.enabled ? "default" : "secondary"}>{item.enabled ? t("common.enabled") : t("common.disabled")}</Badge><Badge variant="outline">{item.source}</Badge></div>
                <div className="mt-1 break-words font-mono text-xs text-muted-foreground">{t("user.runsOn")}: [{item.workflow_labels.join(", ")}]</div>
              </div>
              <div className="flex items-center gap-2">
                <Button type="button" size="icon" variant="ghost" onClick={() => void copyYAML(item)} disabled={copying === item.name} title={t("user.copyRunnerTypeYAML")}><Copy className="h-4 w-4" /></Button>
                {item.source === "managed" ? <Button type="button" size="icon" variant="ghost" title={t("user.resetRunnerTypeControl")}><RotateCcw className="h-4 w-4" /></Button> : null}
              </div>
            </div>
          ))}
        </div>
        <Button type="button" variant="link" className="mt-3 px-0" onClick={() => void load()} disabled={loading}>{t("common.refresh")}</Button>
      </CardContent>
    </Card>
  )
}
