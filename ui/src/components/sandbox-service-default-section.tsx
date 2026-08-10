import { type FormEvent, useCallback, useEffect, useMemo, useState } from "react"
import { Building2, KeyRound, Plus, RefreshCw, Save, ShieldCheck, Trash2, UserRound, X } from "lucide-react"
import { toast } from "sonner"
import { useTranslation } from "react-i18next"

import { formatTime } from "@/admin-format"
import type { SandboxServiceDefault } from "@/admin-types"
import appI18n from "@/i18n"
import { sandboxRegions } from "@/components/sandbox-catalog-utils"
import {
  availableSandboxAudienceAccounts,
  normalizeSandboxAudienceLogin,
  sandboxServiceDefaultAPIURL,
  sandboxServiceDefaultStatus,
} from "@/components/sandbox-service-default-utils"
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
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"

type Request = (url: string, options?: RequestInit) => Promise<unknown>

const emptyConfig: SandboxServiceDefault = {
  enabled: false,
  configured: false,
  audience_mode: "all",
  audiences: [],
  available_accounts: [],
  api_url: "",
  api_key: { configured: false },
}

function normalizeAPIURL(value: string) {
  return value.trim().replace(/\/+$/, "").toLowerCase()
}

function regionForAPIURL(value: string) {
  const normalized = normalizeAPIURL(value)
  return sandboxRegions.find((region) => normalizeAPIURL(region.apiURL) === normalized)
}

