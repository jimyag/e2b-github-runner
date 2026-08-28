import { Copy, Loader2, Pencil, Plus, RotateCcw, Trash2 } from "lucide-react"
import { useEffect, useMemo, useState, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import type { SandboxTemplate, UserRunnerSpec } from "@/admin-types"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

type RequestFn = (url: string, options?: RequestInit) => Promise<unknown>
type FormState = { name: string; labels: string; templateID: string; maxConcurrency: string; enabled: boolean }

const emptyForm: FormState = { name: "", labels: "", templateID: "", maxConcurrency: "0", enabled: true }

function parseLabels(value: string) {
  return value.split(",").map((label) => label.trim()).filter(Boolean)
}

export function RunnerTypeDetails({ item }: { item: UserRunnerSpec }) {
  const { t } = useTranslation()
  return <div className="mt-2 grid gap-1 text-xs text-muted-foreground sm:grid-cols-2">
    <span>{t("user.globalMaxConcurrency")}: {item.global_max_concurrency || t("user.unlimited")}</span>
    <span>{t("user.scopeMaxConcurrency")}: {item.scope_max_concurrency || t("user.unlimited")}</span>
    <span>{t("user.effectiveMaxConcurrency")}: {item.effective_max_concurrency || t("user.unlimited")}</span>
    <span>{t("common.template")}: {item.default_template_name || item.template_id || "-"}</span>
  </div>
}

export function RunnerTypeItem({ item, onCopy, onEdit, onDelete, onControl, onReset, copying }: {
  item: UserRunnerSpec
  onCopy: (item: UserRunnerSpec) => void
  onEdit: (item: UserRunnerSpec) => void
  onDelete: (item: UserRunnerSpec) => void
  onControl: (item: UserRunnerSpec) => void
  onReset: (item: UserRunnerSpec) => void
  copying: boolean
}) {
  const { t } = useTranslation()
  return <div className="flex min-w-0 flex-wrap items-center justify-between gap-3 rounded-md border p-3">
    <div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><span className="font-medium">{item.name}</span><Badge variant={item.enabled ? "default" : "secondary"}>{item.enabled ? t("common.enabled") : t("common.disabled")}</Badge><Badge variant="outline">{item.source}</Badge>{item.overrides_global ? <Badge variant="outline">{t("user.overridesGlobal")}</Badge> : null}</div><div className="mt-1 break-words font-mono text-xs text-muted-foreground">{t("user.runsOn")}: [{item.workflow_labels.join(", ")}]</div><RunnerTypeDetails item={item} /></div>
    <div className="flex items-center gap-1"><Button type="button" size="icon" variant="ghost" onClick={() => onCopy(item)} disabled={copying} title={t("user.copyRunnerTypeYAML")} aria-label={t("user.copyRunnerTypeYAML")}><Copy className="h-4 w-4" /></Button>{item.source === "managed" ? <><Button type="button" size="icon" variant="ghost" onClick={() => onControl(item)} title={t("user.editRunnerTypeControl")} aria-label={`${t("user.editRunnerTypeControl")} ${item.name}`}><Pencil className="h-4 w-4" /></Button>{item.scope_max_concurrency > 0 || !item.enabled ? <Button type="button" size="icon" variant="ghost" onClick={() => onReset(item)} title={t("user.resetRunnerTypeControl")} aria-label={`${t("user.resetRunnerTypeControl")} ${item.name}`}><RotateCcw className="h-4 w-4" /></Button> : null}</> : item.source === "scoped_custom" ? <><Button type="button" size="icon" variant="ghost" onClick={() => onEdit(item)} title={t("common.edit")} aria-label={`${t("common.edit")} ${item.name}`}><Pencil className="h-4 w-4" /></Button><Button type="button" size="icon" variant="ghost" onClick={() => onDelete(item)} title={t("common.delete")} aria-label={`${t("common.delete")} ${item.name}`}><Trash2 className="h-4 w-4" /></Button></> : null}</div>
  </div>
}

export function RunnerTypeForm({ mode, form, templates, saving, onChange, onSubmit, onClose }: {
  mode: "create" | "edit" | "control"
  form: FormState
  templates: SandboxTemplate[]
  saving: boolean
  onChange: (next: Partial<FormState>) => void
  onSubmit: (event: FormEvent<HTMLFormElement>) => void
  onClose: () => void
}) {
  const { t } = useTranslation()
  const editable = mode !== "control"
  return <form onSubmit={onSubmit} className="grid gap-4">{editable ? <><div className="grid gap-2"><Label htmlFor="user-runner-name">{t("common.name")}</Label><Input id="user-runner-name" value={form.name} onChange={(event) => onChange({ name: event.target.value })} required disabled={mode === "edit"} /></div><div className="grid gap-2"><Label htmlFor="user-runner-labels">{t("common.labels")}</Label><Input id="user-runner-labels" value={form.labels} onChange={(event) => onChange({ labels: event.target.value })} required /></div><div className="grid gap-2"><Label htmlFor="user-runner-template">{t("common.template")}</Label>{templates.length > 0 ? <select id="user-runner-template" className="h-9 rounded-md border bg-background px-3 text-sm" value={form.templateID} onChange={(event) => onChange({ templateID: event.target.value })} required><option value="">{t("user.selectTemplate")}</option>{templates.map((template) => <option key={template.template_id} value={template.template_id}>{template.template_id}</option>)}</select> : <Input id="user-runner-template" value={form.templateID} onChange={(event) => onChange({ templateID: event.target.value })} required />}</div></> : null}<div className="grid gap-2"><Label htmlFor="user-runner-concurrency">{t("user.scopeMaxConcurrency")}</Label><Input id="user-runner-concurrency" inputMode="numeric" min="0" type="number" value={form.maxConcurrency} onChange={(event) => onChange({ maxConcurrency: event.target.value })} /></div><label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={form.enabled} onChange={(event) => onChange({ enabled: event.target.checked })} />{t("common.enabled")}</label><DialogFooter><Button type="button" variant="outline" onClick={onClose}>{t("common.cancel")}</Button><Button type="submit" disabled={saving}>{saving ? t("admin.saving") : t("common.save")}</Button></DialogFooter></form>
}

export function UserRunnerTypesSection({ request, installationID }: { request: RequestFn; installationID?: number }) {
  const { t } = useTranslation()
  const [items, setItems] = useState<UserRunnerSpec[]>([])
  const [sandboxSource, setSandboxSource] = useState("none")
  const [templates, setTemplates] = useState<SandboxTemplate[]>([])
  const [loading, setLoading] = useState(true)
  const [copying, setCopying] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [dialog, setDialog] = useState<"create" | "edit" | "control" | null>(null)
  const [selected, setSelected] = useState<UserRunnerSpec | null>(null)
  const [form, setForm] = useState<FormState>(emptyForm)
  const query = installationID ? `?installation_id=${installationID}` : ""

  const load = async () => { setLoading(true); try { const response = await request(`/user/runner-specs${query}`) as { items?: UserRunnerSpec[]; sandbox_source?: string }; setItems(response.items || []); setSandboxSource(response.sandbox_source || "none") } catch (error) { toast.error(error instanceof Error ? error.message : t("user.runnerTypesLoadFailed")) } finally { setLoading(false) } }
  useEffect(() => { let cancelled = false; setLoading(true); void request(`/user/runner-specs${query}`).then((response) => { if (cancelled) return; const data = response as { items?: UserRunnerSpec[]; sandbox_source?: string }; setItems(data.items || []); setSandboxSource(data.sandbox_source || "none") }).catch((error) => { if (!cancelled) toast.error(error instanceof Error ? error.message : t("user.runnerTypesLoadFailed")) }).finally(() => { if (!cancelled) setLoading(false) }); return () => { cancelled = true } }, [query, request, t])
  const openCreate = async () => { setSelected(null); setForm(emptyForm); setDialog("create"); try { const suffix = installationID ? `&installation_id=${installationID}` : ""; const data = await request(`/user/sandbox/templates?region=us-south-1${suffix}`); setTemplates(Array.isArray(data) ? data as SandboxTemplate[] : []) } catch (error) { setTemplates([]); toast.error(error instanceof Error ? error.message : t("user.runnerTypesLoadFailed")) } }
  const openEdit = (item: UserRunnerSpec) => { setSelected(item); setForm({ name: item.name, labels: item.workflow_labels.join(", "), templateID: item.template_id || "", maxConcurrency: String(item.scope_max_concurrency), enabled: item.enabled }); setDialog("edit") }
  const openControl = (item: UserRunnerSpec) => { setSelected(item); setForm({ ...emptyForm, name: item.name, maxConcurrency: String(item.scope_max_concurrency), enabled: item.enabled }); setDialog("control") }
  const closeDialog = () => { if (!saving) setDialog(null) }
  const submit = async (event: FormEvent<HTMLFormElement>) => { event.preventDefault(); if (saving) return; setSaving(true); try { let url = `/user/runner-specs${query}`; let options: RequestInit = { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name: form.name, workflow_labels: parseLabels(form.labels), template_id: form.templateID, max_concurrency: Number(form.maxConcurrency) || 0, enabled: form.enabled }) }; if (dialog === "edit" && selected) { url = `/user/runner-specs/${encodeURIComponent(selected.name)}${query}`; options = { method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ workflow_labels: parseLabels(form.labels), template_id: form.templateID, max_concurrency: Number(form.maxConcurrency) || 0, enabled: form.enabled, expected_updated_at: selected.updated_at }) } } else if (dialog === "control" && selected) { url = `/user/runner-specs/${encodeURIComponent(selected.name)}/control${query}`; options = { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ max_concurrency: Number(form.maxConcurrency) || 0, enabled: form.enabled, expected_updated_at: selected.updated_at }) } } await request(url, options); toast.success(t("user.runnerTypeSaved")); setDialog(null); await load() } catch (error) { toast.error(error instanceof Error ? error.message : t("user.runnerTypeSaveFailed")) } finally { setSaving(false) } }
  const remove = async (item: UserRunnerSpec) => { if (saving || !window.confirm(t("user.confirmDeleteRunnerType", { name: item.name }))) return; setSaving(true); try { const separator = query ? "&" : "?"; await request(`/user/runner-specs/${encodeURIComponent(item.name)}${query}${separator}expected_updated_at=${encodeURIComponent(item.updated_at)}`, { method: "DELETE" }); toast.success(t("user.runnerTypeDeleted")); await load() } catch (error) { toast.error(error instanceof Error ? error.message : t("user.runnerTypeDeleteFailed")) } finally { setSaving(false) } }
  const reset = async (item: UserRunnerSpec) => { if (saving) return; setSaving(true); try { const separator = query ? "&" : "?"; await request(`/user/runner-specs/${encodeURIComponent(item.name)}/control${query}${separator}expected_updated_at=${encodeURIComponent(item.updated_at)}`, { method: "DELETE" }); toast.success(t("user.runnerTypeReset")); await load() } catch (error) { toast.error(error instanceof Error ? error.message : t("user.runnerTypeSaveFailed")) } finally { setSaving(false) } }
  const copyYAML = async (item: UserRunnerSpec) => { setCopying(item.name); try { await navigator.clipboard?.writeText(`runs-on: [${item.workflow_labels.join(", ")}]`); toast.success(t("user.runnerTypeCopied")) } catch { toast.error(t("user.runnerTypeCopyFailed")) } finally { setCopying(null) } }
  const canCreate = sandboxSource !== "none" && sandboxSource !== "admin_default"
  const sorted = useMemo(() => [...items].sort((a, b) => a.name.localeCompare(b.name)), [items])
  return <Card><CardHeader className="flex flex-row items-center justify-between gap-3"><CardTitle>{t("user.runnerTypes")}</CardTitle><Button type="button" size="sm" disabled={!canCreate || saving} title={!canCreate ? t("user.runnerTypesConfigureSandbox") : undefined} onClick={() => void openCreate()}><Plus className="h-4 w-4" />{t("user.createRunnerType")}</Button></CardHeader><CardContent>{loading ? <div className="flex items-center gap-2 text-sm text-muted-foreground"><Loader2 className="h-4 w-4 animate-spin" />{t("common.loading")}</div> : null}{!loading && sorted.length === 0 ? <p className="text-sm text-muted-foreground">{t("user.noRunnerTypes")}</p> : null}<div className="grid gap-3">{sorted.map((item) => <RunnerTypeItem key={`${item.source}:${item.name}`} item={item} copying={copying === item.name} onCopy={(value) => void copyYAML(value)} onEdit={openEdit} onDelete={(value) => void remove(value)} onControl={openControl} onReset={(value) => void reset(value)} />)}</div><Button type="button" variant="link" className="mt-3 px-0" onClick={() => void load()} disabled={loading || saving}>{t("common.refresh")}</Button></CardContent><Dialog open={dialog !== null} onOpenChange={(open) => { if (!open) closeDialog() }}><DialogContent><DialogHeader><DialogTitle>{dialog === "create" ? t("user.createRunnerType") : dialog === "control" ? t("user.editRunnerTypeControl") : t("common.edit")}</DialogTitle></DialogHeader><RunnerTypeForm mode={dialog || "create"} form={form} templates={templates} saving={saving} onChange={(next) => setForm((current) => ({ ...current, ...next }))} onSubmit={submit} onClose={closeDialog} /></DialogContent></Dialog></Card>
}
