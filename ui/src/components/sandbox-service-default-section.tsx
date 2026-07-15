import { type FormEvent, useCallback, useEffect, useMemo, useState } from "react"
import { Building2, KeyRound, Plus, RefreshCw, Save, ShieldCheck, Trash2, UserRound, X } from "lucide-react"
import { toast } from "sonner"

import { formatTime } from "@/admin-format"
import type { SandboxServiceDefault } from "@/admin-types"
import { sandboxRegions } from "@/components/sandbox-catalog-utils"
import {
  availableSandboxAudienceAccounts,
  normalizeSandboxAudienceLogin,
  sandboxAudienceSummary,
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
      setError(cause instanceof Error ? cause.message : "Failed to load Sandbox service default.")
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
      setError("Sandbox service region is required before enabling the default.")
      return
    }
    if (enabled && !config.api_key.configured && !apiKey.trim()) {
      setError("Sandbox API Key is required before enabling the default.")
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
      toast.success("Sandbox service default saved")
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Failed to save Sandbox service default.")
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
      toast.success("GitHub account added")
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Failed to add GitHub account.")
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
      toast.success("GitHub account removed")
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Failed to remove GitHub account.")
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
      toast.success("Sandbox API Key removed")
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Failed to remove Sandbox API Key.")
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
              Sandbox service default
            </CardTitle>
            <CardDescription className="mt-1">
              Platform credentials for accounts and organizations without a complete scoped configuration.
            </CardDescription>
          </div>
          <div className="flex items-center gap-2">
            <Badge variant={status === "Enabled" ? "success" : status === "Incomplete" ? "warning" : "outline"}>
              {status}
            </Badge>
            <Button type="button" variant="outline" size="icon" onClick={() => void load()} disabled={busy} title="Refresh">
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
                Enable fallback for unconfigured accounts
              </Label>
              <p className="mt-1 text-sm text-muted-foreground">
                Scoped account and organization credentials continue to take precedence.
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
                <Label className="text-sm font-medium">Availability</Label>
                <p className="mt-1 text-sm text-muted-foreground">
                  Match the GitHub owner of each repository.
                </p>
              </div>
              <div className="inline-flex max-w-full self-start rounded-md border bg-muted/30 p-0.5 sm:shrink-0 sm:self-auto" role="radiogroup" aria-label="Sandbox default availability">
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
                    {mode === "all" ? "All accounts" : "Selected accounts"}
                  </Button>
                ))}
              </div>
            </div>

            {audienceMode === "selected" ? (
              <div className="grid min-w-0 gap-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="text-sm font-medium">{sandboxAudienceSummary(audienceMode, config.audiences.length)}</span>
                  {config.audiences.length === 0 ? <Badge variant="warning">Matches nobody</Badge> : null}
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
                      aria-label="GitHub account"
                      placeholder="GitHub user or organization"
                      autoComplete="off"
                      disabled={busy}
                    />
                    <datalist id="sandbox-audience-account-suggestions">
                      {availableAccounts.map((account) => (
                        <option key={`${account.account_type}:${account.github_account_id}`} value={account.account_login}>
                          {account.account_type === "organization" ? "Organization" : "User"}
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
                          <span className="sr-only">Add GitHub account</span>
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>Add GitHub account</TooltipContent>
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
                            {audience.account_type === "organization" ? "Organization" : "User"} · GitHub ID {audience.github_account_id}
                          </div>
                        </div>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button type="button" size="icon" variant="ghost" className="size-8" onClick={() => void removeAudience(audience.id)} disabled={busy}>
                              <X className="size-4" />
                              <span className="sr-only">Remove {audience.account_login}</span>
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>Remove account</TooltipContent>
                        </Tooltip>
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="text-sm text-muted-foreground">
                    No GitHub users or organizations are selected.
                  </p>
                )}
              </div>
            ) : null}
          </div>

          <div className="grid gap-5 xl:grid-cols-[minmax(240px,0.8fr)_minmax(300px,1.2fr)]">
            <div className="grid min-w-0 content-start gap-2">
              <Label htmlFor="sandbox-default-region">Region</Label>
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
                    <span className="truncate">Saved endpoint</span>
                  ) : (
                    <SelectValue placeholder="Select Sandbox region" />
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
              <Label htmlFor="sandbox-default-api-key">API Key</Label>
              <Input
                id="sandbox-default-api-key"
                type="password"
                value={apiKey}
                onChange={(event) => setAPIKey(event.target.value)}
                placeholder={config.api_key.configured ? "Enter a new API Key to replace the saved one" : "Enter Sandbox API Key"}
                autoComplete="new-password"
                disabled={busy}
              />
              <p className="text-sm text-muted-foreground">
                {config.api_key.configured
                  ? `Encrypted key saved${config.api_key.updated_at ? ` · ${formatTime(config.api_key.updated_at)}` : ""}`
                  : "No global Sandbox API Key is saved."}
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
              <span className="truncate">{apiURL || "No endpoint configured"}</span>
            </div>
            <div className="flex flex-wrap gap-2">
              {config.api_key.configured ? (
                <Button type="button" variant="outline" onClick={() => setRemoveOpen(true)} disabled={busy}>
                  <Trash2 className="size-4" />
                  Remove API key
                </Button>
              ) : null}
              <Button type="submit" disabled={busy || (enabled && (!apiURL.trim() || (!config.api_key.configured && !apiKey.trim())))}>
                <Save className="size-4" />
                {saving ? "Saving" : "Save settings"}
              </Button>
            </div>
          </div>
        </CardContent>
      </form>

      <Dialog open={removeOpen} onOpenChange={(open) => !deleting && setRemoveOpen(open)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Remove global Sandbox API Key?</DialogTitle>
            <DialogDescription>
              The global default becomes incomplete immediately. Scoped account and organization credentials are not changed.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline" disabled={deleting}>
                Cancel
              </Button>
            </DialogClose>
            <Button type="button" variant="destructive" onClick={() => void removeAPIKey()} disabled={deleting}>
              <Trash2 className="size-4" />
              {deleting ? "Removing" : "Remove API key"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}