export function SandboxServiceDefaultSection({ request }: { request: Request }) {
  const { t, i18n } = useTranslation()
  const [config, setConfig] = useState<SandboxServiceDefault>(emptyConfig)
  const [enabled, setEnabled] = useState(false)
  const [audienceMode, setAudienceMode] = useState<"all" | "selected">("all")
  const [candidateLogin, setCandidateLogin] = useState("")
  const [apiURL, setAPIURL] = useState("")
  const [apiKey, setAPIKey] = useState("")
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [audienceMutating, setAudienceMutating] = useState(false)
  const [removeOpen, setRemoveOpen] = useState(false)
  const [error, setError] = useState("")

  const applyConfig = useCallback((next: SandboxServiceDefault) => {
    const normalized = {
      ...emptyConfig,
      ...(next || {}),
      audiences: next?.audiences ?? [],
      available_accounts: next?.available_accounts ?? [],
    }
    setConfig(normalized)
    setEnabled(Boolean(normalized.enabled))
    setAudienceMode(normalized.audience_mode === "selected" ? "selected" : "all")
    setCandidateLogin("")
    setAPIURL(sandboxServiceDefaultAPIURL(normalized.api_url || "", sandboxRegions))
    setAPIKey("")
  }, [])

  const load = useCallback(async () => {
    setLoading(true)
    setError("")
    try {
      applyConfig((await request("/admin/api/sandbox-service-default")) as SandboxServiceDefault)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : appI18n.t("admin.loadSandboxDefaultFailed"))
    } finally {
      setLoading(false)
    }
  }, [applyConfig, request])

  useEffect(() => {
    void load()
  }, [load])

  const selectedRegion = useMemo(() => regionForAPIURL(apiURL), [apiURL])
  const availableAccounts = useMemo(
    () => availableSandboxAudienceAccounts(config.available_accounts, config.audiences),
    [config.available_accounts, config.audiences],
  )
  const status = sandboxServiceDefaultStatus(config)
  const busy = loading || saving || deleting || audienceMutating

  const save = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (enabled && !apiURL.trim()) {
      setError(t("admin.sandboxDefaultRegionRequired"))
      return
    }
    if (enabled && !config.api_key.configured && !apiKey.trim()) {
      setError(t("admin.sandboxDefaultAPIKeyRequired"))
      return
    }
    setSaving(true)
    setError("")
    try {
      const next = (await request("/admin/api/sandbox-service-default", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          enabled,
          audience_mode: audienceMode,
          api_url: apiURL.trim(),
          api_key: apiKey.trim(),
        }),
      })) as SandboxServiceDefault
      applyConfig(next)
      toast.success(t("admin.sandboxDefaultSaved"))
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t("admin.saveSandboxDefaultFailed"))
    } finally {
      setSaving(false)
    }
  }

  const addAudience = async () => {
    const accountLogin = normalizeSandboxAudienceLogin(candidateLogin)
    if (!accountLogin) return
    setAudienceMutating(true)
    setError("")
    try {
      const next = (await request("/admin/api/sandbox-service-default/audiences", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ account_login: accountLogin }),
      })) as SandboxServiceDefault
      setConfig(next)
      setCandidateLogin("")
      toast.success(t("admin.githubAccountAdded"))
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t("admin.addGitHubAccountFailed"))
    } finally {
      setAudienceMutating(false)
    }
  }

  const removeAudience = async (id: number) => {
    setAudienceMutating(true)
    setError("")
    try {
      const next = (await request(`/admin/api/sandbox-service-default/audiences/${encodeURIComponent(String(id))}`, {
        method: "DELETE",
      })) as SandboxServiceDefault
      setConfig(next)
      toast.success(t("admin.githubAccountRemoved"))
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t("admin.removeGitHubAccountFailed"))
    } finally {
      setAudienceMutating(false)
    }
  }

  const removeAPIKey = async () => {
    setDeleting(true)
    setError("")
    try {
      const next = (await request("/admin/api/sandbox-service-default/api-key", {
        method: "DELETE",
      })) as SandboxServiceDefault
      applyConfig(next)
      setRemoveOpen(false)
      toast.success(t("admin.sandboxAPIKeyRemoved"))
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t("admin.removeSandboxAPIKeyFailed"))
    } finally {
      setDeleting(false)
    }
  }

  return (
    <Card className="min-w-0 gap-0 py-0">
      <CardHeader className="border-b px-5 py-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0">
            <CardTitle className="flex items-center gap-2 text-base">
              <ShieldCheck className="size-4 text-primary" />
              {t("admin.sandboxDefault")}
            </CardTitle>
            <CardDescription className="mt-1">
              {t("admin.sandboxDefaultDescription")}
            </CardDescription>
          </div>
          <div className="flex items-center gap-2">
            <Badge variant={status === "Enabled" ? "success" : status === "Incomplete" ? "warning" : "outline"}>
              {t(status === "Enabled" ? "admin.sandboxDefaultEnabled" : status === "Incomplete" ? "admin.sandboxDefaultIncomplete" : "admin.sandboxDefaultDisabled")}
            </Badge>
            <Button type="button" variant="outline" size="icon" onClick={() => void load()} disabled={busy} title={t("common.refresh")}>
              <RefreshCw className={cn("size-4", loading && "animate-spin")} />
            </Button>
          </div>
        </div>
      </CardHeader>

      <form onSubmit={save}>
        <CardContent className="grid gap-6 p-5">
          <div className="flex flex-col gap-3 border-b pb-5 sm:flex-row sm:items-center sm:justify-between">
            <div className="min-w-0">
              <Label htmlFor="sandbox-default-enabled" className="text-sm font-medium">
                {t("admin.enableFallback")}
              </Label>
              <p className="mt-1 text-sm text-muted-foreground">
                {t("admin.fallbackPrecedence")}
              </p>
            </div>
            <Switch
              id="sandbox-default-enabled"
              checked={enabled}
              onCheckedChange={(checked) => {
                setEnabled(checked)
                setError("")
              }}
              disabled={busy}
            />
          </div>

          <div className="grid gap-4 border-b pb-5">
            <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div className="min-w-0">
                <Label className="text-sm font-medium">{t("admin.availability")}</Label>
                <p className="mt-1 text-sm text-muted-foreground">
                  {t("admin.availabilityDescription")}
                </p>
              </div>
              <div className="inline-flex max-w-full self-start rounded-md border bg-muted/30 p-0.5 sm:shrink-0 sm:self-auto" role="radiogroup" aria-label={t("admin.sandboxDefaultAvailability")}>
                {(["all", "selected"] as const).map((mode) => (
                  <Button
                    key={mode}
                    type="button"
                    size="sm"
                    variant="ghost"
                    role="radio"
                    aria-checked={audienceMode === mode}
                    className={cn("h-8 rounded-[5px] px-3", audienceMode === mode && "bg-background shadow-sm hover:bg-background")}
                    onClick={() => {
                      setAudienceMode(mode)
                      setError("")
                    }}
                    disabled={busy}
                  >
                    {mode === "all" ? t("admin.allAccounts") : t("admin.selectedAccounts")}
                  </Button>
                ))}
              </div>
            </div>

            {audienceMode === "selected" ? (
              <div className="grid min-w-0 gap-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="text-sm font-medium">
                    {config.audiences.length === 0
                      ? t("admin.noGitHubAccountsSelected")
                      : t("admin.availableSelectedAccounts", { count: config.audiences.length })}
                  </span>
                  {config.audiences.length === 0 ? <Badge variant="warning">{t("admin.matchesNobody")}</Badge> : null}
                </div>
                <div className="flex min-w-0 gap-2">
                    <Input
                      value={candidateLogin}
                      onChange={(event) => setCandidateLogin(event.target.value)}
                      onKeyDown={(event) => {
                        if (event.key === "Enter") {
                          event.preventDefault()
                          void addAudience()
                        }
                      }}
                      list="sandbox-audience-account-suggestions"
                      aria-label={t("admin.githubAccount")}
                      placeholder={t("admin.githubAccountPlaceholder")}
                      autoComplete="off"
                      disabled={busy}
                    />
                    <datalist id="sandbox-audience-account-suggestions">
                      {availableAccounts.map((account) => (
                        <option key={`${account.account_type}:${account.github_account_id}`} value={account.account_login}>
                          {account.account_type === "organization" ? t("admin.organization") : t("common.user")}
                        </option>
                      ))}
                    </datalist>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          type="button"
                          size="icon"
                          variant="outline"
                          onClick={() => void addAudience()}
                          disabled={busy || !normalizeSandboxAudienceLogin(candidateLogin)}
                        >
                          <Plus className="size-4" />
                          <span className="sr-only">{t("admin.addGitHubAccount")}</span>
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>{t("admin.addGitHubAccount")}</TooltipContent>
                    </Tooltip>
                </div>

                {config.audiences.length > 0 ? (
                  <div className="divide-y rounded-md border">
                    {config.audiences.map((audience) => (
                      <div key={audience.id} className="flex min-w-0 items-center gap-3 px-3 py-2.5">
                        {audience.account_type === "organization" ? (
                          <Building2 className="size-4 shrink-0 text-muted-foreground" />
                        ) : (
                          <UserRound className="size-4 shrink-0 text-muted-foreground" />
                        )}
                        <div className="min-w-0 flex-1">
                          <div className="truncate text-sm font-medium">{audience.account_login}</div>
                          <div className="truncate text-xs text-muted-foreground">
                            {t("admin.githubAccountID", {
                              type: audience.account_type === "organization" ? t("admin.organization") : t("common.user"),
                              id: audience.github_account_id,
                            })}
                          </div>
                        </div>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button type="button" size="icon" variant="ghost" className="size-8" onClick={() => void removeAudience(audience.id)} disabled={busy}>
                              <X className="size-4" />
                              <span className="sr-only">{t("admin.removeNamedAccount", { login: audience.account_login })}</span>
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>{t("admin.removeAccount")}</TooltipContent>
                        </Tooltip>
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="text-sm text-muted-foreground">
                    {t("admin.noSelectedGitHubAccounts")}
                  </p>
                )}
              </div>
            ) : null}
          </div>

          <div className="grid gap-5 xl:grid-cols-[minmax(240px,0.8fr)_minmax(300px,1.2fr)]">
            <div className="grid min-w-0 content-start gap-2">
              <Label htmlFor="sandbox-default-region">{t("common.region")}</Label>
              <Select
                value={selectedRegion?.id ?? ""}
                onValueChange={(regionID) => {
                  const region = sandboxRegions.find((item) => item.id === regionID)
                  setAPIURL(region?.apiURL ?? "")
                  setError("")
                }}
                disabled={busy}
              >
                <SelectTrigger id="sandbox-default-region" className="w-full">
                  {selectedRegion ? (
                    <span className="truncate">{selectedRegion.label}</span>
                  ) : apiURL ? (
                    <span className="truncate">{t("admin.savedEndpoint")}</span>
                  ) : (
                    <SelectValue placeholder={t("admin.selectRegion")} />
                  )}
                </SelectTrigger>
                <SelectContent>
                  {sandboxRegions.map((region) => (
                    <SelectItem key={region.id} value={region.id} textValue={region.label}>
                      <span>{region.label}</span>
                      <span className="text-muted-foreground">{region.id}</span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="grid min-w-0 content-start gap-2">
              <Label htmlFor="sandbox-default-api-key">{t("common.apiKey")}</Label>
              <Input
                id="sandbox-default-api-key"
                type="password"
                value={apiKey}
                onChange={(event) => setAPIKey(event.target.value)}
                placeholder={config.api_key.configured ? t("admin.apiKeyReplacementPlaceholder") : t("admin.apiKeyPlaceholder")}
                autoComplete="new-password"
                disabled={busy}
              />
              <p className="text-sm text-muted-foreground">
                {config.api_key.configured
                  ? config.api_key.updated_at
                    ? t("admin.encryptedKeySavedAt", { time: formatTime(config.api_key.updated_at, i18n.resolvedLanguage) })
                    : t("admin.encryptedKeySaved")
                  : t("admin.noGlobalAPIKey")}
              </p>
            </div>
          </div>

          {error ? (
            <div className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {error}
            </div>
          ) : null}

          <div className="flex flex-wrap items-center justify-between gap-3 border-t pt-5">
            <div className="flex min-w-0 items-center gap-2 text-sm text-muted-foreground">
              <KeyRound className="size-4 shrink-0" />
              <span className="truncate">{apiURL || t("admin.noEndpoint")}</span>
            </div>
            <div className="flex flex-wrap gap-2">
              {config.api_key.configured ? (
                <Button type="button" variant="outline" onClick={() => setRemoveOpen(true)} disabled={busy}>
                  <Trash2 className="size-4" />
                  {t("admin.removeAPIKey")}
                </Button>
              ) : null}
              <Button type="submit" disabled={busy || (enabled && (!apiURL.trim() || (!config.api_key.configured && !apiKey.trim())))}>
                <Save className="size-4" />
                {saving ? t("admin.saving") : t("admin.saveSettings")}
              </Button>
            </div>
          </div>
        </CardContent>
      </form>

      <Dialog open={removeOpen} onOpenChange={(open) => !deleting && setRemoveOpen(open)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("admin.removeGlobalAPIKeyTitle")}</DialogTitle>
            <DialogDescription>
              {t("admin.removeGlobalAPIKeyDescription")}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline" disabled={deleting}>
                {t("common.cancel")}
              </Button>
            </DialogClose>
            <Button type="button" variant="destructive" onClick={() => void removeAPIKey()} disabled={deleting}>
              <Trash2 className="size-4" />
              {deleting ? t("admin.removing") : t("admin.removeAPIKey")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}
